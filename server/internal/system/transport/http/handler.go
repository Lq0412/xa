package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/system/user"
)

// Handler 暴露账号和会话接口，凭证校验由 system/user 完成。
type Handler struct {
	Users    user.Service
	Sessions auth.SessionStore
}

type credentialsRequest struct {
	Email       string `json:"email" doc:"账号邮箱"`
	Password    string `json:"password" doc:"账号密码"`
	DisplayName string `json:"display_name,omitempty" doc:"注册时使用的显示名称"`
}

type credentialsInput struct {
	Body credentialsRequest
}

type logoutInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
}

type userResponse struct {
	ID          string `json:"id" doc:"用户 ID"`
	Email       string `json:"email" doc:"账号邮箱"`
	DisplayName string `json:"display_name" doc:"显示名称"`
	Role        string `json:"role" doc:"用户角色"`
	WorkspaceID string `json:"workspace_id" doc:"工作空间 ID"`
}

type userOutput struct {
	Body userResponse
}

type loginResponse struct {
	Token string       `json:"token" doc:"会话 Bearer 令牌"`
	User  userResponse `json:"user" doc:"当前用户"`
}

type loginOutput struct {
	Body loginResponse
}

// RegisterRoutes 注册账号与会话接口，并为受保护的接口挂载现有认证。
func (h *Handler) RegisterRoutes(api huma.API, resolver auth.Resolver) {
	const tag = "认证"
	const prefix = "/api/v1/auth/"
	const maxBodyBytes = 1 << 20
	protect := func(next http.Handler) http.Handler { return frameworkhttp.Authenticate(resolver, next) }
	huma.Register(api, frameworkhttp.Operation(api, tag, "register", http.MethodPost, prefix+"register", "注册账号", http.StatusCreated, maxBodyBytes, map[int]string{
		400: "请求 JSON 无效（invalid_json）", 409: "用户已存在（user_exists）", 422: "用户信息无效（invalid_user）", 503: "用户服务不可用（user_unavailable）",
	}), h.register)
	huma.Register(api, frameworkhttp.Operation(api, tag, "login", http.MethodPost, prefix+"login", "登录账号", http.StatusOK, maxBodyBytes, map[int]string{
		400: "请求 JSON 无效（invalid_json）", 401: "凭证无效（invalid_credentials）", 403: "用户已停用（user_disabled）", 503: "用户或会话服务不可用（user_unavailable、session_unavailable）",
	}), h.login)
	logout := frameworkhttp.Operation(api, tag, "logout", http.MethodPost, prefix+"logout", "注销会话", http.StatusNoContent, 0, map[int]string{
		401: "会话无效（unauthenticated）", 503: "认证或会话服务不可用（unauthenticated、session_unavailable）",
	})
	huma.Register(api, frameworkhttp.OperationAuth(logout, protect, false), h.logout)
	me := frameworkhttp.Operation(api, tag, "getMe", http.MethodGet, prefix+"me", "获取当前用户", http.StatusOK, 0, map[int]string{
		401: "会话无效（unauthenticated）", 503: "认证或用户服务不可用（unauthenticated、user_unavailable）",
	})
	huma.Register(api, frameworkhttp.OperationAuth(me, protect, false), h.me)
}

func (h *Handler) register(ctx context.Context, input *credentialsInput) (*userOutput, error) {
	credentials := input.Body
	created, err := h.Users.Register(ctx, user.Credentials{Email: credentials.Email, Password: credentials.Password, DisplayName: credentials.DisplayName})
	if err != nil {
		return nil, userError(ctx, err)
	}
	return &userOutput{Body: toUserResponse(created)}, nil
}

func (h *Handler) login(ctx context.Context, input *credentialsInput) (*loginOutput, error) {
	loggedIn, err := h.Users.Authenticate(ctx, input.Body.Email, input.Body.Password)
	if err != nil {
		return nil, userError(ctx, err)
	}
	token, err := h.Sessions.Create(ctx, auth.Principal{UserID: loggedIn.ID, WorkspaceID: loggedIn.WorkspaceID, Roles: []string{loggedIn.Role}})
	if err != nil {
		return nil, frameworkhttp.APIError(ctx, http.StatusServiceUnavailable, "session_unavailable", err)
	}
	return &loginOutput{Body: loginResponse{Token: token, User: toUserResponse(loggedIn)}}, nil
}

func (h *Handler) logout(ctx context.Context, input *logoutInput) (*frameworkhttp.NoContent, error) {
	token, ok := auth.BearerToken(input.Authorization)
	if !ok {
		return nil, frameworkhttp.APIError(ctx, http.StatusUnauthorized, "unauthenticated", auth.ErrUnauthenticated)
	}
	if err := h.Sessions.Delete(ctx, token); err != nil {
		return nil, frameworkhttp.APIError(ctx, http.StatusServiceUnavailable, "session_unavailable", err)
	}
	return nil, nil
}

func (h *Handler) me(ctx context.Context, _ *frameworkhttp.EmptyInput) (*userOutput, error) {
	principal, ok := frameworkhttp.Principal(ctx)
	if !ok {
		return nil, frameworkhttp.APIError(ctx, http.StatusUnauthorized, "unauthenticated", auth.ErrUnauthenticated)
	}
	current, err := h.Users.Get(ctx, principal.UserID)
	if err != nil {
		return nil, userError(ctx, err)
	}
	return &userOutput{Body: toUserResponse(current)}, nil
}

func toUserResponse(current user.User) userResponse {
	return userResponse{ID: current.ID, Email: current.Email, DisplayName: current.DisplayName, Role: current.Role, WorkspaceID: current.WorkspaceID}
}

func userError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, user.ErrInvalid):
		return frameworkhttp.APIError(ctx, http.StatusUnprocessableEntity, "invalid_user", err)
	case errors.Is(err, user.ErrConflict):
		return frameworkhttp.APIError(ctx, http.StatusConflict, "user_exists", err)
	case errors.Is(err, user.ErrBadSecret):
		return frameworkhttp.APIError(ctx, http.StatusUnauthorized, "invalid_credentials", err)
	case errors.Is(err, user.ErrDisabled):
		return frameworkhttp.APIError(ctx, http.StatusForbidden, "user_disabled", err)
	default:
		return frameworkhttp.APIError(ctx, http.StatusServiceUnavailable, "user_unavailable", err)
	}
}
