package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// AdminHandler handles web admin endpoints.
type AdminHandler struct {
	db *database.DB
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(db *database.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

type adminBootstrapJSON struct {
	Users     []adminUserJSON     `json:"users"`
	Events    []adminEventJSON    `json:"events"`
	Templates []adminTemplateJSON `json:"templates"`
	Patrols   []adminPatrolJSON   `json:"patrols"`
	Sessions  []adminSessionJSON  `json:"sessions"`
}

type adminUserJSON struct {
	ID          string                `json:"id"`
	Username    string                `json:"username"`
	DisplayName string                `json:"display_name"`
	IsAdmin     bool                  `json:"is_admin"`
	CreatedAt   string                `json:"created_at"`
	Patrols     []adminUserPatrolJSON `json:"patrols,omitempty"`
}

type adminUserPatrolJSON struct {
	PatrolID   string `json:"patrol_id"`
	PatrolName string `json:"patrol_name"`
	SortOrder  int    `json:"sort_order"`
}

type adminEventJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

type adminCriterionJSON struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	MinValue    int    `json:"min_value"`
	MaxValue    int    `json:"max_value"`
	SortOrder   int    `json:"sort_order"`
}

type adminTemplateJSON struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	CreatedAt   string               `json:"created_at"`
	Criteria    []adminCriterionJSON `json:"criteria,omitempty"`
}

type adminPatrolJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type adminSessionJSON struct {
	ID                string  `json:"id"`
	EventID           string  `json:"event_id"`
	EventName         string  `json:"event_name"`
	TemplateID        string  `json:"template_id"`
	TemplateName      string  `json:"template_name"`
	Name              string  `json:"name"`
	StartsAt          string  `json:"starts_at"`
	EndsAt            string  `json:"ends_at"`
	Status            string  `json:"status"`
	CreatedAt         string  `json:"created_at"`
	PreviousSessionID *string `json:"previous_session_id"`
	AwardBestPatrol   bool    `json:"award_best_patrol"`
	AwardMostImproved bool    `json:"award_most_improved"`
}

type adminCreateUserRequest struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	IsAdmin     bool   `json:"is_admin"`
}

type adminChangePasswordRequest struct {
	Password string `json:"password"`
}

type adminCreateNamedRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type adminAddCriterionRequest struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	MinValue    int    `json:"min_value"`
	MaxValue    int    `json:"max_value"`
	SortOrder   int    `json:"sort_order"`
}

type adminAssignPatrolRequest struct {
	PatrolID  string `json:"patrol_id"`
	SortOrder int    `json:"sort_order"`
}

type adminCreateSessionRequest struct {
	ID                string `json:"id"`
	EventID           string `json:"event_id"`
	TemplateID        string `json:"template_id"`
	Name              string `json:"name"`
	Start             string `json:"start"`
	Duration          string `json:"duration"`
	AwardBestPatrol   bool   `json:"award_best_patrol"`
	AwardMostImproved bool   `json:"award_most_improved"`
	PreviousSessionID string `json:"previous_session_id"`
}

type adminUpdateSessionRequest struct {
	AwardBestPatrol   bool   `json:"award_best_patrol"`
	AwardMostImproved bool   `json:"award_most_improved"`
	PreviousSessionID string `json:"previous_session_id"`
}

type adminSeedScoresRequest struct {
	UserID   string `json:"user_id"`
	MinScore int    `json:"min_score"`
	MaxScore int    `json:"max_score"`
}

// Bootstrap returns everything the admin workspace needs.
func (h *AdminHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.admin_bootstrap")
	defer span.End()

	users, err := h.loadUsers(ctx)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch users")
		return
	}
	assignments, err := h.loadUserAssignments(ctx)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch user patrols")
		return
	}
	for i := range users {
		users[i].Patrols = assignments[users[i].ID]
	}

	events, err := h.loadEvents(ctx)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch events")
		return
	}

	templates, err := h.loadTemplates(ctx)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch templates")
		return
	}
	criteriaByTemplate, err := h.loadTemplateCriteria(ctx)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch criteria")
		return
	}
	for i := range templates {
		templates[i].Criteria = criteriaByTemplate[templates[i].ID]
	}

	patrols, err := h.loadPatrols(ctx)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch patrols")
		return
	}

	sessions, err := h.loadSessions(ctx)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not fetch sessions")
		return
	}

	span.SetAttributes(
		attribute.Int("admin.users.count", len(users)),
		attribute.Int("admin.events.count", len(events)),
		attribute.Int("admin.templates.count", len(templates)),
		attribute.Int("admin.patrols.count", len(patrols)),
		attribute.Int("admin.sessions.count", len(sessions)),
	)

	writeJSON(w, http.StatusOK, adminBootstrapJSON{
		Users:     users,
		Events:    events,
		Templates: templates,
		Patrols:   patrols,
		Sessions:  sessions,
	})
}

