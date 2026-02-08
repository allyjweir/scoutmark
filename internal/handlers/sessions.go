package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/tracing"
)

// SessionBroadcaster is the interface for broadcasting session progress updates.
type SessionBroadcaster interface {
	BroadcastSessionProgress(ctx context.Context, sessionID string)
}

// SessionHandler handles session-related endpoints.
type SessionHandler struct {
	db          *database.DB
	broadcaster SessionBroadcaster
}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler(db *database.DB, broadcaster SessionBroadcaster) *SessionHandler {
	return &SessionHandler{db: db, broadcaster: broadcaster}
}

type sessionJSON struct {
	ID            string          `json:"id"`
	EventID       string          `json:"event_id"`
	EventName     string          `json:"event_name"`
	TemplateID    string          `json:"template_id"`
	Name          string          `json:"name"`
	StartsAt      string          `json:"starts_at"`
	EndsAt        string          `json:"ends_at"`
	Status        string          `json:"status"`
	CreatedAt     string          `json:"created_at"`
	Criteria      []criterionJSON `json:"criteria,omitempty"`
	UserFinalised bool            `json:"user_finalised"`
}

type criterionJSON struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	MinValue    int    `json:"min_value"`
	MaxValue    int    `json:"max_value"`
	SortOrder   int    `json:"sort_order"`
}

