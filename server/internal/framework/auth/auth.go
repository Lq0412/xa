package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

var (
	ErrUnauthenticated = errors.New("未登录")
	ErrForbidden       = errors.New("无权执行此操作")
)

// Principal 表示经过认证的调用主体。
type Principal struct {
	UserID      string
	WorkspaceID string
	Roles       []string
}

type principalKey struct{}

// WithPrincipal 将已验证主体放入请求上下文，后续业务调用只读取这个快照。
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}

// Resolver 从请求上下文中解析调用主体。
type Resolver interface {
	Resolve(ctx context.Context) (Principal, error)
}

// PrincipalLookup 从数据库读取用户当前状态和角色，避免把权限事实长期留在会话缓存中。
type PrincipalLookup func(context.Context, string) (Principal, error)

// Authorizer 校验主体对资源和动作的权限。
type Authorizer interface {
	Authorize(ctx context.Context, principal Principal, resource string, action string) error
}

type contextKey string

const sessionTokenKey contextKey = "xa.session_token"

// WithSessionToken 将 HTTP 层解析出的会话令牌放入上下文，业务层不直接依赖请求对象。
func WithSessionToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, sessionTokenKey, token)
}

func sessionToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(sessionTokenKey).(string)
	return token, ok && token != ""
}

// SessionStore 保存短期登录状态；用户和权限事实仍以 PostgreSQL 为准。
type SessionStore interface {
	Create(ctx context.Context, principal Principal) (string, error)
	Resolve(ctx context.Context, token string) (Principal, error)
	Delete(ctx context.Context, token string) error
}

// RedisSessionStore 使用随机不透明令牌，避免在客户端暴露用户和角色信息。
type RedisSessionStore struct {
	client *redisclient.Client
	ttl    time.Duration
}

func NewRedisSessionStore(client *redisclient.Client, ttl time.Duration) *RedisSessionStore {
	return &RedisSessionStore{client: client, ttl: ttl}
}

func (s *RedisSessionStore) Create(ctx context.Context, principal Principal) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成登录会话令牌: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	encoded, err := json.Marshal(principal)
	if err != nil {
		return "", fmt.Errorf("编码登录主体: %w", err)
	}
	if err := s.client.Set(ctx, sessionKey(token), encoded, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("保存登录会话: %w", err)
	}
	return token, nil
}

func (s *RedisSessionStore) Resolve(ctx context.Context, token string) (Principal, error) {
	encoded, err := s.client.Get(ctx, sessionKey(token)).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("读取登录会话: %w", err)
	}
	var principal Principal
	if err := json.Unmarshal(encoded, &principal); err != nil {
		return Principal{}, fmt.Errorf("解析登录会话: %w", err)
	}
	if principal.UserID == "" || principal.WorkspaceID == "" {
		return Principal{}, ErrUnauthenticated
	}
	return principal, nil
}

func (s *RedisSessionStore) Delete(ctx context.Context, token string) error {
	if err := s.client.Del(ctx, sessionKey(token)).Err(); err != nil {
		return fmt.Errorf("删除登录会话: %w", err)
	}
	return nil
}

func sessionKey(token string) string { return "xa:session:" + token }

// RedisResolver 从请求上下文解析登录主体，未登录时直接返回明确错误。
type RedisResolver struct{ store SessionStore }

type databaseResolver struct {
	store  SessionStore
	lookup PrincipalLookup
}

func NewRedisResolver(store SessionStore, lookups ...PrincipalLookup) *RedisResolver {
	if len(lookups) == 0 {
		return &RedisResolver{store: store}
	}
	return &RedisResolver{store: &databaseResolver{store: store, lookup: lookups[0]}}
}

func (r *databaseResolver) Create(ctx context.Context, principal Principal) (string, error) {
	return r.store.Create(ctx, principal)
}
func (r *databaseResolver) Resolve(ctx context.Context, token string) (Principal, error) {
	principal, err := r.store.Resolve(ctx, token)
	if err != nil {
		return Principal{}, err
	}
	current, err := r.lookup(ctx, principal.UserID)
	if err != nil {
		_ = r.store.Delete(ctx, token)
		if errors.Is(err, ErrUnauthenticated) {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{}, err
	}
	return current, nil
}
func (r *databaseResolver) Delete(ctx context.Context, token string) error {
	return r.store.Delete(ctx, token)
}

func (r *RedisResolver) Resolve(ctx context.Context) (Principal, error) {
	token, ok := sessionToken(ctx)
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	return r.store.Resolve(ctx, token)
}

// BearerToken 只接受标准 Authorization Bearer 格式。
func BearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", false
	}
	return parts[1], true
}

func HasRole(principal Principal, role string) bool {
	for _, current := range principal.Roles {
		if current == role {
			return true
		}
	}
	return false
}