// CreateUser handles POST /api/admin/users.
func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.admin_create_user")
	defer span.End()

	var req adminCreateUserRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, r, http.StatusBadRequest, "username and password are required")
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}

	existing, err := h.db.GetUserByUsername(ctx, req.Username)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not check user")
		return
	}
	if existing != nil {
		writeError(w, r, http.StatusConflict, "user already exists")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not hash password")
		return
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}

	if _, err := h.db.ExecContext(
		ctx,
		"INSERT INTO users (id, username, password_hash, display_name, is_admin) VALUES ($1, $2, $3, $4, $5)",
		id, req.Username, hash, req.DisplayName, req.IsAdmin,
	); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

// ChangePassword handles PUT /api/admin/users/{user_id}/password.
func (h *AdminHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.PathValue("user_id")

	_, span := tracing.Tracer().Start(ctx, "handler.admin_change_password")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))

	var req adminChangePasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Password == "" {
		writeError(w, r, http.StatusBadRequest, "password is required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not hash password")
		return
	}

	result, err := h.db.ExecContext(ctx, "UPDATE users SET password_hash = $1 WHERE id = $2", hash, userID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not change password")
		return
	}
	rows, err := result.RowsAffected()
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not change password")
		return
	}
	if rows == 0 {
		writeError(w, r, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// CreateEvent handles POST /api/admin/events.
func (h *AdminHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.admin_create_event")
	defer span.End()

	var req adminCreateNamedRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, r, http.StatusBadRequest, "name is required")
		return
	}
	if req.Description == "" {
		req.Description = req.Name
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}
	if _, err := h.db.ExecContext(ctx, "INSERT INTO events (id, name, description) VALUES ($1, $2, $3)", id, req.Name, req.Description); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not create event")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

// CreateTemplate handles POST /api/admin/templates.
func (h *AdminHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.admin_create_template")
	defer span.End()

	var req adminCreateNamedRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, r, http.StatusBadRequest, "name is required")
		return
	}
	if req.Description == "" {
		req.Description = req.Name
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}
	if _, err := h.db.ExecContext(ctx, "INSERT INTO criteria_templates (id, name, description) VALUES ($1, $2, $3)", id, req.Name, req.Description); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not create template")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

// AddCriterion handles POST /api/admin/templates/{template_id}/criteria.
func (h *AdminHandler) AddCriterion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	templateID := r.PathValue("template_id")

	_, span := tracing.Tracer().Start(ctx, "handler.admin_add_criterion")
	defer span.End()
	span.SetAttributes(attribute.String("template.id", templateID))

	var req adminAddCriterionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, r, http.StatusBadRequest, "title is required")
		return
	}
	if req.Description == "" {
		req.Description = req.Title
	}
	if req.MinValue > req.MaxValue {
		writeError(w, r, http.StatusBadRequest, "min_value must be less than or equal to max_value")
		return
	}

	var templateName string
	if err := h.db.QueryRowContext(ctx, "SELECT name FROM criteria_templates WHERE id = $1", templateID).Scan(&templateName); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, r, http.StatusNotFound, "template not found")
			return
		}
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not check template")
		return
	}

	order := req.SortOrder
	if order == 0 {
		if err := h.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(sort_order), 0) FROM criteria WHERE template_id = $1", templateID).Scan(&order); err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not determine criterion order")
			return
		}
		order++
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}
	if _, err := h.db.ExecContext(
		ctx,
		"INSERT INTO criteria (id, template_id, title, description, min_value, max_value, sort_order) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		id, templateID, req.Title, req.Description, req.MinValue, req.MaxValue, order,
	); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not add criterion")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id, "template_name": templateName})
}

// CreatePatrol handles POST /api/admin/patrols.
func (h *AdminHandler) CreatePatrol(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.admin_create_patrol")
	defer span.End()

	var req adminCreateNamedRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, r, http.StatusBadRequest, "name is required")
		return
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}
	if _, err := h.db.ExecContext(ctx, "INSERT INTO patrols (id, name) VALUES ($1, $2)", id, req.Name); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not create patrol")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