type patrolJSON struct {
	PatrolID  string `json:"patrol_id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type submissionJSON struct {
	ID          string                `json:"id"`
	PatrolID    string                `json:"patrol_id"`
	PatrolName  string                `json:"patrol_name"`
	Locked      bool                  `json:"locked"`
	SubmittedAt string                `json:"submitted_at"`
	Scores      []submissionScoreJSON `json:"scores,omitempty"`
}

type submissionScoreJSON struct {
	CriterionID string `json:"criterion_id"`
	Value       int    `json:"value"`
}

type draftJSON struct {
	ID        string           `json:"id"`
	PatrolID  string           `json:"patrol_id"`
	Scores    []draftScoreJSON `json:"scores"`
	UpdatedAt string           `json:"updated_at"`
}

type draftScoreJSON struct {
	CriterionID string `json:"criterion_id"`
	Value       int    `json:"value"`
}

// ListSessions handles GET /api/sessions
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.list_sessions")
	defer span.End()

	user := auth.UserFromContext(ctx)

	// Parse optional status filter from query params
	statusParam := r.URL.Query()["status"]

	sessions, err := h.db.ListSessions(ctx, statusParam)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch sessions")
		return
	}

	// Look up which sessions this user has fully finalised
	finalisedSet, err := h.db.GetUserFinalisedSessionIDs(ctx, user.ID)
	if err != nil {
		tracing.RecordError(ctx, err)
		// Non-fatal: just proceed without finalised info
		finalisedSet = map[string]bool{}
	}

	span.SetAttributes(attribute.Int("sessions.count", len(sessions)))

	result := lo.Map(sessions, func(s database.SessionDetailRow, _ int) sessionJSON {
		return sessionJSON{
			ID:            s.ID,
			EventID:       s.EventID,
			EventName:     s.EventName,
			TemplateID:    s.TemplateID,
			Name:          s.Name,
			StartsAt:      s.StartsAt.Format("2006-01-02T15:04:05Z"),
			EndsAt:        s.EndsAt.Format("2006-01-02T15:04:05Z"),
			Status:        s.ComputeStatus(),
			CreatedAt:     s.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UserFinalised: finalisedSet[s.ID],
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"sessions": result})
}

// GetSession handles GET /api/sessions/{id}
func (h *SessionHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_session")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	user := auth.UserFromContext(ctx)

	// Fetch session
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}

	// Fetch criteria for the session's template
	criteria, err := h.db.GetTemplateCriteria(ctx, session.TemplateID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch criteria")
		return
	}

	// Fetch user's patrols
	patrols, err := h.db.GetUserPatrols(ctx, user.ID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch patrols")
		return
	}

	// Fetch existing submissions
	submissions, err := h.db.GetSubmissionsForSession(ctx, user.ID, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch submissions")
		return
	}

	span.SetAttributes(
		attribute.Int("criteria.count", len(criteria)),
		attribute.Int("patrols.count", len(patrols)),
		attribute.Int("submissions.count", len(submissions)),
	)

	sessionResult := sessionJSON{
		ID:         session.ID,
		EventID:    session.EventID,
		EventName:  session.EventName,
		TemplateID: session.TemplateID,
		Name:       session.Name,
		StartsAt:   session.StartsAt.Format("2006-01-02T15:04:05Z"),
		EndsAt:     session.EndsAt.Format("2006-01-02T15:04:05Z"),
		Status:     session.ComputeStatus(),
		CreatedAt:  session.CreatedAt.Format("2006-01-02T15:04:05Z"),
		Criteria: lo.Map(criteria, func(c database.CriterionRow, _ int) criterionJSON {
			return criterionJSON{
				ID:          c.ID,
				Title:       c.Title,
				Description: c.Description,
				MinValue:    c.MinValue,
				MaxValue:    c.MaxValue,
				SortOrder:   c.SortOrder,
			}
		}),
	}

	patrolResult := lo.Map(patrols, func(p database.UserPatrolRow, _ int) patrolJSON {
		return patrolJSON{PatrolID: p.PatrolID, Name: p.Name, SortOrder: p.SortOrder}
	})

	submissionResult := lo.Map(submissions, func(s database.SubmissionRow, _ int) submissionJSON {
		return submissionJSON{
			ID:          s.ID,
			PatrolID:    s.PatrolID,
			PatrolName:  s.PatrolName,
			Locked:      s.Locked,
			SubmittedAt: s.SubmittedAt.Format("2006-01-02T15:04:05Z"),
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"session":     sessionResult,
		"patrols":     patrolResult,
		"submissions": submissionResult,
	})
}

// GetDraft handles GET /api/sessions/{session_id}/patrols/{patrol_id}/draft
func (h *SessionHandler) GetDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_draft")
	defer span.End()
	tracing.AddSessionAttrs(ctx, sessionID, patrolID)

	user := auth.UserFromContext(ctx)

	// IDOR check: verify user owns this patrol
	owns, err := h.db.UserOwnsPatrol(ctx, user.ID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}

	draft, err := h.db.GetDraft(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch draft")
		return
	}

	if draft == nil {
		writeJSON(w, http.StatusOK, map[string]any{"draft": nil})
		return
	}

	scores, err := h.db.GetDraftScores(ctx, draft.ID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch draft scores")
		return
	}

	span.SetAttributes(attribute.Int("draft.scores_count", len(scores)))

	result := draftJSON{
		ID:        draft.ID,
		PatrolID:  draft.PatrolID,
		UpdatedAt: draft.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		Scores: lo.Map(scores, func(s database.DraftScoreRow, _ int) draftScoreJSON {
			return draftScoreJSON{CriterionID: s.CriterionID, Value: s.Value}
		}),
	}

	writeJSON(w, http.StatusOK, map[string]any{"draft": result})
}

// SubmitScores handles POST /api/sessions/{session_id}/patrols/{patrol_id}/submit
func (h *SessionHandler) SubmitScores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")

	_, span := tracing.Tracer().Start(ctx, "handler.submit_scores")
	defer span.End()
	tracing.AddSessionAttrs(ctx, sessionID, patrolID)

	user := auth.UserFromContext(ctx)

	// Verify session is active
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	if session.ComputeStatus() != "ACTIVE" {
		span.SetAttributes(attribute.String("session.status", session.ComputeStatus()))
		writeError(w, r, http.StatusBadRequest, "session is not active")
		return
	}

	// IDOR check: verify user owns this patrol
	owns, err := h.db.UserOwnsPatrol(ctx, user.ID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}

	var req struct {
		Scores map[string]int `json:"scores"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate score values against criteria bounds
	criteria, err := h.db.GetTemplateCriteria(ctx, session.TemplateID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch criteria")
		return
	}
	criteriaByID := make(map[string]database.CriterionRow, len(criteria))
	for _, c := range criteria {
		criteriaByID[c.ID] = c
	}
	for criterionID, value := range req.Scores {
		c, ok := criteriaByID[criterionID]
		if !ok {
			writeError(w, r, http.StatusBadRequest, "invalid criterion ID: "+criterionID)
			return
		}
		if value < c.MinValue || value > c.MaxValue {
			writeError(w, r, http.StatusBadRequest,
				fmt.Sprintf("score for %q must be between %d and %d", c.Title, c.MinValue, c.MaxValue))
			return
		}
	}

	span.SetAttributes(attribute.Int("scores.count", len(req.Scores)))

	submission, err := h.db.CreateSubmission(ctx, user.ID, sessionID, patrolID, req.Scores)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not submit scores")
		return
	}

	span.SetAttributes(
		attribute.String("submission.id", submission.ID),
		attribute.Bool("submission.success", true),
	)

	writeJSON(w, http.StatusOK, submissionJSON{
		ID:          submission.ID,
		PatrolID:    submission.PatrolID,
		Locked:      submission.Locked,
		SubmittedAt: submission.SubmittedAt.Format("2006-01-02T15:04:05Z"),
	})

	// Broadcast updated progress to WebSocket subscribers
	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(ctx, sessionID)
	}
}

