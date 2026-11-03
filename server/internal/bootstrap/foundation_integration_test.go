//go:build integration

package bootstrap_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	redisclient "github.com/redis/go-redis/v9"
	"linq-bs/server/internal/bootstrap"
	"linq-bs/server/internal/framework/config"
	httpserver "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/framework/redis"
	"linq-bs/server/internal/framework/storage"
)

// 集成测试必须使用实际环境；缺失配置或依赖时失败，不跳过验证。
func loadConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestConfigRejectsInvalidEnvironment(t *testing.T) {
	loadConfig(t)
	t.Run("missing_password", func(t *testing.T) {
		t.Setenv("XA_POSTGRES_PASSWORD", "")
		if _, err := config.Load(); err == nil {
			t.Fatal("空密码未被拒绝")
		}
	})
	t.Run("invalid_timeout", func(t *testing.T) {
		t.Setenv("XA_SHUTDOWN_TIMEOUT", "0s")
		if _, err := config.Load(); err == nil {
			t.Fatal("无效超时未被拒绝")
		}
	})
}

func TestRealDependencyReadWrite(t *testing.T) {
	cfg := loadConfig(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	// 使用仓库实际文件验证完整字节内容，不构造业务记录或模拟返回值。
	content, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("postgres_commit_and_rollback", func(t *testing.T) {
		db, err := postgres.Open(ctx, cfg.Postgres)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		conn, err := db.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		if _, err := conn.Exec(ctx, "CREATE TEMP TABLE xa_foundation_probe (content bytea NOT NULL)"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := conn.Exec(context.Background(), "DROP TABLE xa_foundation_probe"); err != nil {
				t.Error(err)
			}
		}()
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "INSERT INTO xa_foundation_probe VALUES ($1)", content)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		abort := errors.New("验证事务回滚")
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "DELETE FROM xa_foundation_probe"); err != nil {
				return err
			}
			return abort
		})
		if !errors.Is(err, abort) {
			t.Fatalf("回滚结果不正确: %v", err)
		}
		var stored []byte
		if err := conn.QueryRow(ctx, "SELECT content FROM xa_foundation_probe").Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(content, stored) {
			t.Fatal("事务提交或回滚后内容不一致")
		}
		var version string
		if err := conn.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'vector'").Scan(&version); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("redis_write_read_delete", func(t *testing.T) {
		cache, err := redis.Open(ctx, cfg.Redis)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := cache.Close(); err != nil {
				t.Error(err)
			}
		}()
		key := "xa:integration:" + rand.Text()
		defer func() {
			if err := cache.Del(context.Background(), key).Err(); err != nil {
				t.Error(err)
			}
		}()
		if err := cache.Set(ctx, key, content, time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		stored, err := cache.Get(ctx, key).Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(content, stored) {
			t.Fatal("Redis 读取内容不一致")
		}
		if err := cache.Del(ctx, key).Err(); err != nil {
			t.Fatal(err)
		}
		if err := cache.Get(ctx, key).Err(); !errors.Is(err, redisclient.Nil) {
			t.Fatalf("删除后仍可读取: %v", err)
		}
	})

	t.Run("s3_upload_read_delete", func(t *testing.T) {
		objects, err := storage.Open(ctx, cfg.ObjectStore)
		if err != nil {
			t.Fatal(err)
		}
		defer objects.Close()
		key := "integration/" + rand.Text() + "/go.mod"
		defer func() {
			if err := objects.Delete(context.Background(), key); err != nil {
				t.Error(err)
			}
		}()
		if err := objects.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "text/plain"); err != nil {
			t.Fatal(err)
		}
		reader, err := objects.Open(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		stored, readErr := io.ReadAll(reader)
		if err := errors.Join(readErr, reader.Close()); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(content, stored) {
			t.Fatal("S3 读取内容不一致")
		}
		if err := objects.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
		reader, err = objects.Open(ctx, key)
		if err == nil {
			if err := reader.Close(); err != nil {
				t.Error(err)
			}
			t.Fatal("删除后的对象仍能打开")
		}
	})
}

