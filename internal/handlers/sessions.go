package handlers

import (
	"net/http"

	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/tracing"
)

// SessionHandler handles session-related endpoints.
type SessionHandler struct {
	db *database.DB
}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler(db *database.DB) *SessionHandler {
	return &SessionHandler{db: db}
}

type sessionJSON struct {
	ID         string         `json:"id"`
	EventID    string         `json:"event_id"`
	EventName  string         `json:"event_name"`
	TemplateID string         `json:"template_id"`
	Name       string         `json:"name"`
	StartsAt   string         `json:"starts_at"`
	EndsAt     string         `json:"ends_at"`
	Status     string         `json:"status"`
	CreatedAt  string         `json:"created_at"`
	Criteria   []criterionJSON `json:"criteria,omitempty"`
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
	ID          string              `json:"id"`
	PatrolID    string              `json:"patrol_id"`
	PatrolName  string              `json:"patrol_name"`
	Locked      bool                `json:"locked"`
	SubmittedAt string              `json:"submitted_at"`
	Scores      []submissionScoreJSON `json:"scores,omitempty"`
}

type submissionScoreJSON struct {
	CriterionID string `json:"criterion_id"`
	Value       int    `json:"value"`
}

type draftJSON struct {
	ID        string          `json:"id"`
	PatrolID  string          `json:"patrol_id"`
	Scores    []draftScoreJSON `json:"scores"`
	UpdatedAt string          `json:"updated_at"`
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

	// Parse optional status filter from query params
	statusParam := r.URL.Query()["status"]

	sessions, err := h.db.ListSessions(ctx, statusParam)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch sessions")
		return
	}

	span.SetAttributes(attribute.Int("sessions.count", len(sessions)))

	result := lo.Map(sessions, func(s database.SessionDetailRow, _ int) sessionJSON {
		return sessionJSON{
			ID:         s.ID,
			EventID:    s.EventID,
			EventName:  s.EventName,
			TemplateID: s.TemplateID,
			Name:       s.Name,
			StartsAt:   s.StartsAt.Format("2006-01-02T15:04:05Z"),
			EndsAt:     s.EndsAt.Format("2006-01-02T15:04:05Z"),
			Status:     s.ComputeStatus(),
			CreatedAt:  s.CreatedAt.Format("2006-01-02T15:04:05Z"),
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
		ID:       draft.ID,
		PatrolID: draft.PatrolID,
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

	var req struct {
		Scores map[string]int `json:"scores"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
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
