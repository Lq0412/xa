package definition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"linq-bs/server/internal/framework/auth"
)

// ServiceImpl 负责 Agent 配置校验、版本生成和归属判定。
type ServiceImpl struct{ repository Repository }

func NewService(repository Repository) *ServiceImpl { return &ServiceImpl{repository: repository} }

func (s *ServiceImpl) Save(ctx context.Context, actor auth.Principal, input SaveInput) (Saved, error) {
	if err := s.Validate(ctx, actor, input); err != nil {
		return Saved{}, err
	}
	now := time.Now().UTC()
	if input.AgentID == "" {
		definition := AgentDefinition{
			ID: "agt_" + uuid.NewString(), WorkspaceID: actor.WorkspaceID, OwnerID: actor.UserID,
			OwnerType: ownerType(actor), Name: strings.TrimSpace(input.Name), Avatar: strings.TrimSpace(input.Avatar),
			Description: strings.TrimSpace(input.Description), Tags: append([]string(nil), input.Tags...),
			Visibility: input.Visibility, Status: StatusActive, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		}
		version, err := newVersion(definition, input.Configuration, actor.UserID, now, 1)
		if err != nil {
			return Saved{}, err
		}
		saved := Saved{Definition: definition, Version: version}
		if err := s.repository.SaveRevision(ctx, actor, saved, ""); err != nil {
			return Saved{}, err
		}
		definition.CurrentVersionID = version.ID
		saved.Definition = definition
		return saved, nil
	}

	current, err := s.repository.Find(ctx, actor, input.AgentID)
	if err != nil {
		return Saved{}, err
	}
	if err := checkManage(actor, current); err != nil {
		return Saved{}, err
	}
	currentVersion, err := s.repository.FindVersion(ctx, actor, current.ID, current.CurrentVersionID)
	if err != nil {
		return Saved{}, err
	}
	definition := current
	definition.Name = strings.TrimSpace(input.Name)
	definition.Avatar = strings.TrimSpace(input.Avatar)
	definition.Description = strings.TrimSpace(input.Description)
	definition.Tags = append([]string(nil), input.Tags...)
	definition.Visibility = input.Visibility
	definition.UpdatedAt = now
	version, err := newVersion(definition, input.Configuration, actor.UserID, now, currentVersion.VersionNumber+1)
	if err != nil {
		return Saved{}, err
	}
	if input.ExpectedVersionID == "" {
		input.ExpectedVersionID = current.CurrentVersionID
	}
	if err := s.repository.SaveRevision(ctx, actor, Saved{Definition: definition, Version: version}, input.ExpectedVersionID); err != nil {
		return Saved{}, err
	}
	definition.CurrentVersionID = version.ID
	return Saved{Definition: definition, Version: version}, nil
}

func (s *ServiceImpl) Validate(_ context.Context, actor auth.Principal, input SaveInput) error {
	if actor.UserID == "" || actor.WorkspaceID == "" {
		return auth.ErrUnauthenticated
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 120 || len([]rune(strings.TrimSpace(input.Description))) > 2000 {
		return ErrInvalid
	}
	switch input.Visibility {
	case VisibilityPrivate:
		// 私有 Agent 可以由普通用户或管理员创建，归属始终由当前主体决定。
	case VisibilityPublic:
		if !auth.HasRole(actor, "admin") {
			return ErrForbidden
		}
	default:
		return ErrInvalid
	}
	for _, slot := range input.Configuration.Prompts {
		if len([]rune(slot.Content)) > 20000 || len([]rune(slot.Name)) > 100 {
			return ErrInvalid
		}
	}
	return nil
}

func (s *ServiceImpl) Get(ctx context.Context, actor auth.Principal, agentID string) (AgentDefinition, error) {
	return s.repository.Find(ctx, actor, agentID)
}

func (s *ServiceImpl) GetVersion(ctx context.Context, actor auth.Principal, agentID, versionID string) (AgentVersion, error) {
	return s.repository.FindVersion(ctx, actor, agentID, versionID)
}

func (s *ServiceImpl) List(ctx context.Context, actor auth.Principal) ([]AgentDefinition, error) {
	return s.repository.List(ctx, actor)
}

func (s *ServiceImpl) Enable(ctx context.Context, actor auth.Principal, agentID string) error {
	return s.repository.SetStatus(ctx, actor, agentID, StatusActive)
}

func (s *ServiceImpl) Disable(ctx context.Context, actor auth.Principal, agentID string) error {
	return s.repository.SetStatus(ctx, actor, agentID, StatusOffline)
}

func (s *ServiceImpl) Delete(ctx context.Context, actor auth.Principal, agentID string) error {
	return s.repository.SetStatus(ctx, actor, agentID, StatusDeleted)
}

func newVersion(definition AgentDefinition, configuration Configuration, actorID string, now time.Time, number int) (AgentVersion, error) {
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return AgentVersion{}, fmt.Errorf("编码 Agent 配置失败: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return AgentVersion{
		ID: "agtv_" + uuid.NewString(), WorkspaceID: definition.WorkspaceID, AgentID: definition.ID,
		VersionNumber: number, Configuration: configuration, ConfigChecksum: hex.EncodeToString(hash[:]),
		CreatedBy: actorID, CreatedAt: now,
	}, nil
}

func ownerType(actor auth.Principal) string {
	if auth.HasRole(actor, "admin") {
		return "system"
	}
	return "user"
}

func checkManage(actor auth.Principal, definition AgentDefinition) error {
	if definition.WorkspaceID != actor.WorkspaceID || (definition.OwnerType == "user" && definition.OwnerID != actor.UserID) || (definition.OwnerType == "system" && !auth.HasRole(actor, "admin")) {
		return ErrForbidden
	}
	if definition.Status == StatusDeleted {
		return ErrNotFound
	}
	return nil
}

var _ Service = (*ServiceImpl)(nil)