// AssignPatrol handles POST /api/admin/users/{user_id}/patrols.
func (h *AdminHandler) AssignPatrol(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.PathValue("user_id")

	_, span := tracing.Tracer().Start(ctx, "handler.admin_assign_patrol")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))

	var req adminAssignPatrolRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PatrolID == "" {
		writeError(w, r, http.StatusBadRequest, "patrol_id is required")
		return
	}

	var userName string
	if err := h.db.QueryRowContext(ctx, "SELECT username FROM users WHERE id = $1", userID).Scan(&userName); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, r, http.StatusNotFound, "user not found")
			return
		}
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not check user")
		return
	}

	var patrolName string
	if err := h.db.QueryRowContext(ctx, "SELECT name FROM patrols WHERE id = $1", req.PatrolID).Scan(&patrolName); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, r, http.StatusNotFound, "patrol not found")
			return
		}
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not check patrol")
		return
	}

	order := req.SortOrder
	if order == 0 {
		if err := h.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(sort_order), 0) FROM user_patrols WHERE user_id = $1", userID).Scan(&order); err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not determine patrol order")
			return
		}
		order++
	}

	if _, err := h.db.ExecContext(
		ctx,
		`INSERT INTO user_patrols (user_id, patrol_id, sort_order)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, patrol_id) DO UPDATE SET sort_order = EXCLUDED.sort_order`,
		userID, req.PatrolID, order,
	); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not assign patrol")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "user": userName, "patrol": patrolName})
}

