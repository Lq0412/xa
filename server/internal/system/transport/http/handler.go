package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"linq-bs/server/internal/framework/auth"
	frameworkhttp "linq-bs/server/internal/framework/http"
	"linq-bs/server/internal/system/user"
)

// Handler 暴露注册、登录和当前用户查询，凭证校验由 system/user 完成。
type Handler struct {
	Users    user.Service
	Sessions auth.SessionStore
}

type credentialsRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var input credentialsRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	created, err := h.Users.Register(r.Context(), user.Credentials{Email: input.Email, Password: input.Password, DisplayName: input.DisplayName})
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusCreated, userResponse(created))
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var input credentialsRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	loggedIn, err := h.Users.Authenticate(r.Context(), input.Email, input.Password)
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	token, err := h.Sessions.Create(r.Context(), auth.Principal{UserID: loggedIn.ID, WorkspaceID: loggedIn.WorkspaceID, Roles: []string{loggedIn.Role}})
	if err != nil {
		frameworkhttp.WriteError(w, r, http.StatusServiceUnavailable, "session_unavailable", err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusOK, map[string]any{"token": token, "user": userResponse(loggedIn)})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token, ok := auth.BearerToken(r.Header.Get("Authorization"))
	if !ok {
		frameworkhttp.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", auth.ErrUnauthenticated)
		return
	}
	if err := h.Sessions.Delete(r.Context(), token); err != nil {
		frameworkhttp.WriteError(w, r, http.StatusServiceUnavailable, "session_unavailable", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := frameworkhttp.Principal(r.Context())
	if !ok {
		frameworkhttp.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", auth.ErrUnauthenticated)
		return
	}
	current, err := h.Users.Get(r.Context(), principal.UserID)
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	frameworkhttp.WriteJSON(w, http.StatusOK, userResponse(current))
}

func userResponse(current user.User) map[string]string {
	return map[string]string{"id": current.ID, "email": current.Email, "display_name": current.DisplayName, "role": current.Role, "workspace_id": current.WorkspaceID}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		frameworkhttp.WriteError(w, r, http.StatusBadRequest, "invalid_json", err)
		return false
	}
	return true
}

func writeUserError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, user.ErrInvalid):
		frameworkhttp.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_user", err)
	case errors.Is(err, user.ErrConflict):
		frameworkhttp.WriteError(w, r, http.StatusConflict, "user_exists", err)
	case errors.Is(err, user.ErrBadSecret):
		frameworkhttp.WriteError(w, r, http.StatusUnauthorized, "invalid_credentials", err)
	case errors.Is(err, user.ErrDisabled):
		frameworkhttp.WriteError(w, r, http.StatusForbidden, "user_disabled", err)
	default:
		frameworkhttp.WriteError(w, r, http.StatusServiceUnavailable, "user_unavailable", err)
	}
}
