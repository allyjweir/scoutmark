package database

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
)

// ─── Patrol Queries ─────────────────────────────────────────────────

// UserPatrolRow represents a patrol assigned to a user with ordering.
type UserPatrolRow struct {
	PatrolID  string
	Name      string
	SortOrder int
}

// GetUserPatrols returns the ordered list of patrols for a user.
func (d *DB) GetUserPatrols(ctx context.Context, userID string) ([]UserPatrolRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT p.id, p.name, up.sort_order
		 FROM user_patrols up
		 JOIN patrols p ON p.id = up.patrol_id
		 WHERE up.user_id = ?
		 ORDER BY up.sort_order ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user patrols: %w", err)
	}
	defer rows.Close()

	var patrols []UserPatrolRow
	for rows.Next() {
		var p UserPatrolRow
		if err := rows.Scan(&p.PatrolID, &p.Name, &p.SortOrder); err != nil {
			return nil, fmt.Errorf("scanning patrol: %w", err)
		}
		patrols = append(patrols, p)
	}
	return patrols, rows.Err()
}

// ─── Session Queries ────────────────────────────────────────────────

// SessionRow represents a scoring session.
type SessionDetailRow struct {
	ID         string
	EventID    string
	EventName  string
	TemplateID string
	Name       string
	StartsAt   time.Time
	EndsAt     time.Time
	CreatedAt  time.Time
}

// ComputeStatus derives the session status from time boundaries.
func (s *SessionDetailRow) ComputeStatus() string {
	now := time.Now()
	switch {
	case now.Before(s.StartsAt):
		return "UPCOMING"
	case now.After(s.EndsAt):
		return "CLOSED"
	default:
		return "ACTIVE"
	}
}

// ListSessions returns sessions, optionally filtered by status.
func (d *DB) ListSessions(ctx context.Context, statuses []string) ([]SessionDetailRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.id, s.event_id, e.name, s.template_id, s.name, s.starts_at, s.ends_at, s.created_at
		 FROM sessions s
		 JOIN events e ON e.id = s.event_id
		 ORDER BY s.starts_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionDetailRow
	for rows.Next() {
		var s SessionDetailRow
		if err := rows.Scan(&s.ID, &s.EventID, &s.EventName, &s.TemplateID, &s.Name, &s.StartsAt, &s.EndsAt, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Filter by status if requested
	if len(statuses) > 0 {
		statusSet := lo.SliceToMap(statuses, func(s string) (string, bool) { return s, true })
		sessions = lo.Filter(sessions, func(s SessionDetailRow, _ int) bool {
			return statusSet[s.ComputeStatus()]
		})
	}

	return sessions, nil
}

// GetSession returns a single session by ID.
func (d *DB) GetSession(ctx context.Context, sessionID string) (*SessionDetailRow, error) {
	row := d.QueryRowContext(ctx,
		`SELECT s.id, s.event_id, e.name, s.template_id, s.name, s.starts_at, s.ends_at, s.created_at
		 FROM sessions s
		 JOIN events e ON e.id = s.event_id
		 WHERE s.id = ?`,
		sessionID,
	)

	s := &SessionDetailRow{}
	if err := row.Scan(&s.ID, &s.EventID, &s.EventName, &s.TemplateID, &s.Name, &s.StartsAt, &s.EndsAt, &s.CreatedAt); err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}
	return s, nil
}

// ─── Criteria Template Queries ──────────────────────────────────────

// CriterionRow represents a single scoring criterion.
type CriterionRow struct {
	ID          string
	TemplateID  string
	Title       string
	Description string
	MinValue    int
	MaxValue    int
	SortOrder   int
}

// GetTemplateCriteria returns all criteria for a template, ordered.
func (d *DB) GetTemplateCriteria(ctx context.Context, templateID string) ([]CriterionRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, template_id, title, description, min_value, max_value, sort_order
		 FROM criteria
		 WHERE template_id = ?
		 ORDER BY sort_order ASC`,
		templateID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying criteria: %w", err)
	}
	defer rows.Close()

	var criteria []CriterionRow
	for rows.Next() {
		var c CriterionRow
		if err := rows.Scan(&c.ID, &c.TemplateID, &c.Title, &c.Description, &c.MinValue, &c.MaxValue, &c.SortOrder); err != nil {
			return nil, fmt.Errorf("scanning criterion: %w", err)
		}
		criteria = append(criteria, c)
	}
	return criteria, rows.Err()
}
