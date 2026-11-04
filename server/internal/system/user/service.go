package user

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"linq-bs/server/internal/framework/workspace"
)

var (
	ErrNotFound  = errors.New("用户不存在")
	ErrConflict  = errors.New("用户已存在")
	ErrInvalid   = errors.New("用户资料无效")
	ErrDisabled  = errors.New("用户已停用")
	ErrBadSecret = errors.New("邮箱或密码错误")
)

// User 表示平台用户。
type User struct {
	ID          string
	WorkspaceID string
	Email       string
	DisplayName string
	Role        string
	Status      string
}

// Credentials 是登录和注册所需的最小输入，不接受客户端指定身份和角色。
type Credentials struct {
	Email       string
	Password    string
	DisplayName string
}

// Repository 隔离用户数据访问。
type Repository interface {
	Find(ctx context.Context, userID string) (User, error)
	FindByEmail(ctx context.Context, workspaceID, email string) (User, string, error)
	Save(ctx context.Context, user User, passwordHash string) error
}

// Service 提供用户相关用例。
type Service interface {
	Get(ctx context.Context, userID string) (User, error)
	Register(ctx context.Context, credentials Credentials) (User, error)
	Authenticate(ctx context.Context, email, password string) (User, error)
}

// ServiceImpl 实现用户资料和密码校验，密码只以 bcrypt 摘要形式交给仓储。
type ServiceImpl struct {
	repository Repository
}

func NewService(repository Repository) *ServiceImpl { return &ServiceImpl{repository: repository} }

func (s *ServiceImpl) Get(ctx context.Context, userID string) (User, error) {
	if userID == "" {
		return User{}, ErrNotFound
	}
	return s.repository.Find(ctx, userID)
}

func (s *ServiceImpl) Register(ctx context.Context, credentials Credentials) (User, error) {
	email, err := normalizeEmail(credentials.Email)
	if err != nil || len(credentials.Password) < 8 || len(credentials.Password) > 128 {
		return User{}, ErrInvalid
	}
	displayName := strings.TrimSpace(credentials.DisplayName)
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}
	if len([]rune(displayName)) > 80 {
		return User{}, ErrInvalid
	}
	if _, _, err := s.repository.FindByEmail(ctx, workspace.DefaultWorkspaceID, email); err == nil {
		return User{}, ErrConflict
	} else if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(credentials.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("生成密码摘要: %w", err)
	}
	newUser := User{ID: newID(), WorkspaceID: workspace.DefaultWorkspaceID, Email: email, DisplayName: displayName, Role: "user", Status: "active"}
	if err := s.repository.Save(ctx, newUser, string(hash)); err != nil {
		if errors.Is(err, ErrConflict) {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	return newUser, nil
}

func (s *ServiceImpl) Authenticate(ctx context.Context, email, password string) (User, error) {
	normalized, err := normalizeEmail(email)
	if err != nil || password == "" {
		return User{}, ErrBadSecret
	}
	user, hash, err := s.repository.FindByEmail(ctx, workspace.DefaultWorkspaceID, normalized)
	if errors.Is(err, ErrNotFound) {
		return User{}, ErrBadSecret
	}
	if err != nil {
		return User{}, err
	}
	if user.Status != "active" {
		return User{}, ErrDisabled
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, ErrBadSecret
	}
	return user, nil
}

func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 320 {
		return "", ErrInvalid
	}
	return email, nil
}

func newID() string {
	return "usr_" + uuid.NewString()
}
