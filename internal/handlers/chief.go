package handlers

import (
	"fmt"
	"net/http"

	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/tracing"
)

// ChiefHandler handles camp chief scoring endpoints.
type ChiefHandler struct {
	db *database.DB
}

// NewChiefHandler creates a new ChiefHandler.
func NewChiefHandler(db *database.DB) *ChiefHandler {
	return &ChiefHandler{db: db}
}

type chiefRoundJSON struct {
	ID             string                `json:"id"`
	SessionID      string                `json:"session_id"`
	Status         string                `json:"status"`
	WinnerPatrolID *string               `json:"winner_patrol_id"`
	CreatedAt      string                `json:"created_at"`
	CompletedAt    *string               `json:"completed_at,omitempty"`
	Patrols        []chiefPatrolJSON     `json:"patrols,omitempty"`
	Scores         map[string][]scoreVal `json:"scores,omitempty"`
}

type chiefPatrolJSON struct {
	PatrolID    string `json:"patrol_id"`
	PatrolName  string `json:"patrol_name"`
	SubcampName string `json:"subcamp_name"`
	TotalScore  int    `json:"total_score"`
}

type scoreVal struct {
	CriterionID string `json:"criterion_id"`
	Value       int    `json:"value"`
}

// GetChiefRound handles GET /api/sessions/{session_id}/chief-round
// Returns the chief round for a session. For non-camp-chief users, returns
// limited info (status only). For camp chief, returns full details.
func (h *ChiefHandler) GetChiefRound(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_chief_round")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	user := auth.UserFromContext(ctx)

	round, err := h.db.GetChiefRound(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch chief round")
		return
	}

	if round == nil {
		// Check if session is fully submitted — if so, create the chief round
		fullySubmitted, err := h.db.IsSessionFullySubmitted(ctx, sessionID)
		if err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not check session status")
			return
		}

		if !fullySubmitted {
			// Not ready yet
			writeJSON(w, http.StatusOK, map[string]any{
				"chief_round": nil,
				"message":     "Awaiting all scorers to submit",
			})
			return
		}

		// Create the chief round
		round, err = h.db.CreateChiefRound(ctx, sessionID)
		if err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not create chief round")
			return
		}
	}

	// Non-camp-chief users get limited view
	if !user.IsCampChief() {
		writeJSON(w, http.StatusOK, map[string]any{
			"chief_round": map[string]any{
				"id":               round.ID,
				"session_id":       round.SessionID,
				"status":           round.Status,
				"winner_patrol_id": round.WinnerPatrolID,
			},
			"message": "Awaiting camp chief's final review",
		})
		return
	}

	// Camp chief gets full details
	patrols, err := h.db.GetChiefRoundPatrols(ctx, round.ID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch chief round patrols")
		return
	}

	scores, err := h.db.GetChiefScores(ctx, round.ID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch chief scores")
		return
	}

	// Group scores by patrol
	scoresByPatrol := make(map[string][]scoreVal)
	for _, s := range scores {
		scoresByPatrol[s.PatrolID] = append(scoresByPatrol[s.PatrolID], scoreVal{
			CriterionID: s.CriterionID,
			Value:       s.Value,
		})
	}

	var completedAt *string
	if round.CompletedAt != nil {
		t := round.CompletedAt.Format("2006-01-02T15:04:05Z")
		completedAt = &t
	}

	result := chiefRoundJSON{
		ID:             round.ID,
		SessionID:      round.SessionID,
		Status:         round.Status,
		WinnerPatrolID: round.WinnerPatrolID,
		CreatedAt:      round.CreatedAt.Format("2006-01-02T15:04:05Z"),
		CompletedAt:    completedAt,
		Patrols: lo.Map(patrols, func(p database.ChiefRoundPatrolRow, _ int) chiefPatrolJSON {
			return chiefPatrolJSON{
				PatrolID:    p.PatrolID,
				PatrolName:  p.PatrolName,
				SubcampName: p.SubcampName,
				TotalScore:  p.TotalScore,
			}
		}),
		Scores: scoresByPatrol,
	}

	writeJSON(w, http.StatusOK, map[string]any{"chief_round": result})
}

