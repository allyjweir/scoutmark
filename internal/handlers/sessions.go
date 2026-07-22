package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

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

func (h *SessionHandler) ensurePatrolScoringUnlocked(w http.ResponseWriter, r *http.Request, sessionID, patrolID string) bool {
	locked, err := h.db.IsPatrolScoringLocked(r.Context(), sessionID, patrolID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not check subcamp lock")
		return false
	}
	if locked {
		writeError(w, r, http.StatusLocked, "your subcamp scoring is locked")
		return false
	}
	return true
}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler(db *database.DB, broadcaster SessionBroadcaster) *SessionHandler {
	return &SessionHandler{db: db, broadcaster: broadcaster}
}

type sessionJSON struct {
	ID                string          `json:"id"`
	EventID           string          `json:"event_id"`
	EventName         string          `json:"event_name"`
	TemplateID        string          `json:"template_id"`
	Name              string          `json:"name"`
	RoundType         string          `json:"round_type"`
	SourceSessionID   *string         `json:"source_session_id,omitempty"`
	WinnerPatrolName  *string         `json:"winner_patrol_name,omitempty"`
	WinnerSubcampName *string         `json:"winner_subcamp_name,omitempty"`
	StartsAt          string          `json:"starts_at"`
	EndsAt            string          `json:"ends_at"`
	Status            string          `json:"status"`
	OwnSubcampLocked  bool            `json:"own_subcamp_locked,omitempty"`
	LockedAt          *string         `json:"locked_at,omitempty"`
	LockedBy          *string         `json:"locked_by,omitempty"`
	LockedByName      *string         `json:"locked_by_name,omitempty"`
	CreatedAt         string          `json:"created_at"`
	Criteria          []criterionJSON `json:"criteria,omitempty"`
	UserFinalised     bool            `json:"user_finalised"`
	PreviousSessionID *string         `json:"previous_session_id"`
	AwardBestPatrol   bool            `json:"award_best_patrol"`
	AwardMostImproved bool            `json:"award_most_improved"`
}

type criterionJSON struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	MinValue    int                  `json:"min_value"`
	MaxValue    int                  `json:"max_value"`
	SortOrder   int                  `json:"sort_order"`
	Rubric      *criterionRubricJSON `json:"rubric,omitempty"`
}

type criterionRubricJSON struct {
	Checklist []string                       `json:"checklist"`
	Bands     []database.CriterionRubricBand `json:"bands"`
}

func rubricJSONForCriterion(c database.CriterionRow) *criterionRubricJSON {
	checklist := c.RubricChecklist
	bands := c.RubricBands
	if len(checklist) == 0 && len(bands) == 0 {
		fallbackChecklist, fallbackBands, ok := database.DefaultCriterionRubric(c.ID)
		if !ok {
			return nil
		}
		checklist = fallbackChecklist
		bands = fallbackBands
	}
	return &criterionRubricJSON{
		Checklist: checklist,
		Bands:     bands,
	}
}

