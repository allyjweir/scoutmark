package handlers

import (
	"net/http"
	"time"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	db *database.DB
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db *database.DB) *AuthHandler {
	return &AuthHandler{db: db}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	SessionToken             string   `json:"session_token"`
	User                     userJSON `json:"user"`
	PasswordChangeRequired   bool     `json:"password_change_required"`
}

type userJSON struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	IsAdmin     bool   `json:"is_admin"`
}

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.login")
	defer span.End()

	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	span.SetAttributes(attribute.String("auth.username", req.Username))

	user, err := h.db.GetUserByUsername(ctx, req.Username)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if user == nil || !auth.CheckPassword(req.Password, user.PasswordHash) {
		span.SetAttributes(attribute.Bool("auth.success", false))
		writeError(w, r, http.StatusUnauthorized, "invalid username or password")
		return
	}

	session, err := h.db.CreateUserSession(ctx, user.ID, auth.SessionDuration)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not create session")
		return
	}

	span.SetAttributes(
		attribute.Bool("auth.success", true),
		attribute.String("user.id", user.ID),
	)

	// Set cookie for browser-based auth
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(auth.SessionDuration),
	})

	writeJSON(w, http.StatusOK, loginResponse{
		SessionToken:           session.Token,
		PasswordChangeRequired: user.PasswordChangeRequired,
		User: userJSON{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
		},
	})
}

// Logout handles POST /api/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user := auth.UserFromContext(ctx)
	if user == nil {
		writeError(w, r, http.StatusUnauthorized, "not logged in")
		return
	}

	// Delete the session — try cookie first, then Authorization header
	cookie, err := r.Cookie("session_token")
	if err == nil {
		h.db.DeleteUserSession(ctx, cookie.Value)
	} else if token := r.Header.Get("Authorization"); len(token) > 7 {
		h.db.DeleteUserSession(ctx, token[7:]) // strip "Bearer "
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetCurrentUser handles GET /api/auth/me
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, r, http.StatusUnauthorized, "not logged in")
		return
	}

	trace.SpanFromContext(r.Context()).SetAttributes(
		attribute.String("user.id", user.ID),
	)

	writeJSON(w, http.StatusOK, userJSON{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
	})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword handles POST /api/auth/change-password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.change_password")
	defer span.End()

	user := auth.UserFromContext(ctx)
	if user == nil {
		writeError(w, r, http.StatusUnauthorized, "not logged in")
		return
	}

	var req changePasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NewPassword == "" {
		writeError(w, r, http.StatusBadRequest, "new password cannot be empty")
		return
	}

	// Get the current user to verify old password
	currentUser, err := h.db.GetUserByID(ctx, user.ID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if currentUser == nil {
		writeError(w, r, http.StatusUnauthorized, "user not found")
		return
	}

	// Verify the old password
	if !auth.CheckPassword(req.OldPassword, currentUser.PasswordHash) {
		span.SetAttributes(attribute.Bool("password.valid", false))
		writeError(w, r, http.StatusUnauthorized, "invalid old password")
		return
	}

	// Hash the new password
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not hash password")
		return
	}

	// Update the password in the database
	if err := h.db.UpdateUserPassword(ctx, user.ID, newHash); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not update password")
		return
	}

	span.SetAttributes(
		attribute.Bool("password.changed", true),
		attribute.String("user.id", user.ID),
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}