// SaveChiefScores handles POST /api/sessions/{session_id}/chief-round/scores
// Camp chief submits scores for a patrol.
func (h *ChiefHandler) SaveChiefScores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.save_chief_scores")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	round, err := h.db.GetChiefRound(ctx, sessionID)
	if err != nil || round == nil {
		writeError(w, r, http.StatusNotFound, "chief round not found")
		return
	}

	if round.Status == "completed" {
		writeError(w, r, http.StatusBadRequest, "chief round already completed")
		return
	}

	var req struct {
		PatrolID string         `json:"patrol_id"`
		Scores   map[string]int `json:"scores"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PatrolID == "" || len(req.Scores) == 0 {
		writeError(w, r, http.StatusBadRequest, "patrol_id and scores are required")
		return
	}

	// Validate the patrol is in this chief round
	patrols, err := h.db.GetChiefRoundPatrols(ctx, round.ID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch chief round patrols")
		return
	}

	validPatrol := lo.ContainsBy(patrols, func(p database.ChiefRoundPatrolRow) bool {
		return p.PatrolID == req.PatrolID
	})
	if !validPatrol {
		writeError(w, r, http.StatusBadRequest, "patrol not in chief round")
		return
	}

	// Validate score values against session criteria
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
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

	span.SetAttributes(
		attribute.String("patrol.id", req.PatrolID),
		attribute.Int("scores.count", len(req.Scores)),
	)

	if err := h.db.SaveChiefScores(ctx, round.ID, req.PatrolID, req.Scores); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not save scores")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// CompleteChiefRound handles POST /api/sessions/{session_id}/chief-round/complete
// Camp chief declares the overall winner.
func (h *ChiefHandler) CompleteChiefRound(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.complete_chief_round")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	round, err := h.db.GetChiefRound(ctx, sessionID)
	if err != nil || round == nil {
		writeError(w, r, http.StatusNotFound, "chief round not found")
		return
	}

	if round.Status == "completed" {
		writeError(w, r, http.StatusBadRequest, "chief round already completed")
		return
	}

	var req struct {
		WinnerPatrolID string `json:"winner_patrol_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.WinnerPatrolID == "" {
		writeError(w, r, http.StatusBadRequest, "winner_patrol_id is required")
		return
	}

	// Validate the winner is in this chief round
	patrols, err := h.db.GetChiefRoundPatrols(ctx, round.ID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch chief round patrols")
		return
	}

	validPatrol := lo.ContainsBy(patrols, func(p database.ChiefRoundPatrolRow) bool {
		return p.PatrolID == req.WinnerPatrolID
	})
	if !validPatrol {
		writeError(w, r, http.StatusBadRequest, "winner patrol not in chief round")
		return
	}

	span.SetAttributes(attribute.String("winner.patrol_id", req.WinnerPatrolID))

	if err := h.db.CompleteChiefRound(ctx, round.ID, req.WinnerPatrolID); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not complete chief round")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "winner_patrol_id": req.WinnerPatrolID})
}

// GetPatrolOriginalScores handles GET /api/sessions/{session_id}/chief-round/patrols/{patrol_id}/scores
// Returns the original submission scores and comments for a patrol (for the camp chief to review).
func (h *ChiefHandler) GetPatrolOriginalScores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")
	patrolID := r.PathValue("patrol_id")

	_, span := tracing.Tracer().Start(ctx, "handler.get_patrol_original_scores")
	defer span.End()
	span.SetAttributes(
		attribute.String("session.id", sessionID),
		attribute.String("patrol.id", patrolID),
	)

	// Fetch submission scores for this patrol in this session
	scores, err := h.db.GetSubmissionScoresByPatrol(ctx, sessionID, patrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch scores")
		return
	}

	result := lo.Map(scores, func(s database.SubmissionScoreRow, _ int) submissionScoreJSON {
		return submissionScoreJSON{
			CriterionID: s.CriterionID,
			Value:       s.Value,
			Comment:     s.Comment,
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"scores": result})
}
