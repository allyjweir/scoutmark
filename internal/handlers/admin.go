package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
)

type adminUserJSON struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	IsAdmin     bool    `json:"is_admin"`
	SubcampID   *string `json:"subcamp_id"`
	SubcampName *string `json:"subcamp_name"`
}

func adminUserResponse(user database.AdminUserRow) adminUserJSON {
	return adminUserJSON{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		SubcampID:   user.SubcampID,
		SubcampName: user.SubcampName,
	}
}

// ListAdminUsers handles GET /api/admin/users.
func (h *AuthHandler) ListAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.db.ListUsers(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not fetch users")
		return
	}
	result := make([]adminUserJSON, 0, len(users))
	for _, user := range users {
		result = append(result, adminUserResponse(user))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

// ListAdminSubcamps handles GET /api/admin/subcamps.
func (h *AuthHandler) ListAdminSubcamps(w http.ResponseWriter, r *http.Request) {
	subcamps, err := h.db.ListSubcamps(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not fetch subcamps")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subcamps": subcamps})
}

// CreateAdminUser handles POST /api/admin/users.
func (h *AuthHandler) CreateAdminUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		SubcampID   string `json:"subcamp_id"`
		IsAdmin     bool   `json:"is_admin"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.Username == "" || req.DisplayName == "" || req.SubcampID == "" {
		writeError(w, r, http.StatusBadRequest, "username, display name, and subcamp are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, r, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not secure password")
		return
	}
	user, err := h.db.CreateUser(r.Context(), req.Username, hash, req.DisplayName, req.SubcampID, req.IsAdmin)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "could not create user; username and subcamp must be unique and valid")
		return
	}
	writeJSON(w, http.StatusCreated, adminUserResponse(*user))
}

// ResetAdminUserPassword handles PUT /api/admin/users/{user_id}/password.
func (h *AuthHandler) ResetAdminUserPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, r, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not secure password")
		return
	}
	if err := h.db.AdminSetUserPassword(r.Context(), r.PathValue("user_id"), hash); err != nil {
		if errors.Is(err, database.ErrUserNotFound) {
			writeError(w, r, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "could not reset password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "password reset"})
}

func sessionResponse(s *database.SessionDetailRow) sessionJSON {
	return sessionJSON{
		ID:                s.ID,
		EventID:           s.EventID,
		EventName:         s.EventName,
		TemplateID:        s.TemplateID,
		Name:              s.Name,
		RoundType:         s.RoundType,
		StartsAt:          s.StartsAt.UTC().Format(time.RFC3339),
		EndsAt:            s.EndsAt.UTC().Format(time.RFC3339),
		Status:            s.ComputeStatus(),
		LockedAt:          formatOptionalTime(s.LockedAt),
		LockedBy:          s.LockedBy,
		LockedByName:      s.LockedByName,
		CreatedAt:         s.CreatedAt.UTC().Format(time.RFC3339),
		PreviousSessionID: s.PreviousSessionID,
		AwardBestPatrol:   s.AwardBestPatrol,
		AwardMostImproved: s.AwardMostImproved,
	}
}

// UpdateAdminSession handles PUT /api/admin/sessions/{session_id}.
func (h *SessionHandler) UpdateAdminSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StartsAt string `json:"starts_at"`
		EndsAt   string `json:"ends_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "starts_at must be an ISO 8601 timestamp")
		return
	}
	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "ends_at must be an ISO 8601 timestamp")
		return
	}
	if !endsAt.After(startsAt) {
		writeError(w, r, http.StatusBadRequest, "end time must be after start time")
		return
	}
	if err := h.db.UpdateSessionTimes(r.Context(), r.PathValue("session_id"), startsAt, endsAt); err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	session, err := h.db.GetSession(r.Context(), r.PathValue("session_id"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not fetch updated session")
		return
	}
	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(r.Context(), session.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": sessionResponse(session)})
}

// ListAdminSessionSubcamps handles GET /api/admin/sessions/{session_id}/subcamps.
func (h *SessionHandler) ListAdminSessionSubcamps(w http.ResponseWriter, r *http.Request) {
	subcamps, err := h.db.ListSessionSubcamps(r.Context(), r.PathValue("session_id"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not fetch session subcamps")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subcamps": subcamps})
}

// LockAdminSessionSubcamp handles POST /api/admin/sessions/{session_id}/subcamps/{subcamp_id}/lock.
func (h *SessionHandler) LockAdminSessionSubcamp(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if err := h.db.LockSessionSubcamp(r.Context(), r.PathValue("session_id"), r.PathValue("subcamp_id"), user.ID); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(r.Context(), r.PathValue("session_id"))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// UnlockAdminSessionSubcamp handles POST /api/admin/sessions/{session_id}/subcamps/{subcamp_id}/unlock.
func (h *SessionHandler) UnlockAdminSessionSubcamp(w http.ResponseWriter, r *http.Request) {
	if err := h.db.UnlockSessionSubcamp(r.Context(), r.PathValue("session_id"), r.PathValue("subcamp_id")); err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not unlock subcamp")
		return
	}
	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(r.Context(), r.PathValue("session_id"))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// UpdateAdminPatrolScores handles PUT /api/admin/sessions/{session_id}/patrols/{patrol_id}/scores.
func (h *SessionHandler) UpdateAdminPatrolScores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")
	var req struct {
		Scores    map[string]int `json:"scores"`
		Confirmed bool           `json:"confirmed"`
	}
	if err := readJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.Confirmed {
		writeError(w, r, http.StatusBadRequest, "confirm score correction before saving")
		return
	}
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	inSession, err := h.db.SessionHasPatrol(ctx, sessionID, patrolID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not verify session patrol")
		return
	}
	if !inSession {
		writeError(w, r, http.StatusForbidden, "patrol is not in this session")
		return
	}
	criteria, err := h.db.GetTemplateCriteria(ctx, session.TemplateID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not validate scores")
		return
	}
	valid := make(map[string]database.CriterionRow, len(criteria))
	for _, criterion := range criteria {
		valid[criterion.ID] = criterion
	}
	for id, value := range req.Scores {
		criterion, ok := valid[id]
		if !ok || value < criterion.MinValue || value > criterion.MaxValue {
			writeError(w, r, http.StatusBadRequest, "score is outside its criterion range")
			return
		}
	}
	if len(req.Scores) == 0 {
		writeError(w, r, http.StatusBadRequest, "at least one score is required")
		return
	}
	// Admin corrections deliberately bypass session and subcamp locks so historical scores can be fixed.
	if err := h.db.UpdateSubmissionScores(ctx, sessionID, patrolID, auth.UserFromContext(ctx).ID, req.Scores); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(ctx, sessionID)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