func TestProcessLifecycle(t *testing.T) {
	for _, name := range []string{"server", "worker"} {
		t.Run(name, func(t *testing.T) {
			cfg := loadConfig(t)
			address := freeAddress(t)
			cfg.HTTP.ServerAddress = address
			if name == "worker" {
				cfg.HTTP.ServerAddress = loadConfig(t).HTTP.ServerAddress
				cfg.HTTP.WorkerAddress = address
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if name == "worker" {
					done <- bootstrap.NewWorker(cfg, slog.Default()).Run(ctx)
					return
				}
				done <- bootstrap.NewServer(cfg, slog.Default()).Run(ctx)
			}()
			waitReady(t, "http://"+address, done)
			for _, path := range []string{"/health/live", "/health/ready"} {
				status, _ := request(t, "http://"+address+path)
				if status != http.StatusOK {
					t.Fatalf("%s 返回 %d", path, status)
				}
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(cfg.ShutdownTimeout + time.Second):
				t.Fatal("进程未按时退出")
			}
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatalf("退出后端口未释放: %v", err)
			}
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStartupRejectsDependencyFailureAndPortConflict(t *testing.T) {
	t.Run("invalid_redis_credentials", func(t *testing.T) {
		cfg := loadConfig(t)
		cfg.Redis.Password += rand.Text()
		ctx, cancel := context.WithTimeout(t.Context(), cfg.StartupTimeout)
		defer cancel()
		if err := bootstrap.NewServer(cfg, slog.Default()).Run(ctx); err == nil || !strings.Contains(err.Error(), "Redis") {
			t.Fatalf("Redis 认证失败未明确返回: %v", err)
		}
	})
	t.Run("occupied_port", func(t *testing.T) {
		cfg := loadConfig(t)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := listener.Close(); err != nil {
				t.Error(err)
			}
		}()
		cfg.HTTP.ServerAddress = listener.Addr().String()
		ctx, cancel := context.WithTimeout(t.Context(), cfg.StartupTimeout)
		defer cancel()
		if err := bootstrap.NewServer(cfg, slog.Default()).Run(ctx); !errors.Is(err, syscall.EADDRINUSE) {
			t.Fatalf("端口冲突未明确返回: %v", err)
		}
	})
}

func TestReadinessDetectsDisconnectedClient(t *testing.T) {
	cfg := loadConfig(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cache, err := redis.Open(ctx, cfg.Redis)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	checks := []httpserver.Check{{Name: "redis", Ping: func(ctx context.Context) error { return cache.Ping(ctx).Err() }}}
	done := make(chan error, 1)
	go func() {
		done <- httpserver.Serve(ctx, listener, httpserver.HealthHandler(checks, cfg.HealthTimeout, slog.Default()), cfg.ShutdownTimeout)
	}()
	baseURL := "http://" + listener.Addr().String()
	waitReady(t, baseURL, done)
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	status, body := request(t, baseURL+"/health/ready")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("连接失效后返回 %d", status)
	}
	var health struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatal(err)
	}
	if health.Dependencies["redis"] != "unavailable" {
		t.Fatal("未报告 Redis 不可用")
	}
	status, _ = request(t, baseURL+"/health/live")
	if status != http.StatusOK {
		t.Fatal("依赖故障不应影响存活检查")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(cfg.ShutdownTimeout + time.Second):
		t.Fatal("HTTP 未按时关闭")
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitReady(t *testing.T, baseURL string, done <-chan error) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.After(20 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("进程在就绪前退出: %v", err)
		case <-deadline:
			t.Fatal("等待进程就绪超时")
		case <-tick.C:
			response, err := client.Get(baseURL + "/health/ready")
			if err != nil {
				continue
			}
			_, readErr := io.Copy(io.Discard, response.Body)
			if err := errors.Join(readErr, response.Body.Close()); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode == http.StatusOK {
				return
			}
		}
	}
}

func request(t *testing.T, endpoint string) (int, []byte) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	if err := errors.Join(readErr, response.Body.Close()); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body
}
