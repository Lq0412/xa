// Package config 从环境变量读取并校验进程配置，不输出凭证内容。
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/minio/minio-go/v7/pkg/s3utils"
)

// Config 是进程启动所需的配置快照。
type Config struct {
	HTTP            HTTPConfig        `envPrefix:"XA_HTTP_"`
	Admin           AdminConfig       `envPrefix:"XA_ADMIN_"`
	Postgres        PostgresConfig    `envPrefix:"XA_POSTGRES_"`
	Redis           RedisConfig       `envPrefix:"XA_REDIS_"`
	ObjectStore     ObjectStoreConfig `envPrefix:"XA_S3_"`
	Bailian         BailianConfig     `envPrefix:"XA_BAILIAN_"`
	ChatModel       string            `env:"XA_CHAT_MODEL" envDefault:"qwen3.7-plus"`
	LightModel      string            `env:"XA_LIGHT_MODEL" envDefault:"qwen3.8-flash"`
	StartupTimeout  time.Duration     `env:"XA_STARTUP_TIMEOUT" envDefault:"15s"`
	ShutdownTimeout time.Duration     `env:"XA_SHUTDOWN_TIMEOUT" envDefault:"10s"`
	HealthTimeout   time.Duration     `env:"XA_HEALTH_TIMEOUT" envDefault:"3s"`
}

// BailianConfig 保存阿里百炼 OpenAI 兼容接口配置。
type BailianConfig struct {
	APIKey  string `env:"API_KEY,required,notEmpty"`
	BaseURL string `env:"BASE_URL,required,notEmpty"`
}

// AdminConfig 是首次部署时可选的管理员初始化凭证；密码只从环境注入。
type AdminConfig struct {
	Email    string `env:"EMAIL"`
	Password string `env:"PASSWORD"`
}

// HTTPConfig 分开配置业务进程和 worker 的健康接口地址。
type HTTPConfig struct {
	ServerAddress string `env:"SERVER_ADDRESS" envDefault:"127.0.0.1:18080"`
	WorkerAddress string `env:"WORKER_ADDRESS" envDefault:"127.0.0.1:18081"`
}

// PostgresConfig 提供连接参数；密码不设置默认值。
type PostgresConfig struct {
	Host     string `env:"HOST" envDefault:"127.0.0.1"`
	Port     uint16 `env:"PORT" envDefault:"15432"`
	Database string `env:"DATABASE" envDefault:"xa"`
	User     string `env:"USER" envDefault:"xa"`
	Password string `env:"PASSWORD,required,notEmpty"`
	SSLMode  string `env:"SSL_MODE" envDefault:"disable"`
}

// URL 正确转义连接参数，返回值含凭证，禁止记录到日志。
func (c PostgresConfig) URL() string {
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(c.Host, strconv.Itoa(int(c.Port))),
		User: url.UserPassword(c.User, c.Password), Path: "/" + c.Database}
	q := url.Values{"sslmode": {c.SSLMode}}
	u.RawQuery = q.Encode()
	return u.String()
}

// RedisConfig 指定短期状态存储及认证信息。
type RedisConfig struct {
	Address  string `env:"ADDRESS" envDefault:"127.0.0.1:16379"`
	Password string `env:"PASSWORD,required,notEmpty"`
}

// ObjectStoreConfig 使用 S3 协议，Endpoint 仅包含主机和可选端口。
type ObjectStoreConfig struct {
	Endpoint  string `env:"ENDPOINT" envDefault:"127.0.0.1:19000"`
	Bucket    string `env:"BUCKET" envDefault:"xa-files"`
	Region    string `env:"REGION" envDefault:"us-east-1"`
	AccessKey string `env:"ACCESS_KEY,required,notEmpty"`
	SecretKey string `env:"SECRET_KEY,required,notEmpty"`
	Secure    bool   `env:"SECURE" envDefault:"false"`
}

// Load 读取当前环境；格式或约束错误在创建外部连接之前返回。
func Load() (Config, error) {
	c, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("读取环境配置: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate 校验进程必需的配置，不尝试自动补充凭证或替换依赖。
func (c Config) Validate() error {
	for name, address := range map[string]string{"server": c.HTTP.ServerAddress, "worker": c.HTTP.WorkerAddress, "redis": c.Redis.Address} {
		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%s 地址必须包含有效主机和端口", name)
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%s 端口必须在 1 到 65535 之间", name)
		}
	}
	if c.HTTP.ServerAddress == c.HTTP.WorkerAddress {
		return fmt.Errorf("server 和 worker 不能使用相同监听地址")
	}
	if (c.Admin.Email == "") != (c.Admin.Password == "") {
		return fmt.Errorf("管理员邮箱和密码必须同时配置")
	}
	if len(c.Admin.Password) > 0 && len(c.Admin.Password) < 8 {
		return fmt.Errorf("管理员密码至少需要 8 个字符")
	}
	if c.StartupTimeout <= 0 || c.ShutdownTimeout <= 0 || c.HealthTimeout <= 0 {
		return fmt.Errorf("启动、关闭和健康检查超时必须大于零")
	}
	if c.Postgres.Host == "" || c.Postgres.Port == 0 || c.Postgres.Database == "" || c.Postgres.User == "" || c.Postgres.Password == "" {
		return fmt.Errorf("PostgreSQL 连接参数不完整")
	}
	switch c.Postgres.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("不支持的 PostgreSQL SSL 模式")
	}
	if c.Redis.Password == "" || c.ObjectStore.AccessKey == "" || c.ObjectStore.SecretKey == "" {
		return fmt.Errorf("Redis 和对象存储凭证不能为空")
	}
	if c.Bailian.APIKey == "" || c.Bailian.BaseURL == "" {
		return fmt.Errorf("阿里百炼配置不能为空")
	}
	if _, err := url.ParseRequestURI(c.Bailian.BaseURL); err != nil {
		return fmt.Errorf("阿里百炼 BaseURL 无效")
	}
	u, err := url.Parse("https://" + c.ObjectStore.Endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("对象存储 Endpoint 只能包含主机和可选端口")
	}
	if err := s3utils.CheckValidBucketNameStrict(c.ObjectStore.Bucket); err != nil {
		return fmt.Errorf("对象存储 Bucket 名称无效: %w", err)
	}
	if c.ObjectStore.Region == "" {
		return fmt.Errorf("对象存储 Region 不能为空")
	}
	return nil
}
