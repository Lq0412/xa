package user

import (
	"context"
	"errors"
	"testing"
)

type memoryRepository struct {
	users  map[string]User
	hashes map[string]string
}

func (r *memoryRepository) Find(_ context.Context, userID string) (User, error) {
	for _, current := range r.users {
		if current.ID == userID {
			return current, nil
		}
	}
	return User{}, ErrNotFound
}

func (r *memoryRepository) FindByEmail(_ context.Context, workspaceID, email string) (User, string, error) {
	for _, current := range r.users {
		if current.WorkspaceID == workspaceID && current.Email == email {
			return current, r.hashes[current.ID], nil
		}
	}
	return User{}, "", ErrNotFound
}

func (r *memoryRepository) Save(_ context.Context, current User, hash string) error {
	if _, _, err := r.FindByEmail(context.Background(), current.WorkspaceID, current.Email); err == nil {
		return ErrConflict
	}
	r.users[current.ID] = current
	r.hashes[current.ID] = hash
	return nil
}

func TestServiceRegisterAndAuthenticate(t *testing.T) {
	repository := &memoryRepository{users: make(map[string]User), hashes: make(map[string]string)}
	service := NewService(repository)
	created, err := service.Register(context.Background(), Credentials{Email: " User@Example.com ", Password: "password-123", DisplayName: "测试用户"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "user@example.com" || created.Role != "user" {
		t.Fatalf("注册资料不正确: %+v", created)
	}
	if repository.hashes[created.ID] == "password-123" {
		t.Fatal("密码被明文保存")
	}
	if _, err := service.Authenticate(context.Background(), created.Email, "password-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), created.Email, "wrong-password"); !errors.Is(err, ErrBadSecret) {
		t.Fatalf("错误密码未被拒绝: %v", err)
	}
	if _, err := service.Register(context.Background(), Credentials{Email: created.Email, Password: "password-123"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("重复邮箱未被拒绝: %v", err)
	}
}

func TestServiceRejectsInvalidCredentials(t *testing.T) {
	service := NewService(&memoryRepository{users: make(map[string]User), hashes: make(map[string]string)})
	for _, credentials := range []Credentials{{Email: "not-an-email", Password: "password-123"}, {Email: "a@example.com", Password: "short"}} {
		if _, err := service.Register(context.Background(), credentials); !errors.Is(err, ErrInvalid) {
			t.Fatalf("无效凭证未被拒绝: %v", err)
		}
	}
}