type patrolJSON struct {
	PatrolID  string `json:"patrol_id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	SubcampID string `json:"subcamp_id,omitempty"`
	Subcamp   string `json:"subcamp,omitempty"`
}

type submissionJSON struct {
	ID          string                `json:"id"`
	PatrolID    string                `json:"patrol_id"`
	PatrolName  string                `json:"patrol_name"`
	SubmittedBy string                `json:"submitted_by,omitempty"`
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

type awardJSON struct {
	AwardType string `json:"award_type"`
	PatrolID  string `json:"patrol_id"`
}

type round2FinalistJSON struct {
	SubcampID       string `json:"subcamp_id"`
	SubcampName     string `json:"subcamp_name"`
	PatrolID        string `json:"patrol_id"`
	PatrolName      string `json:"patrol_name"`
	SelectionSource string `json:"selection_source"`
}

func (h *SessionHandler) maybeEnsureRound2(ctx context.Context, session *database.SessionDetailRow) {
	if session == nil {
		return
	}
	if session.RoundType != "regular" || !session.AwardBestPatrol {
		return
	}
	status := session.ComputeStatus()
	if status != "CLOSED" && status != "LOCKED" {
		return
	}
	if _, err := h.db.EnsureRound2ForSourceSession(ctx, session.ID); err != nil {
		tracing.RecordError(ctx, err)
	}
}

func formatOptionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.Format("2006-01-02T15:04:05Z")
	return &formatted
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
	for i := range sessions {
		h.maybeEnsureRound2(ctx, &sessions[i])
	}

	winnerBySessionID := map[string]*database.Round2WinnerRow{}
	for _, s := range sessions {
		if s.RoundType != "round2" {
			continue
		}
		winner, err := h.db.GetRound2Winner(ctx, s.ID)
		if err != nil {
			tracing.RecordError(ctx, err)
			continue
		}
		winnerBySessionID[s.ID] = winner
	}

	// Look up which sessions this user has fully finalised
	finalisedSet, err := h.db.GetUserFinalisedSessionIDs(ctx, user.ID, user.IsAdmin)
	if err != nil {
		tracing.RecordError(ctx, err)
		// Non-fatal: just proceed without finalised info
		finalisedSet = map[string]bool{}
	}

	span.SetAttributes(attribute.Int("sessions.count", len(sessions)))

	result := lo.Map(sessions, func(s database.SessionDetailRow, _ int) sessionJSON {
		var winnerPatrolName *string
		var winnerSubcampName *string
		if winner := winnerBySessionID[s.ID]; winner != nil {
			winnerPatrolName = &winner.PatrolName
			winnerSubcampName = &winner.SubcampName
		}

		return sessionJSON{
			ID:                s.ID,
			EventID:           s.EventID,
			EventName:         s.EventName,
			TemplateID:        s.TemplateID,
			Name:              s.Name,
			RoundType:         s.RoundType,
			SourceSessionID:   s.SourceSessionID,
			WinnerPatrolName:  winnerPatrolName,
			WinnerSubcampName: winnerSubcampName,
			StartsAt:          s.StartsAt.Format("2006-01-02T15:04:05Z"),
			EndsAt:            s.EndsAt.Format("2006-01-02T15:04:05Z"),
			Status:            s.ComputeStatus(),
			LockedAt:          formatOptionalTime(s.LockedAt),
			LockedBy:          s.LockedBy,
			LockedByName:      s.LockedByName,
			CreatedAt:         s.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UserFinalised:     finalisedSet[s.ID],
			PreviousSessionID: s.PreviousSessionID,
			AwardBestPatrol:   s.AwardBestPatrol,
			AwardMostImproved: s.AwardMostImproved,
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
	h.maybeEnsureRound2(ctx, session)

	// Fetch criteria for the session's template
	criteria, err := h.db.GetTemplateCriteria(ctx, session.TemplateID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch criteria")
		return
	}

	// Fetch user's patrols
	canViewAllPatrols := user.IsAdmin || (user.IsCampChief && session.RoundType == "round2")
	patrols, err := h.db.GetSessionPatrolsForUser(ctx, user.ID, sessionID, canViewAllPatrols)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch patrols")
		return
	}

	// Fetch existing submissions for user's patrols (shared model)
	patrolIDs := lo.Map(patrols, func(p database.UserPatrolRow, _ int) string { return p.PatrolID })
	submissions, err := h.db.GetSubmissionsForPatrols(ctx, sessionID, patrolIDs)
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

	ownSubcampLocked := false
	if user.SubcampID != nil {
		ownSubcampLocked, err = h.db.IsSubcampScoringLocked(ctx, sessionID, *user.SubcampID)
		if err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not check subcamp lock")
			return
		}
	}

	sessionResult := sessionJSON{
		ID:                session.ID,
		EventID:           session.EventID,
		EventName:         session.EventName,
		TemplateID:        session.TemplateID,
		Name:              session.Name,
		RoundType:         session.RoundType,
		SourceSessionID:   session.SourceSessionID,
		StartsAt:          session.StartsAt.Format("2006-01-02T15:04:05Z"),
		EndsAt:            session.EndsAt.Format("2006-01-02T15:04:05Z"),
		Status:            session.ComputeStatus(),
		OwnSubcampLocked:  ownSubcampLocked,
		LockedAt:          formatOptionalTime(session.LockedAt),
		LockedBy:          session.LockedBy,
		LockedByName:      session.LockedByName,
		CreatedAt:         session.CreatedAt.Format("2006-01-02T15:04:05Z"),
		PreviousSessionID: session.PreviousSessionID,
		AwardBestPatrol:   session.AwardBestPatrol,
		AwardMostImproved: session.AwardMostImproved,
		Criteria: lo.Map(criteria, func(c database.CriterionRow, _ int) criterionJSON {
			return criterionJSON{
				ID:          c.ID,
				Title:       c.Title,
				Description: c.Description,
				MinValue:    c.MinValue,
				MaxValue:    c.MaxValue,
				SortOrder:   c.SortOrder,
				Rubric:      rubricJSONForCriterion(c),
			}
		}),
	}
	if session.RoundType == "round2" {
		winner, err := h.db.GetRound2Winner(ctx, session.ID)
		if err != nil {
			tracing.RecordError(ctx, err)
		} else if winner != nil {
			sessionResult.WinnerPatrolName = &winner.PatrolName
			sessionResult.WinnerSubcampName = &winner.SubcampName
		}
	}

	patrolResult := lo.Map(patrols, func(p database.UserPatrolRow, _ int) patrolJSON {
		return patrolJSON{PatrolID: p.PatrolID, Name: p.Name, SortOrder: p.SortOrder, SubcampID: p.SubcampID, Subcamp: p.Subcamp}
	})

	submissionResult := lo.Map(submissions, func(s database.SubmissionRow, _ int) submissionJSON {
		return submissionJSON{
			ID:          s.ID,
			PatrolID:    s.PatrolID,
			PatrolName:  s.PatrolName,
			SubmittedBy: s.SubmittedBy,
			Locked:      s.Locked,
			SubmittedAt: s.SubmittedAt.Format("2006-01-02T15:04:05Z"),
		}
	})

	// Fetch user's award selections for this session
	awardRows, err := h.db.GetSessionAwards(ctx, user.ID, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		// Non-fatal, continue without awards
		awardRows = nil
	}
	awardResult := lo.Map(awardRows, func(a database.SessionAwardRow, _ int) awardJSON {
		return awardJSON{AwardType: a.AwardType, PatrolID: a.PatrolID}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"session":     sessionResult,
		"patrols":     patrolResult,
		"submissions": submissionResult,
		"awards":      awardResult,
	})
}

// LockSession handles POST /api/admin/sessions/{session_id}/lock
func (h *SessionHandler) LockSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	user := auth.UserFromContext(ctx)
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}

	if session.ComputeStatus() != "ACTIVE" {
		writeError(w, r, http.StatusBadRequest, "only active sessions can be locked")
		return
	}
	if user.IsCampChief && session.RoundType != "round2" {
		writeError(w, r, http.StatusForbidden, "camp chief can only manage round 2 sessions")
		return
	}

	if err := h.db.LockSession(ctx, sessionID, user.ID); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not lock session")
		return
	}
	if _, err := h.db.EnsureRound2ForSourceSession(ctx, sessionID); err != nil {
		tracing.RecordError(ctx, err)
	}

	lockedSession, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch locked session")
		return
	}

	if fb, ok := h.broadcaster.(interface {
		BroadcastSessionLocked(sessionID, userID, displayName, lockedAt, endsAt string)
	}); ok && lockedSession.LockedAt != nil {
		fb.BroadcastSessionLocked(
			sessionID,
			user.ID,
			user.DisplayName,
			lockedSession.LockedAt.UTC().Format("2006-01-02T15:04:05Z"),
			lockedSession.EndsAt.Format("2006-01-02T15:04:05Z"),
		)
	}
	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(ctx, sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// UnlockSession handles POST /api/admin/sessions/{session_id}/unlock
func (h *SessionHandler) UnlockSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	user := auth.UserFromContext(ctx)
	if user.IsCampChief && session.RoundType != "round2" {
		writeError(w, r, http.StatusForbidden, "camp chief can only manage round 2 sessions")
		return
	}

	if session.LockedAt == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	if err := h.db.UnlockSession(ctx, sessionID); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not unlock session")
		return
	}

	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(ctx, sessionID)
	}
	if fb, ok := h.broadcaster.(interface {
		BroadcastSessionUnlocked(sessionID string)
	}); ok {
		fb.BroadcastSessionUnlocked(sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// EnsureRound2 handles POST /api/admin/sessions/{session_id}/round2
// Creates round 2 for a closed regular session if it does not already exist.
func (h *SessionHandler) EnsureRound2(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	round2, err := h.db.EnsureRound2ForSourceSession(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not create round 2 session")
		return
	}
	if round2 == nil {
		writeError(w, r, http.StatusBadRequest, "round 2 not eligible yet or no finalists available")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session": sessionJSON{
			ID:                round2.ID,
			EventID:           round2.EventID,
			EventName:         round2.EventName,
			TemplateID:        round2.TemplateID,
			Name:              round2.Name,
			RoundType:         round2.RoundType,
			SourceSessionID:   round2.SourceSessionID,
			StartsAt:          round2.StartsAt.Format("2006-01-02T15:04:05Z"),
			EndsAt:            round2.EndsAt.Format("2006-01-02T15:04:05Z"),
			Status:            round2.ComputeStatus(),
			LockedAt:          formatOptionalTime(round2.LockedAt),
			LockedBy:          round2.LockedBy,
			LockedByName:      round2.LockedByName,
			CreatedAt:         round2.CreatedAt.Format("2006-01-02T15:04:05Z"),
			PreviousSessionID: round2.PreviousSessionID,
			AwardBestPatrol:   round2.AwardBestPatrol,
			AwardMostImproved: round2.AwardMostImproved,
		},
	})
}

// GetRound2Finalists handles GET /api/admin/sessions/{session_id}/round2/finalists
func (h *SessionHandler) GetRound2Finalists(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	if session.RoundType != "round2" {
		writeError(w, r, http.StatusBadRequest, "session is not round 2")
		return
	}

	finalists, err := h.db.GetRound2Finalists(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch round 2 finalists")
		return
	}

	result := lo.Map(finalists, func(f database.Round2FinalistRow, _ int) round2FinalistJSON {
		return round2FinalistJSON{
			SubcampID:       f.SubcampID,
			SubcampName:     f.SubcampName,
			PatrolID:        f.PatrolID,
			PatrolName:      f.PatrolName,
			SelectionSource: f.SelectionSource,
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"finalists": result})
}

// SetRound2Finalist handles PUT /api/admin/sessions/{session_id}/round2/finalists/{subcamp_id}
func (h *SessionHandler) SetRound2Finalist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	subcampID := r.PathValue("subcamp_id")

	var req struct {
		PatrolID string `json:"patrol_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PatrolID == "" {
		writeError(w, r, http.StatusBadRequest, "patrol_id is required")
		return
	}

	if err := h.db.SetRound2Finalist(ctx, sessionID, subcampID, req.PatrolID); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	owns, err := h.db.UserOwnsSessionPatrol(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}
	if !h.ensurePatrolScoringUnlocked(w, r, sessionID, patrolID) {
		return
	}

	draft, err := h.db.GetDraft(ctx, sessionID, patrolID)
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
		if session.LockedAt != nil {
			locker := lo.FromPtr(session.LockedByName)
			if locker == "" {
				locker = "an administrator"
			}
			writeError(w, r, http.StatusLocked, fmt.Sprintf("session was locked by %s", locker))
			return
		}
		span.SetAttributes(attribute.String("session.status", session.ComputeStatus()))
		writeError(w, r, http.StatusBadRequest, "session is not active")
		return
	}

	// IDOR check: verify user owns this patrol
	owns, err := h.db.UserOwnsSessionPatrol(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}
	if !h.ensurePatrolScoringUnlocked(w, r, sessionID, patrolID) {
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
		SubmittedBy: submission.SubmittedBy,
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
		SubmittedBy: submission.SubmittedBy,
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
			SubmittedBy: s.SubmittedBy,
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
		if session.LockedAt != nil {
			locker := lo.FromPtr(session.LockedByName)
			if locker == "" {
				locker = "an administrator"
			}
			writeError(w, r, http.StatusLocked, fmt.Sprintf("session was locked by %s", locker))
			return
		}
		span.SetAttributes(attribute.String("session.status", session.ComputeStatus()))
		writeError(w, r, http.StatusBadRequest, "session is not active")
		return
	}

	if session.RoundType == "round2" {
		if !user.IsAdmin && !user.IsCampChief {
			writeError(w, r, http.StatusForbidden, "only admins or camp chief can finalise round 2 scoring")
			return
		}

		patrols, err := h.db.GetSessionPatrolsForUser(ctx, user.ID, sessionID, true)
		if err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not fetch round 2 finalists")
			return
		}

		subcampSet := map[string]bool{}
		subcampIDs := make([]string, 0)
		for _, patrol := range patrols {
			if subcampSet[patrol.SubcampID] {
				continue
			}
			subcampSet[patrol.SubcampID] = true
			subcampIDs = append(subcampIDs, patrol.SubcampID)
		}

		allNewSubmissions := make([]database.SubmissionRow, 0)
		for _, subcampID := range subcampIDs {
			newSubmissions, err := h.db.FinaliseSession(ctx, user.ID, sessionID, subcampID)
			if err != nil {
				tracing.RecordError(ctx, err)
				writeError(w, r, http.StatusInternalServerError, "could not finalise round 2 session")
				return
			}
			allNewSubmissions = append(allNewSubmissions, newSubmissions...)
		}

		if err := h.db.LockSession(ctx, sessionID, user.ID); err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not lock round 2 session")
			return
		}

		patrolIDs := lo.Map(patrols, func(p database.UserPatrolRow, _ int) string { return p.PatrolID })
		allSubmissions, err := h.db.GetSubmissionsForPatrols(ctx, sessionID, patrolIDs)
		if err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not fetch round 2 submissions")
			return
		}

		result := lo.Map(allSubmissions, func(s database.SubmissionRow, _ int) submissionJSON {
			return submissionJSON{
				ID:          s.ID,
				PatrolID:    s.PatrolID,
				PatrolName:  s.PatrolName,
				SubmittedBy: s.SubmittedBy,
				Locked:      s.Locked,
				SubmittedAt: s.SubmittedAt.Format("2006-01-02T15:04:05Z"),
			}
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"submissions":     result,
			"finalised_count": len(allNewSubmissions),
			"round2_closed":   true,
		})

		if h.broadcaster != nil {
			h.broadcaster.BroadcastSessionProgress(ctx, sessionID)
		}
		if fb, ok := h.broadcaster.(interface {
			BroadcastSessionLocked(sessionID, userID, displayName, lockedAt, endsAt string)
		}); ok {
			fb.BroadcastSessionLocked(
				sessionID,
				user.ID,
				user.DisplayName,
				time.Now().UTC().Format("2006-01-02T15:04:05Z"),
				session.EndsAt.Format("2006-01-02T15:04:05Z"),
			)
		}
		return
	}

	var req struct {
		SubcampID string `json:"subcamp_id"`
	}
	if err := readJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	targetSubcampID := ""
	if req.SubcampID != "" {
		if !user.IsAdmin {
			writeError(w, r, http.StatusForbidden, "only admins can finalise another subcamp")
			return
		}
		targetSubcampID = req.SubcampID
	} else if user.SubcampID != nil {
		targetSubcampID = *user.SubcampID
	} else if !user.IsAdmin {
		writeError(w, r, http.StatusForbidden, "user is not assigned to a subcamp")
		return
	} else if user.IsAdmin {
		writeError(w, r, http.StatusBadRequest, "subcamp_id is required for admin users without an assigned subcamp")
		return
	}
	locked, err := h.db.IsSubcampScoringLocked(ctx, sessionID, targetSubcampID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "could not check subcamp lock")
		return
	}
	if locked {
		writeError(w, r, http.StatusLocked, "subcamp scoring is locked")
		return
	}

	newSubmissions, err := h.db.FinaliseSession(ctx, user.ID, sessionID, targetSubcampID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not finalise session")
		return
	}

	// Return all submissions for the finalised subcamp patrols.
	userPatrols, err := h.db.GetSessionPatrolsForUser(ctx, user.ID, sessionID, user.IsAdmin)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch patrols")
		return
	}
	if targetSubcampID != "" {
		userPatrols = lo.Filter(userPatrols, func(p database.UserPatrolRow, _ int) bool {
			return p.SubcampID == targetSubcampID
		})
	}
	patrolIDs := lo.Map(userPatrols, func(p database.UserPatrolRow, _ int) string { return p.PatrolID })
	allSubmissions, err := h.db.GetSubmissionsForPatrols(ctx, sessionID, patrolIDs)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch submissions")
		return
	}

	autoLocked := false
	fullySubmitted, err := h.db.IsSessionFullySubmitted(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
	} else if fullySubmitted {
		if err := h.db.LockSession(ctx, sessionID, user.ID); err != nil {
			tracing.RecordError(ctx, err)
		} else {
			autoLocked = true
			if _, err := h.db.EnsureRound2ForSourceSession(ctx, sessionID); err != nil {
				tracing.RecordError(ctx, err)
			}
		}
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
			SubmittedBy: s.SubmittedBy,
			Locked:      s.Locked,
			SubmittedAt: s.SubmittedAt.Format("2006-01-02T15:04:05Z"),
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"submissions":     result,
		"finalised_count": len(newSubmissions),
		"auto_locked":     autoLocked,
	})

	// Broadcast updated progress to WebSocket subscribers
	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(ctx, sessionID)
	}

	// Broadcast finalised event to all session subscribers (for lock screen)
	if fb, ok := h.broadcaster.(interface {
		BroadcastSessionFinalised(sessionID, userID, displayName, subcampID, finalisedAt, endsAt string)
	}); ok {
		fb.BroadcastSessionFinalised(
			sessionID,
			user.ID,
			user.DisplayName,
			targetSubcampID,
			time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			session.EndsAt.Format("2006-01-02T15:04:05Z"),
		)
	}
	if autoLocked {
		if fb, ok := h.broadcaster.(interface {
			BroadcastSessionLocked(sessionID, userID, displayName, lockedAt, endsAt string)
		}); ok {
			fb.BroadcastSessionLocked(
				sessionID,
				user.ID,
				user.DisplayName,
				time.Now().UTC().Format("2006-01-02T15:04:05Z"),
				session.EndsAt.Format("2006-01-02T15:04:05Z"),
			)
		}
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
		if session.LockedAt != nil {
			locker := lo.FromPtr(session.LockedByName)
			if locker == "" {
				locker = "an administrator"
			}
			writeError(w, r, http.StatusLocked, fmt.Sprintf("session was locked by %s", locker))
			return
		}
		span.SetAttributes(attribute.String("session.status", session.ComputeStatus()))
		writeError(w, r, http.StatusBadRequest, "session is not active")
		return
	}

	if user.IsCampChief && session.RoundType != "round2" {
		writeError(w, r, http.StatusForbidden, "camp chief can only revise round 2 sessions")
		return
	}
	reviseAllPatrols := user.IsCampChief
	if err := h.db.ReviseSession(ctx, user.ID, sessionID, reviseAllPatrols); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not revise session")
		return
	}

	if h.broadcaster != nil {
		h.broadcaster.BroadcastSessionProgress(ctx, sessionID)
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
	owns, err := h.db.UserOwnsSessionPatrol(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}

	scores, err := h.db.GetSubmissionScoresByPatrol(ctx, sessionID, patrolID)
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
		SubcampID   string           `json:"subcamp_id"`
		SubcampName string           `json:"subcamp_name"`
		Patrols     []patrolProgress `json:"patrols"`
	}
	type subcampProgress struct {
		SubcampID   string         `json:"subcamp_id"`
		SubcampName string         `json:"subcamp_name"`
		Users       []userProgress `json:"users"`
	}

	userMap := make(map[string]*userProgress)
	var userOrder []string

	for _, row := range progress {
		up, exists := userMap[row.UserID]
		if !exists {
			up = &userProgress{
				UserID:      row.UserID,
				DisplayName: row.DisplayName,
				SubcampID:   row.SubcampID,
				SubcampName: row.SubcampName,
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

	// Preserve insertion order, grouped by subcamp.
	usersBySubcamp := make(map[string][]userProgress)
	subcampOrder := []string{}
	seenSubcamp := make(map[string]bool)
	for _, id := range userOrder {
		u := *userMap[id]
		usersBySubcamp[u.SubcampID] = append(usersBySubcamp[u.SubcampID], u)
		if !seenSubcamp[u.SubcampID] {
			subcampOrder = append(subcampOrder, u.SubcampID)
			seenSubcamp[u.SubcampID] = true
		}
	}

	subcamps := make([]subcampProgress, 0, len(subcampOrder))
	for _, sid := range subcampOrder {
		name := sid
		for _, u := range usersBySubcamp[sid] {
			if u.SubcampName != "" {
				name = u.SubcampName
				break
			}
		}
		subcamps = append(subcamps, subcampProgress{SubcampID: sid, SubcampName: name, Users: usersBySubcamp[sid]})
	}

	span.SetAttributes(attribute.Int("users.count", len(userOrder)))

	sessionResult := sessionJSON{
		ID:                session.ID,
		EventID:           session.EventID,
		EventName:         session.EventName,
		Name:              session.Name,
		RoundType:         session.RoundType,
		SourceSessionID:   session.SourceSessionID,
		StartsAt:          session.StartsAt.Format("2006-01-02T15:04:05Z"),
		EndsAt:            session.EndsAt.Format("2006-01-02T15:04:05Z"),
		Status:            session.ComputeStatus(),
		CreatedAt:         session.CreatedAt.Format("2006-01-02T15:04:05Z"),
		PreviousSessionID: session.PreviousSessionID,
		AwardBestPatrol:   session.AwardBestPatrol,
		AwardMostImproved: session.AwardMostImproved,
	}

	// Fetch all awards for this session (admin view: all users)
	allAwards, err := h.db.GetAllSessionAwards(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		allAwards = nil
	}

	// Build a patrol name lookup from progress data
	patrolNames := make(map[string]string)
	for _, row := range progress {
		patrolNames[row.PatrolID] = row.PatrolName
	}

	// Group awards by user_id
	type userAwardJSON struct {
		AwardType  string `json:"award_type"`
		PatrolID   string `json:"patrol_id"`
		PatrolName string `json:"patrol_name"`
	}
	userAwardsMap := make(map[string][]userAwardJSON)
	for _, a := range allAwards {
		userAwardsMap[a.UserID] = append(userAwardsMap[a.UserID], userAwardJSON{
			AwardType:  a.AwardType,
			PatrolID:   a.PatrolID,
			PatrolName: patrolNames[a.PatrolID],
		})
	}

	// Attach awards to users
	type userWithAwards struct {
		UserID      string           `json:"user_id"`
		DisplayName string           `json:"display_name"`
		SubcampID   string           `json:"subcamp_id"`
		SubcampName string           `json:"subcamp_name"`
		Patrols     []patrolProgress `json:"patrols"`
		Awards      []userAwardJSON  `json:"awards,omitempty"`
	}
	type subcampWithAwards struct {
		SubcampID   string           `json:"subcamp_id"`
		SubcampName string           `json:"subcamp_name"`
		Users       []userWithAwards `json:"users"`
	}

	subcampsWithAwards := make([]subcampWithAwards, 0, len(subcamps))
	flatUsersWithAwards := make([]userWithAwards, 0)
	for _, sc := range subcamps {
		usersWithAwards := make([]userWithAwards, 0, len(sc.Users))
		for _, u := range sc.Users {
			uw := userWithAwards{
				UserID:      u.UserID,
				DisplayName: u.DisplayName,
				SubcampID:   u.SubcampID,
				SubcampName: u.SubcampName,
				Patrols:     u.Patrols,
				Awards:      userAwardsMap[u.UserID],
			}
			usersWithAwards = append(usersWithAwards, uw)
			flatUsersWithAwards = append(flatUsersWithAwards, uw)
		}
		subcampsWithAwards = append(subcampsWithAwards, subcampWithAwards{
			SubcampID:   sc.SubcampID,
			SubcampName: sc.SubcampName,
			Users:       usersWithAwards,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session":  sessionResult,
		"users":    flatUsersWithAwards,
		"subcamps": subcampsWithAwards,
	})
}

// SaveAward handles POST /api/sessions/{session_id}/awards
// Saves or updates a single award selection for the current user.
func (h *SessionHandler) SaveAward(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.save_award")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	user := auth.UserFromContext(ctx)

	var req struct {
		AwardType string `json:"award_type"`
		PatrolID  string `json:"patrol_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate award type
	if req.AwardType != "best_patrol" && req.AwardType != "most_improved" {
		writeError(w, r, http.StatusBadRequest, "invalid award type")
		return
	}

	// Validate session exists and has this award enabled
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	if req.AwardType == "best_patrol" && !session.AwardBestPatrol {
		writeError(w, r, http.StatusBadRequest, "best patrol award not enabled for this session")
		return
	}
	if req.AwardType == "most_improved" && !session.AwardMostImproved {
		writeError(w, r, http.StatusBadRequest, "most improved award not enabled for this session")
		return
	}

	// IDOR check: verify user owns the patrol
	owns, err := h.db.UserOwnsSessionPatrol(ctx, user.ID, sessionID, req.PatrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}

	span.SetAttributes(
		attribute.String("award.type", req.AwardType),
		attribute.String("award.patrol_id", req.PatrolID),
	)

	award, err := h.db.UpsertSessionAward(ctx, user.ID, sessionID, req.AwardType, req.PatrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not save award")
		return
	}

	writeJSON(w, http.StatusOK, awardJSON{
		AwardType: award.AwardType,
		PatrolID:  award.PatrolID,
	})
}

// GetPreviousScores handles GET /api/sessions/{session_id}/previous-scores
// Returns per-patrol totals from the previous session for the current user.
func (h *SessionHandler) GetPreviousScores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_previous_scores")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	user := auth.UserFromContext(ctx)

	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}

	if session.PreviousSessionID == nil {
		writeJSON(w, http.StatusOK, map[string]any{"totals": []any{}})
		return
	}

	// Get user's patrol IDs for the lookup
	userPatrols, err := h.db.GetSessionPatrolsForUser(ctx, user.ID, sessionID, user.IsAdmin)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch patrols")
		return
	}
	patrolIDs := lo.Map(userPatrols, func(p database.UserPatrolRow, _ int) string { return p.PatrolID })

	type patrolTotalJSON struct {
		PatrolID   string `json:"patrol_id"`
		PatrolName string `json:"patrol_name"`
		Total      int    `json:"total"`
	}

	totals, err := h.db.GetPreviousSessionTotals(ctx, *session.PreviousSessionID, patrolIDs)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch previous scores")
		return
	}

	span.SetAttributes(attribute.Int("totals.count", len(totals)))

	result := lo.Map(totals, func(t database.PatrolTotalRow, _ int) patrolTotalJSON {
		return patrolTotalJSON{
			PatrolID:   t.PatrolID,
			PatrolName: t.PatrolName,
			Total:      t.Total,
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"totals": result})
}

// GetAdminUserScores handles GET /api/admin/sessions/{session_id}/users/{user_id}/scores
// Returns all submitted scores for a specific user in a session (admin only).
func (h *SessionHandler) GetAdminUserScores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	userID := r.PathValue("user_id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_admin_user_scores")
	defer span.End()
	span.SetAttributes(
		attribute.String("session.id", sessionID),
		attribute.String("user.id", userID),
	)

	// Fetch session details (for criteria)
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}

	// Fetch criteria
	criteriaRows, err := h.db.GetTemplateCriteria(ctx, session.TemplateID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch criteria")
		return
	}

	// Fetch user display name
	targetUser, err := h.db.GetUserByID(ctx, userID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "user not found")
		return
	}

	// Fetch all submissions for this user in this session
	submissions, err := h.db.GetAdminUserSubmissions(ctx, userID, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch submissions")
		return
	}

	span.SetAttributes(attribute.Int("submissions.count", len(submissions)))

	criteria := lo.Map(criteriaRows, func(c database.CriterionRow, _ int) criterionJSON {
		return criterionJSON{
			ID:          c.ID,
			Title:       c.Title,
			Description: c.Description,
			MinValue:    c.MinValue,
			MaxValue:    c.MaxValue,
			SortOrder:   c.SortOrder,
			Rubric:      rubricJSONForCriterion(c),
		}
	})

	type perUserCommentJSON struct {
		CriterionID string `json:"criterion_id"`
		UserID      string `json:"user_id"`
		DisplayName string `json:"display_name"`
		Comment     string `json:"comment"`
	}

	type patrolScoresJSON struct {
		PatrolID   string                `json:"patrol_id"`
		PatrolName string                `json:"patrol_name"`
		Scores     []submissionScoreJSON `json:"scores"`
		Comments   []perUserCommentJSON  `json:"comments"`
	}

	// Fetch per-user comments from submission_comments table
	perUserComments, err := h.db.GetSubmissionCommentsBySession(ctx, userID, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		// Non-fatal: continue without comments
		perUserComments = nil
	}

	// Group per-user comments by patrol (using submission_id → patrol lookup)
	// Build a submission_id → patrol_id map from the submissions data
	subToPatrol := make(map[string]string)
	for _, s := range submissions {
		for _, sc := range s.Scores {
			subToPatrol[sc.SubmissionID] = s.PatrolID
		}
	}
	commentsByPatrol := make(map[string][]perUserCommentJSON)
	for _, c := range perUserComments {
		patrolID := subToPatrol[c.DraftID] // DraftID holds submission_id for submission comments
		commentsByPatrol[patrolID] = append(commentsByPatrol[patrolID], perUserCommentJSON{
			CriterionID: c.CriterionID,
			UserID:      c.UserID,
			DisplayName: c.DisplayName,
			Comment:     c.Comment,
		})
	}

	patrols := lo.Map(submissions, func(s database.AdminUserSubmissionRow, _ int) patrolScoresJSON {
		return patrolScoresJSON{
			PatrolID:   s.PatrolID,
			PatrolName: s.PatrolName,
			Scores: lo.Map(s.Scores, func(sc database.SubmissionScoreRow, _ int) submissionScoreJSON {
				return submissionScoreJSON{
					CriterionID: sc.CriterionID,
					Value:       sc.Value,
				}
			}),
			Comments: commentsByPatrol[s.PatrolID],
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":      userID,
		"display_name": targetUser.DisplayName,
		"session_name": session.Name,
		"criteria":     criteria,
		"patrols":      patrols,
	})
}

// ─── Per-User Comments ──────────────────────────────────────────────

// CommentBroadcaster extends SessionBroadcaster with per-user comment broadcasts.
type CommentBroadcaster interface {
	SessionBroadcaster
	BroadcastCommentUpdated(sessionID string, payload any, exclude any)
}

type commentJSON2 struct {
	CriterionID string `json:"criterion_id"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Comment     string `json:"comment"`
	UpdatedAt   string `json:"updated_at"`
}

// GetDraftComments handles GET /api/sessions/{session_id}/patrols/{patrol_id}/comments
func (h *SessionHandler) GetDraftComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")
	user := auth.UserFromContext(ctx)

	_, span := tracing.Tracer().Start(ctx, "handler.get_draft_comments")
	defer span.End()
	tracing.AddSessionAttrs(ctx, sessionID, patrolID)

	// IDOR check
	owns, err := h.db.UserOwnsSessionPatrol(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}

	comments, err := h.db.GetDraftComments(ctx, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch comments")
		return
	}

	result := make([]commentJSON2, 0, len(comments))
	for _, c := range comments {
		result = append(result, commentJSON2{
			CriterionID: c.CriterionID,
			UserID:      c.UserID,
			DisplayName: c.DisplayName,
			Comment:     c.Comment,
			UpdatedAt:   c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"comments": result})
}

// SaveDraftComment handles PUT /api/sessions/{session_id}/patrols/{patrol_id}/comments/{criterion_id}
func (h *SessionHandler) SaveDraftComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")
	criterionID := r.PathValue("criterion_id")
	user := auth.UserFromContext(ctx)

	_, span := tracing.Tracer().Start(ctx, "handler.save_draft_comment")
	defer span.End()
	tracing.AddSessionAttrs(ctx, sessionID, patrolID)

	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	if session.ComputeStatus() != "ACTIVE" {
		if session.LockedAt != nil {
			locker := lo.FromPtr(session.LockedByName)
			if locker == "" {
				locker = "an administrator"
			}
			writeError(w, r, http.StatusLocked, fmt.Sprintf("session was locked by %s", locker))
			return
		}
		writeError(w, r, http.StatusBadRequest, "session is not active")
		return
	}

	// IDOR check
	owns, err := h.db.UserOwnsSessionPatrol(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}
	if !h.ensurePatrolScoringUnlocked(w, r, sessionID, patrolID) {
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	saved, err := h.db.SaveDraftComment(ctx, user.ID, user.DisplayName, sessionID, patrolID, criterionID, req.Comment)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not save comment")
		return
	}

	result := commentJSON2{
		CriterionID: saved.CriterionID,
		UserID:      saved.UserID,
		DisplayName: saved.DisplayName,
		Comment:     saved.Comment,
		UpdatedAt:   saved.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	writeJSON(w, http.StatusOK, result)

	// Broadcast to WS peers
	if b, ok := h.broadcaster.(CommentBroadcaster); ok {
		b.BroadcastCommentUpdated(sessionID, map[string]any{
			"session_id":   sessionID,
			"patrol_id":    patrolID,
			"criterion_id": criterionID,
			"user_id":      user.ID,
			"display_name": user.DisplayName,
			"comment":      req.Comment,
		}, nil)
	}
}

// DeleteDraftComment handles DELETE /api/sessions/{session_id}/patrols/{patrol_id}/comments/{criterion_id}
func (h *SessionHandler) DeleteDraftComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")
	criterionID := r.PathValue("criterion_id")
	user := auth.UserFromContext(ctx)

	_, span := tracing.Tracer().Start(ctx, "handler.delete_draft_comment")
	defer span.End()
	tracing.AddSessionAttrs(ctx, sessionID, patrolID)

	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	if session.ComputeStatus() != "ACTIVE" {
		if session.LockedAt != nil {
			locker := lo.FromPtr(session.LockedByName)
			if locker == "" {
				locker = "an administrator"
			}
			writeError(w, r, http.StatusLocked, fmt.Sprintf("session was locked by %s", locker))
			return
		}
		writeError(w, r, http.StatusBadRequest, "session is not active")
		return
	}

	// IDOR check
	owns, err := h.db.UserOwnsSessionPatrol(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}
	if !h.ensurePatrolScoringUnlocked(w, r, sessionID, patrolID) {
		return
	}

	if err := h.db.DeleteDraftComment(ctx, user.ID, sessionID, patrolID, criterionID); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not delete comment")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	// Broadcast to WS peers
	if b, ok := h.broadcaster.(CommentBroadcaster); ok {
		b.BroadcastCommentUpdated(sessionID, map[string]any{
			"session_id":   sessionID,
			"patrol_id":    patrolID,
			"criterion_id": criterionID,
			"user_id":      user.ID,
			"display_name": user.DisplayName,
			"comment":      "",
		}, nil)
	}
}

// GetSubmittedComments handles GET /api/sessions/{session_id}/patrols/{patrol_id}/submitted-comments
func (h *SessionHandler) GetSubmittedComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")
	user := auth.UserFromContext(ctx)

	_, span := tracing.Tracer().Start(ctx, "handler.get_submitted_comments")
	defer span.End()

	// IDOR check
	owns, err := h.db.UserOwnsSessionPatrol(ctx, user.ID, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}
	if !owns {
		writeError(w, r, http.StatusForbidden, "not assigned to this patrol")
		return
	}

	comments, err := h.db.GetSubmissionCommentsByPatrol(ctx, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch comments")
		return
	}

	result := make([]commentJSON2, 0, len(comments))
	for _, c := range comments {
		result = append(result, commentJSON2{
			CriterionID: c.CriterionID,
			UserID:      c.UserID,
			DisplayName: c.DisplayName,
			Comment:     c.Comment,
			UpdatedAt:   c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"comments": result})
}

// GetSessionComments handles GET /api/admin/sessions/{session_id}/comments
// Returns all non-empty comments across all users for a session (admin only).
func (h *SessionHandler) GetSessionComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_session_comments")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	comments, err := h.db.GetAllSessionComments(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch comments")
		return
	}

	span.SetAttributes(attribute.Int("comments.count", len(comments)))

	type commentJSON struct {
		UserID         string `json:"user_id"`
		DisplayName    string `json:"display_name"`
		PatrolID       string `json:"patrol_id"`
		PatrolName     string `json:"patrol_name"`
		CriterionID    string `json:"criterion_id"`
		CriterionTitle string `json:"criterion_title"`
		Value          int    `json:"value"`
		Comment        string `json:"comment"`
	}

	result := lo.Map(comments, func(c database.SessionCommentRow, _ int) commentJSON {
		return commentJSON{
			UserID:         c.UserID,
			DisplayName:    c.DisplayName,
			PatrolID:       c.PatrolID,
			PatrolName:     c.PatrolName,
			CriterionID:    c.CriterionID,
			CriterionTitle: c.CriterionTitle,
			Value:          c.Value,
			Comment:        c.Comment,
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"comments": result})
}
