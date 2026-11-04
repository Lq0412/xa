package definition

import (
	"context"
	"errors"
	"testing"

	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/workspace"
)

type memoryRepository struct {
	definitions map[string]AgentDefinition
	versions    map[string]AgentVersion
	conflicts   bool
}

func (r *memoryRepository) Find(_ context.Context, _ auth.Principal, agentID string) (AgentDefinition, error) {
	definition, ok := r.definitions[agentID]
	if !ok {
		return AgentDefinition{}, ErrNotFound
	}
	return definition, nil
}

func (r *memoryRepository) List(_ context.Context, _ auth.Principal) ([]AgentDefinition, error) {
	result := make([]AgentDefinition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		result = append(result, definition)
	}
	return result, nil
}

func (r *memoryRepository) FindVersion(_ context.Context, _ auth.Principal, _, versionID string) (AgentVersion, error) {
	version, ok := r.versions[versionID]
	if !ok {
		return AgentVersion{}, ErrNotFound
	}
	return version, nil
}

func (r *memoryRepository) SaveRevision(_ context.Context, _ auth.Principal, saved Saved, expectedVersionID string) error {
	if r.conflicts {
		return ErrConflict
	}
	if saved.Definition.CurrentVersionID != "" {
		current := r.definitions[saved.Definition.ID]
		if current.CurrentVersionID != expectedVersionID {
			return ErrConflict
		}
	}
	saved.Definition.CurrentVersionID = saved.Version.ID
	r.definitions[saved.Definition.ID] = saved.Definition
	r.versions[saved.Version.ID] = saved.Version
	return nil
}

func (r *memoryRepository) SetStatus(_ context.Context, _ auth.Principal, agentID string, status Status) error {
	definition, ok := r.definitions[agentID]
	if !ok {
		return ErrNotFound
	}
	definition.Status = status
	r.definitions[agentID] = definition
	return nil
}

func TestServiceSavesPrivateAgentAndRejectsPublicUser(t *testing.T) {
	repository := &memoryRepository{definitions: map[string]AgentDefinition{}, versions: map[string]AgentVersion{}}
	service := NewService(repository)
	actor := auth.Principal{UserID: "user-1", WorkspaceID: workspace.DefaultWorkspaceID, Roles: []string{"user"}}
	saved, err := service.Save(context.Background(), actor, SaveInput{Name: "我的教练", Visibility: VisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version.ID == "" || saved.Definition.CurrentVersionID != saved.Version.ID {
		t.Fatalf("版本指针未建立: %+v", saved)
	}
	if _, err := service.Save(context.Background(), actor, SaveInput{Name: "公共教练", Visibility: VisibilityPublic}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("普通用户创建公共 Agent 未被拒绝: %v", err)
	}
}

func TestServiceVersionConflictDoesNotOverwrite(t *testing.T) {
	repository := &memoryRepository{definitions: map[string]AgentDefinition{}, versions: map[string]AgentVersion{}, conflicts: true}
	service := NewService(repository)
	actor := auth.Principal{UserID: "admin-1", WorkspaceID: workspace.DefaultWorkspaceID, Roles: []string{"admin"}}
	if _, err := service.Save(context.Background(), actor, SaveInput{Name: "公共教练", Visibility: VisibilityPublic}); !errors.Is(err, ErrConflict) {
		t.Fatalf("配置冲突未返回: %v", err)
	}
	if len(repository.definitions) != 0 {
		t.Fatal("冲突保存产生了半成品")
	}
}