// CreateSession handles POST /api/admin/sessions.
func (h *AdminHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.admin_create_session")
	defer span.End()

	var req adminCreateSessionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.EventID == "" || req.TemplateID == "" || req.Name == "" {
		writeError(w, r, http.StatusBadRequest, "event_id, template_id and name are required")
		return
	}

	if req.Duration == "" {
		req.Duration = "3h"
	}
	duration, err := time.ParseDuration(req.Duration)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid duration")
		return
	}

	var startsAt time.Time
	switch {
	case req.Start == "", req.Start == "now":
		startsAt = time.Now()
	default:
		startsAt, err = time.Parse(time.RFC3339, req.Start)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid start time")
			return
		}
	}
	endsAt := startsAt.Add(duration)

	var eventName string
	if err := h.db.QueryRowContext(ctx, "SELECT name FROM events WHERE id = $1", req.EventID).Scan(&eventName); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, r, http.StatusNotFound, "event not found")
			return
		}
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not check event")
		return
	}

	var templateName string
	if err := h.db.QueryRowContext(ctx, "SELECT name FROM criteria_templates WHERE id = $1", req.TemplateID).Scan(&templateName); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, r, http.StatusNotFound, "template not found")
			return
		}
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not check template")
		return
	}

	var prevArg any
	if req.PreviousSessionID != "" {
		var prevName string
		if err := h.db.QueryRowContext(ctx, "SELECT name FROM sessions WHERE id = $1", req.PreviousSessionID).Scan(&prevName); err != nil {
			if err == sql.ErrNoRows {
				writeError(w, r, http.StatusNotFound, "previous session not found")
				return
			}
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not check previous session")
			return
		}
		prevArg = req.PreviousSessionID
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}

	if _, err := h.db.ExecContext(
		ctx,
		`INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at, award_best_patrol, award_most_improved, previous_session_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, req.EventID, req.TemplateID, req.Name, startsAt, endsAt, req.AwardBestPatrol, req.AwardMostImproved, prevArg,
	); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not create session")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":            true,
		"id":            id,
		"event_name":    eventName,
		"template_name": templateName,
	})
}

// UpdateSession handles PUT /api/admin/sessions/{session_id}.
func (h *AdminHandler) UpdateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.admin_update_session")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	var req adminUpdateSessionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if _, err := h.db.GetSession(ctx, sessionID); err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}

	var prevArg any
	if req.PreviousSessionID != "" {
		var prevName string
		if err := h.db.QueryRowContext(ctx, "SELECT name FROM sessions WHERE id = $1", req.PreviousSessionID).Scan(&prevName); err != nil {
			if err == sql.ErrNoRows {
				writeError(w, r, http.StatusNotFound, "previous session not found")
				return
			}
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not check previous session")
			return
		}
		prevArg = req.PreviousSessionID
	}

	if _, err := h.db.ExecContext(
		ctx,
		`UPDATE sessions
		 SET award_best_patrol = $2, award_most_improved = $3, previous_session_id = $4
		 WHERE id = $1`,
		sessionID, req.AwardBestPatrol, req.AwardMostImproved, prevArg,
	); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not update session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// SeedScores handles POST /api/admin/sessions/{session_id}/seed-scores.
func (h *AdminHandler) SeedScores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("session_id")

	_, span := tracing.Tracer().Start(ctx, "handler.admin_seed_scores")
	defer span.End()
	span.SetAttributes(attribute.String("session.id", sessionID))

	var req adminSeedScoresRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		writeError(w, r, http.StatusBadRequest, "user_id is required")
		return
	}
	if req.MinScore == 0 {
		req.MinScore = 3
	}
	if req.MaxScore == 0 {
		req.MaxScore = 10
	}
	if req.MinScore > req.MaxScore {
		writeError(w, r, http.StatusBadRequest, "min_score must be less than or equal to max_score")
		return
	}

	if _, err := h.db.GetSession(ctx, sessionID); err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}
	user, err := h.db.GetUserByID(ctx, req.UserID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not check user")
		return
	}
	if user == nil {
		writeError(w, r, http.StatusNotFound, "user not found")
		return
	}

	var templateID string
	if err := h.db.QueryRowContext(ctx, "SELECT template_id FROM sessions WHERE id = $1", sessionID).Scan(&templateID); err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not load session template")
		return
	}

	criteria, err := h.loadTemplateCriterionIDs(ctx, templateID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not load criteria")
		return
	}
	if len(criteria) == 0 {
		writeError(w, r, http.StatusBadRequest, "template has no criteria")
		return
	}

	patrolIDs, err := h.loadUserPatrolIDs(ctx, req.UserID)
	if err != nil {
		tracing.RecordError(ctx, err)
		writeError(w, r, http.StatusInternalServerError, "could not load patrols")
		return
	}
	if len(patrolIDs) == 0 {
		writeError(w, r, http.StatusBadRequest, "user has no assigned patrols")
		return
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	scoreRange := req.MaxScore - req.MinScore + 1
	seeded := 0
	for _, patrolID := range patrolIDs {
		scores := make(map[string]int, len(criteria))
		for _, criterionID := range criteria {
			scores[criterionID] = req.MinScore + rng.Intn(scoreRange)
		}
		if _, err := h.db.CreateSubmission(ctx, req.UserID, sessionID, patrolID, scores, nil); err != nil {
			tracing.RecordError(ctx, err)
			writeError(w, r, http.StatusInternalServerError, "could not seed scores")
			return
		}
		seeded++
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seeded": seeded})
}

func (h *AdminHandler) loadUsers(ctx context.Context) ([]adminUserJSON, error) {
	rows, err := h.db.QueryContext(ctx,
		"SELECT id, username, display_name, is_admin, created_at FROM users ORDER BY created_at ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	var users []adminUserJSON
	for rows.Next() {
		var row adminUserJSON
		var createdAt time.Time
		if err := rows.Scan(&row.ID, &row.Username, &row.DisplayName, &row.IsAdmin, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		row.CreatedAt = createdAt.Format(time.RFC3339)
		users = append(users, row)
	}
	return users, rows.Err()
}

func (h *AdminHandler) loadUserAssignments(ctx context.Context) (map[string][]adminUserPatrolJSON, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT up.user_id, p.id, p.name, up.sort_order
		 FROM user_patrols up
		 JOIN patrols p ON p.id = up.patrol_id
		 ORDER BY up.user_id, up.sort_order`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user patrols: %w", err)
	}
	defer rows.Close()

	assignments := map[string][]adminUserPatrolJSON{}
	for rows.Next() {
		var userID string
		var row adminUserPatrolJSON
		if err := rows.Scan(&userID, &row.PatrolID, &row.PatrolName, &row.SortOrder); err != nil {
			return nil, fmt.Errorf("scanning user patrol assignment: %w", err)
		}
		assignments[userID] = append(assignments[userID], row)
	}
	return assignments, rows.Err()
}

