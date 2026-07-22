package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/tracing"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const (
	userContextKey contextKey = "user"

	// SessionDuration is how long auth tokens last.
	SessionDuration = 30 * 24 * time.Hour // 30 days
)

// AuthUser is the authenticated user stored in request context.
type AuthUser struct {
	ID                     string
	Username               string
	DisplayName            string
	IsAdmin                bool
	IsCampChief            bool
	SubcampID              *string
	PasswordChangeRequired bool
}

// UserFromContext extracts the authenticated user from context.
func UserFromContext(ctx context.Context) *AuthUser {
	u, _ := ctx.Value(userContextKey).(*AuthUser)
	return u
}

// Middleware returns HTTP middleware that validates session tokens.
// It checks the Authorization header (Bearer token) or the session_token cookie.
func Middleware(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()

			session, err := db.GetUserSession(ctx, token)
			if err != nil {
				tracing.RecordError(ctx, err)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				return
			}
			if session == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			user, err := db.GetUserByID(ctx, session.UserID)
			if err != nil || user == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			authUser := &AuthUser{
				ID:                     user.ID,
				Username:               user.Username,
				DisplayName:            user.DisplayName,
				IsAdmin:                user.IsAdmin,
				IsCampChief:            user.IsCampChief,
				SubcampID:              user.SubcampID,
				PasswordChangeRequired: user.PasswordChangeRequired,
			}

			// Add user attributes to the trace span
			tracing.AddUserAttrs(ctx, authUser.ID, authUser.DisplayName)

			ctx = context.WithValue(ctx, userContextKey, authUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin returns middleware that checks the user is an admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil || !user.IsAdmin {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HashPassword generates a bcrypt hash for a password.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compares a password against a bcrypt hash.
func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func extractToken(r *http.Request) string {
	// Check Authorization header first
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// Fall back to cookie (also used for WebSocket upgrade requests)
	cookie, err := r.Cookie("session_token")
	if err == nil {
		return cookie.Value
	}

	return ""
}