// UnlockSubmission handles POST /api/submissions/{id}/unlock (admin only)
func (h *SessionHandler) UnlockSubmission(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	submissionID := r.PathValue("id")

	_, span := tracing.Tracer().Start(ctx, "handler.unlock_submission")
	defer span.End()
	span.SetAttributes(attribute.String("submission.id", submissionID))

	submission, err := h.db.UnlockSubmission(ctx, submissionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not unlock submission")
		return
	}

	writeJSON(w, http.StatusOK, submissionJSON{
		ID:          submission.ID,
		PatrolID:    submission.PatrolID,
		PatrolName:  submission.PatrolName,
		Locked:      submission.Locked,
		SubmittedAt: submission.SubmittedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// ListSubmissions handles GET /api/sessions/{id}/submissions
func (h *SessionHandler) ListSubmissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.list_submissions")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	submissions, err := h.db.ListAllSubmissionsForSession(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch submissions")
		return
	}

	result := lo.Map(submissions, func(s database.SubmissionRow, _ int) submissionJSON {
		return submissionJSON{
			ID:          s.ID,
			PatrolID:    s.PatrolID,
			PatrolName:  s.PatrolName,
			Locked:      s.Locked,
			SubmittedAt: s.SubmittedAt.Format("2006-01-02T15:04:05Z"),
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"submissions": result})
}

// FinaliseSession handles POST /api/sessions/{session_id}/finalise
func (h *SessionHandler) FinaliseSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.finalise_session")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	user := auth.UserFromContext(ctx)

	// Verify session is active
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	if session.ComputeStatus() != "ACTIVE" {
		span.SetAttributes(attribute.String("session.status", session.ComputeStatus()))
		writeError(w, r, http.StatusBadRequest, "session is not active")
		return
	}

	newSubmissions, err := h.db.FinaliseSession(ctx, user.ID, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not finalise session")
		return
	}

	// Return all submissions for this session (including previously submitted)
	allSubmissions, err := h.db.GetSubmissionsForSession(ctx, user.ID, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch submissions")
		return
	}

	span.SetAttributes(
		attribute.Int("finalised.count", len(newSubmissions)),
		attribute.Int("total.submissions", len(allSubmissions)),
	)

	result := lo.Map(allSubmissions, func(s database.SubmissionRow, _ int) submissionJSON {
		return submissionJSON{
			ID:          s.ID,
			PatrolID:    s.PatrolID,
			PatrolName:  s.PatrolName,
			Locked:      s.Locked,
			SubmittedAt: s.SubmittedAt.Format("2006-01-02T15:04:05Z"),
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"submissions":     result,
		"finalised_count": len(newSubmissions),
	})

	// Broadcast updated progress to WebSocket subscribers
	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(ctx, sessionID)
	}
}