func (h *AdminHandler) loadEvents(ctx context.Context) ([]adminEventJSON, error) {
	rows, err := h.db.QueryContext(ctx,
		"SELECT id, name, description, created_at FROM events ORDER BY created_at ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []adminEventJSON
	for rows.Next() {
		var row adminEventJSON
		var createdAt time.Time
		if err := rows.Scan(&row.ID, &row.Name, &row.Description, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		row.CreatedAt = createdAt.Format(time.RFC3339)
		events = append(events, row)
	}
	return events, rows.Err()
}

func (h *AdminHandler) loadTemplates(ctx context.Context) ([]adminTemplateJSON, error) {
	rows, err := h.db.QueryContext(ctx,
		"SELECT id, name, description, created_at FROM criteria_templates ORDER BY created_at ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("querying templates: %w", err)
	}
	defer rows.Close()

	var templates []adminTemplateJSON
	for rows.Next() {
		var row adminTemplateJSON
		var createdAt time.Time
		if err := rows.Scan(&row.ID, &row.Name, &row.Description, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning template: %w", err)
		}
		row.CreatedAt = createdAt.Format(time.RFC3339)
		templates = append(templates, row)
	}
	return templates, rows.Err()
}

func (h *AdminHandler) loadTemplateCriteria(ctx context.Context) (map[string][]adminCriterionJSON, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT template_id, id, title, description, min_value, max_value, sort_order
		 FROM criteria
		 ORDER BY template_id, sort_order`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying criteria: %w", err)
	}
	defer rows.Close()

	result := map[string][]adminCriterionJSON{}
	for rows.Next() {
		var templateID string
		var row adminCriterionJSON
		if err := rows.Scan(&templateID, &row.ID, &row.Title, &row.Description, &row.MinValue, &row.MaxValue, &row.SortOrder); err != nil {
			return nil, fmt.Errorf("scanning criterion: %w", err)
		}
		result[templateID] = append(result[templateID], row)
	}
	return result, rows.Err()
}

func (h *AdminHandler) loadPatrols(ctx context.Context) ([]adminPatrolJSON, error) {
	rows, err := h.db.QueryContext(ctx,
		"SELECT id, name, created_at FROM patrols ORDER BY created_at ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("querying patrols: %w", err)
	}
	defer rows.Close()

	var patrols []adminPatrolJSON
	for rows.Next() {
		var row adminPatrolJSON
		var createdAt time.Time
		if err := rows.Scan(&row.ID, &row.Name, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning patrol: %w", err)
		}
		row.CreatedAt = createdAt.Format(time.RFC3339)
		patrols = append(patrols, row)
	}
	return patrols, rows.Err()
}

func (h *AdminHandler) loadSessions(ctx context.Context) ([]adminSessionJSON, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT s.id, s.event_id, e.name, s.template_id, ct.name, s.name, s.starts_at, s.ends_at, s.created_at,
		        s.previous_session_id, s.award_best_patrol, s.award_most_improved
		 FROM sessions s
		 JOIN events e ON e.id = s.event_id
		 JOIN criteria_templates ct ON ct.id = s.template_id
		 ORDER BY s.starts_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	var sessions []adminSessionJSON
	for rows.Next() {
		var row adminSessionJSON
		var startsAt time.Time
		var endsAt time.Time
		var createdAt time.Time
		if err := rows.Scan(
			&row.ID,
			&row.EventID,
			&row.EventName,
			&row.TemplateID,
			&row.TemplateName,
			&row.Name,
			&startsAt,
			&endsAt,
			&createdAt,
			&row.PreviousSessionID,
			&row.AwardBestPatrol,
			&row.AwardMostImproved,
		); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		row.StartsAt = startsAt.Format(time.RFC3339)
		row.EndsAt = endsAt.Format(time.RFC3339)
		row.CreatedAt = createdAt.Format(time.RFC3339)
		row.Status = func() string {
			now := time.Now()
			switch {
			case now.Before(startsAt):
				return "UPCOMING"
			case now.After(endsAt):
				return "CLOSED"
			default:
				return "ACTIVE"
			}
		}()
		sessions = append(sessions, row)
	}
	return sessions, rows.Err()
}

func (h *AdminHandler) loadTemplateCriterionIDs(ctx context.Context, templateID string) ([]string, error) {
	rows, err := h.db.QueryContext(ctx,
		"SELECT id FROM criteria WHERE template_id = $1 ORDER BY sort_order",
		templateID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying criteria: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning criterion id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (h *AdminHandler) loadUserPatrolIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := h.db.QueryContext(ctx,
		"SELECT patrol_id FROM user_patrols WHERE user_id = $1 ORDER BY sort_order",
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user patrols: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning patrol id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
