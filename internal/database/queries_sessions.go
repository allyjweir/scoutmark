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

// ─── User Completion Queries ────────────────────────────────────────

// GetUserFinalisedSessionIDs returns the set of session IDs where the given user
// has submitted scores for ALL of their assigned patrols.
func (d *DB) GetUserFinalisedSessionIDs(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT up.user_id, s.id AS session_id,
		        COUNT(up.patrol_id) AS total_patrols,
		        COUNT(sub.id) AS submitted_patrols
		 FROM user_patrols up
		 CROSS JOIN sessions s
		 LEFT JOIN submissions sub
		   ON sub.user_id = up.user_id
		  AND sub.session_id = s.id
		  AND sub.patrol_id = up.patrol_id
		 WHERE up.user_id = ?
		 GROUP BY up.user_id, s.id
		 HAVING total_patrols > 0 AND total_patrols = submitted_patrols`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying finalised sessions: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var uid, sid string
		var total, submitted int
		if err := rows.Scan(&uid, &sid, &total, &submitted); err != nil {
			return nil, fmt.Errorf("scanning finalised session: %w", err)
		}
		result[sid] = true
	}
	return result, rows.Err()
}

// ─── Admin / Progress Queries ───────────────────────────────────────

// UserProgressRow represents a single user's scoring progress for a session.
type UserProgressRow struct {
	UserID      string
	DisplayName string
	PatrolID    string
	PatrolName  string
	Status      string // "not_started", "drafting", "submitted"
}

// GetSessionProgress returns the scoring progress for all users assigned patrols in a session.
// It checks drafts and submissions for each user+patrol combo.
func (d *DB) GetSessionProgress(ctx context.Context, sessionID string) ([]UserProgressRow, error) {
	// Get all users who have patrol assignments (these are the users who can score)
	rows, err := d.QueryContext(ctx,
		`SELECT u.id, u.display_name, up.patrol_id, p.name,
		        CASE
		          WHEN s.id IS NOT NULL THEN 'submitted'
		          WHEN dr.id IS NOT NULL THEN 'drafting'
		          ELSE 'not_started'
		        END AS status
		 FROM users u
		 JOIN user_patrols up ON up.user_id = u.id
		 JOIN patrols p ON p.id = up.patrol_id
		 LEFT JOIN submissions s ON s.user_id = u.id AND s.session_id = ? AND s.patrol_id = up.patrol_id
		 LEFT JOIN drafts dr ON dr.user_id = u.id AND dr.session_id = ? AND dr.patrol_id = up.patrol_id
		 ORDER BY u.display_name, up.sort_order`,
		sessionID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying session progress: %w", err)
	}
	defer rows.Close()

	var progress []UserProgressRow
	for rows.Next() {
		var r UserProgressRow
		if err := rows.Scan(&r.UserID, &r.DisplayName, &r.PatrolID, &r.PatrolName, &r.Status); err != nil {
			return nil, fmt.Errorf("scanning progress row: %w", err)
		}
		progress = append(progress, r)
	}
	return progress, rows.Err()
}
