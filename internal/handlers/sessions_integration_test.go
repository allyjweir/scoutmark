package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
)

func TestFinaliseSessionAsAdminForSubcamp(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()

	mustExec(t, db, `INSERT INTO events (id, name, description) VALUES ('event', 'Event', '')`)
	mustExec(t, db, `INSERT INTO criteria_templates (id, name, description) VALUES ('template', 'Template', '')`)
	mustExec(t, db, `INSERT INTO criteria (id, template_id, title, description, max_value) VALUES
		('criterion-1', 'template', 'Criterion 1', '', 5),
		('criterion-2', 'template', 'Criterion 2', '', 5)`)
	mustExec(t, db, `INSERT INTO subcamps (id, name) VALUES ('alpha', 'Alpha'), ('bravo', 'Bravo')`)
	mustExec(t, db, `INSERT INTO users (id, username, password_hash, display_name, is_admin, password_change_required)
		VALUES ('admin', 'admin', 'hash', 'Administrator', TRUE, FALSE)`)
	mustExec(t, db, `INSERT INTO user_sessions (token, user_id, expires_at) VALUES ('admin-token', 'admin', $1)`, time.Now().Add(time.Hour))
	mustExec(t, db, `INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at)
		VALUES ('session', 'event', 'template', 'Session', $1, $2)`, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	mustExec(t, db, `INSERT INTO session_subcamps (session_id, subcamp_id) VALUES ('session', 'alpha'), ('session', 'bravo')`)
	mustExec(t, db, `INSERT INTO patrols (id, name, subcamp_id) VALUES
		('alpha-1', 'Alpha 1', 'alpha'), ('alpha-2', 'Alpha 2', 'alpha'), ('bravo-1', 'Bravo 1', 'bravo')`)
	mustExec(t, db, `INSERT INTO drafts (id, session_id, patrol_id) VALUES ('draft', 'session', 'alpha-1')`)
	mustExec(t, db, `INSERT INTO draft_scores (id, draft_id, criterion_id, value) VALUES ('draft-score', 'draft', 'criterion-1', 4)`)

	handler := NewSessionHandler(db, nil)
	mux := http.NewServeMux()
	mux.Handle("POST /api/sessions/{session_id}/finalise", auth.Middleware(db)(http.HandlerFunc(handler.FinaliseSession)))

	request := httptest.NewRequest(http.MethodPost, "/api/sessions/session/finalise",
		bytes.NewBufferString(`{"subcamp_id":"alpha"}`))
	request.Header.Set("Authorization", "Bearer "+"admin-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("finalise response = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result struct {
		FinalisedCount int `json:"finalised_count"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if result.FinalisedCount != 2 {
		t.Fatalf("finalised_count = %d, want 2", result.FinalisedCount)
	}

	assertSubmission(t, db, "session", "alpha-1", map[string]int{"criterion-1": 4, "criterion-2": 0})
	assertSubmission(t, db, "session", "alpha-2", map[string]int{"criterion-1": 0, "criterion-2": 0})
	var bravoSubmissions int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM submissions WHERE session_id = 'session' AND patrol_id = 'bravo-1'`).Scan(&bravoSubmissions); err != nil {
		t.Fatalf("counting bravo submissions: %v", err)
	}
	if bravoSubmissions != 0 {
		t.Fatalf("bravo submissions = %d, want 0", bravoSubmissions)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/sessions/session/revise",
		bytes.NewBufferString(`{"subcamp_id":"alpha"}`))
	request.Header.Set("Authorization", "Bearer "+"admin-token")
	response = httptest.NewRecorder()
	mux.Handle("POST /api/sessions/{session_id}/revise", auth.Middleware(db)(http.HandlerFunc(handler.ReviseSession)))
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("revise response = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var alphaSubmissions int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM submissions WHERE session_id = 'session' AND patrol_id IN ('alpha-1', 'alpha-2')`).Scan(&alphaSubmissions); err != nil {
		t.Fatalf("counting alpha submissions: %v", err)
	}
	if alphaSubmissions != 0 {
		t.Fatalf("alpha submissions after revise = %d, want 0", alphaSubmissions)
	}

	mustExec(t, db, `INSERT INTO session_subcamp_locks (session_id, subcamp_id, locked_by) VALUES ('session', 'bravo', 'admin')`)
	request = httptest.NewRequest(http.MethodPost, "/api/sessions/session/finalise",
		bytes.NewBufferString(`{"subcamp_id":"bravo"}`))
	request.Header.Set("Authorization", "Bearer "+"admin-token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusLocked {
		t.Fatalf("locked finalise response = %d, want %d: %s", response.Code, http.StatusLocked, response.Body.String())
	}
}

func newIntegrationDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	db := &database.DB{DB: sqlDB}
	t.Cleanup(func() { db.Close() })

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	if _, err := db.ExecContext(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("dropping test schema: %v", err)
		}
	})
	if _, err := db.ExecContext(context.Background(), "SET search_path TO "+schema); err != nil {
		t.Fatalf("setting test schema: %v", err)
	}
	if err := db.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrating test schema: %v", err)
	}
	return db
}

func mustExec(t *testing.T, db *database.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("executing query: %v\n%s", err, query)
	}
}

func assertSubmission(t *testing.T, db *database.DB, sessionID, patrolID string, wantScores map[string]int) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `SELECT ss.criterion_id, ss.value
		FROM submissions s JOIN submission_scores ss ON ss.submission_id = s.id
		WHERE s.session_id = $1 AND s.patrol_id = $2`, sessionID, patrolID)
	if err != nil {
		t.Fatalf("querying submission scores: %v", err)
	}
	defer rows.Close()

	gotScores := make(map[string]int)
	for rows.Next() {
		var criterionID string
		var value int
		if err := rows.Scan(&criterionID, &value); err != nil {
			t.Fatalf("scanning submission score: %v", err)
		}
		gotScores[criterionID] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating submission scores: %v", err)
	}
	if len(gotScores) != len(wantScores) {
		t.Fatalf("scores = %v, want %v", gotScores, wantScores)
	}
	for criterionID, wantValue := range wantScores {
		if gotScores[criterionID] != wantValue {
			t.Fatalf("score for %s = %d, want %d", criterionID, gotScores[criterionID], wantValue)
		}
	}
}