// ReviseSession handles POST /api/sessions/{session_id}/revise
// Converts all submissions back to drafts so the user can edit them.
func (h *SessionHandler) ReviseSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.revise_session")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	user := auth.UserFromContext(ctx)

	// Verify session is active — can only revise active sessions
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	if session.ComputeStatus() != "ACTIVE" {
		span.SetAttributes(attribute.String("session.status", session.ComputeStatus()))
		writeError(w, r, http.StatusBadRequest, "session is not active")
		return
	}

	if err := h.db.ReviseSession(ctx, user.ID, sessionID); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not revise session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GetSubmissionScores handles GET /api/sessions/{session_id}/patrols/{patrol_id}/scores
// Returns the submitted scores for a patrol (read-only view).
func (h *SessionHandler) GetSubmissionScores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_submission_scores")
	defer span.End()
	span.SetAttributes(
		attribute.String("session.id", sessionID),
		attribute.String("patrol.id", patrolID),
	)

	user := auth.UserFromContext(ctx)

	// IDOR check: verify user owns this patrol
	owns, err := h.db.UserOwnsPatrol(ctx, user.ID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}

	scores, err := h.db.GetSubmissionScoresByPatrol(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch scores")
		return
	}

	span.SetAttributes(attribute.Int("scores.count", len(scores)))

	result := lo.Map(scores, func(s database.SubmissionScoreRow, _ int) submissionScoreJSON {
		return submissionScoreJSON{
			CriterionID: s.CriterionID,
			Value:       s.Value,
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"scores": result})
}

// GetSessionProgress handles GET /api/admin/sessions/{session_id}/progress
// Returns scoring progress for all users in a session (admin only).
func (h *SessionHandler) GetSessionProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_session_progress")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	// Fetch session details
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}

	// Fetch progress rows
	progress, err := h.db.GetSessionProgress(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch progress")
		return
	}

	// Group by user
	type patrolProgress struct {
		PatrolID   string `json:"patrol_id"`
		PatrolName string `json:"patrol_name"`
		Status     string `json:"status"` // not_started, drafting, submitted
	}
	type userProgress struct {
		UserID      string           `json:"user_id"`
		DisplayName string           `json:"display_name"`
		Patrols     []patrolProgress `json:"patrols"`
	}

	userMap := make(map[string]*userProgress)
	var userOrder []string

	for _, row := range progress {
		up, exists := userMap[row.UserID]
		if !exists {
			up = &userProgress{
				UserID:      row.UserID,
				DisplayName: row.DisplayName,
			}
			userMap[row.UserID] = up
			userOrder = append(userOrder, row.UserID)
		}
		up.Patrols = append(up.Patrols, patrolProgress{
			PatrolID:   row.PatrolID,
			PatrolName: row.PatrolName,
			Status:     row.Status,
		})
	}

	// Preserve insertion order
	users := make([]userProgress, 0, len(userOrder))
	for _, id := range userOrder {
		users = append(users, *userMap[id])
	}

	span.SetAttributes(attribute.Int("users.count", len(users)))

	sessionResult := sessionJSON{
		ID:        session.ID,
		EventID:   session.EventID,
		EventName: session.EventName,
		Name:      session.Name,
		StartsAt:  session.StartsAt.Format("2006-01-02T15:04:05Z"),
		EndsAt:    session.EndsAt.Format("2006-01-02T15:04:05Z"),
		Status:    session.ComputeStatus(),
		CreatedAt: session.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session": sessionResult,
		"users":   users,
	})
}
