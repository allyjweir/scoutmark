package database

import (
	"context"
	"encoding/json"
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
	SubcampID string
	Subcamp   string
}

type CriterionRubricBand struct {
	Label    string   `json:"label"`
	Title    string   `json:"title"`
	MinValue int      `json:"min_value"`
	MaxValue int      `json:"max_value"`
	Bullets  []string `json:"bullets"`
}

// GetSessionPatrolsForUser returns patrols in a session that a user can access.
// Non-admin users see only their subcamp's patrols; admins see all session subcamps.
func (d *DB) GetSessionPatrolsForUser(ctx context.Context, userID, sessionID string, isAdmin bool) ([]UserPatrolRow, error) {
	query := `SELECT p.id, p.name, p.sort_order, sc.id, sc.name
		 FROM patrols p
		 JOIN subcamps sc ON sc.id = p.subcamp_id
		 JOIN session_subcamps ss ON ss.subcamp_id = sc.id
		 WHERE ss.session_id = $1
		   AND (
		     NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = $1)
		     OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = $1 AND sp.patrol_id = p.id)
		   )`
	args := []any{sessionID}

	if !isAdmin {
		query += `
		 AND sc.id = (SELECT subcamp_id FROM users WHERE id = $2)`
		args = append(args, userID)
	}

	query += `
		 ORDER BY sc.name ASC, p.sort_order ASC, p.name ASC`

	rows, err := d.QueryContext(ctx,
		query,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying session patrols: %w", err)
	}
	defer rows.Close()

	var patrols []UserPatrolRow
	for rows.Next() {
		var p UserPatrolRow
		if err := rows.Scan(&p.PatrolID, &p.Name, &p.SortOrder, &p.SubcampID, &p.Subcamp); err != nil {
			return nil, fmt.Errorf("scanning patrol: %w", err)
		}
		patrols = append(patrols, p)
	}
	return patrols, rows.Err()
}

// ─── Session Queries ────────────────────────────────────────────────

// SessionRow represents a scoring session.
type SessionDetailRow struct {
	ID                string
	EventID           string
	EventName         string
	TemplateID        string
	Name              string
	RoundType         string
	SourceSessionID   *string
	StartsAt          time.Time
	EndsAt            time.Time
	LockedAt          *time.Time
	LockedBy          *string
	LockedByName      *string
	CreatedAt         time.Time
	PreviousSessionID *string
	AwardBestPatrol   bool
	AwardMostImproved bool
}

// ComputeStatus derives the session status from time boundaries.
func (s *SessionDetailRow) ComputeStatus() string {
	if s.LockedAt != nil {
		return "LOCKED"
	}

	now := time.Now()
	if s.RoundType == "round2" {
		if now.Before(s.StartsAt) {
			return "UPCOMING"
		}
		return "ACTIVE"
	}

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
		`SELECT s.id, s.event_id, e.name, s.template_id, s.name, s.starts_at, s.ends_at,
		        s.round_type, s.source_session_id,
		        s.locked_at, s.locked_by, lu.display_name,
		        s.created_at,
		        s.previous_session_id, s.award_best_patrol, s.award_most_improved
		 FROM sessions s
		 JOIN events e ON e.id = s.event_id
		 LEFT JOIN users lu ON lu.id = s.locked_by
		 ORDER BY s.starts_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionDetailRow
	for rows.Next() {
		var s SessionDetailRow
		if err := rows.Scan(&s.ID, &s.EventID, &s.EventName, &s.TemplateID, &s.Name, &s.StartsAt, &s.EndsAt,
			&s.RoundType, &s.SourceSessionID,
			&s.LockedAt, &s.LockedBy, &s.LockedByName,
			&s.CreatedAt,
			&s.PreviousSessionID, &s.AwardBestPatrol, &s.AwardMostImproved); err != nil {
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
		`SELECT s.id, s.event_id, e.name, s.template_id, s.name, s.starts_at, s.ends_at,
		        s.round_type, s.source_session_id,
		        s.locked_at, s.locked_by, lu.display_name,
		        s.created_at,
		        s.previous_session_id, s.award_best_patrol, s.award_most_improved
		 FROM sessions s
		 JOIN events e ON e.id = s.event_id
		 LEFT JOIN users lu ON lu.id = s.locked_by
		 WHERE s.id = $1`,
		sessionID,
	)

	s := &SessionDetailRow{}
	if err := row.Scan(&s.ID, &s.EventID, &s.EventName, &s.TemplateID, &s.Name, &s.StartsAt, &s.EndsAt,
		&s.RoundType, &s.SourceSessionID,
		&s.LockedAt, &s.LockedBy, &s.LockedByName,
		&s.CreatedAt,
		&s.PreviousSessionID, &s.AwardBestPatrol, &s.AwardMostImproved); err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}
	return s, nil
}

// LockSession marks a session as locked by an admin/camp chief user.
func (d *DB) LockSession(ctx context.Context, sessionID, lockedByUserID string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE sessions
		 SET locked_at = NOW(), locked_by = $2
		 WHERE id = $1`,
		sessionID, lockedByUserID,
	)
	if err != nil {
		return fmt.Errorf("locking session: %w", err)
	}
	return nil
}

// LockSessionIfUnlocked marks a session as locked if it has not already been locked.
func (d *DB) LockSessionIfUnlocked(ctx context.Context, sessionID, lockedByUserID string) (bool, error) {
	result, err := d.ExecContext(ctx,
		`UPDATE sessions
		 SET locked_at = NOW(), locked_by = $2
		 WHERE id = $1 AND locked_at IS NULL`,
		sessionID, lockedByUserID,
	)
	if err != nil {
		return false, fmt.Errorf("locking session: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking locked session: %w", err)
	}
	return affected > 0, nil
}

// UnlockSession clears a previously applied session-level lock.
func (d *DB) UnlockSession(ctx context.Context, sessionID string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE sessions
		 SET locked_at = NULL, locked_by = NULL
		 WHERE id = $1`,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("unlocking session: %w", err)
	}
	return nil
}

// UpdateSessionTimes updates the opening and closing time of a session.
func (d *DB) UpdateSessionTimes(ctx context.Context, sessionID string, startsAt, endsAt time.Time) error {
	result, err := d.ExecContext(ctx, `UPDATE sessions SET starts_at = $2, ends_at = $3 WHERE id = $1`, sessionID, startsAt, endsAt)
	if err != nil {
		return fmt.Errorf("updating session times: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking updated session: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

// SubcampRow is a subcamp available for user assignment.
type SubcampRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (d *DB) ListSubcamps(ctx context.Context) ([]SubcampRow, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, name FROM subcamps ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("querying subcamps: %w", err)
	}
	defer rows.Close()
	var subcamps []SubcampRow
	for rows.Next() {
		var subcamp SubcampRow
		if err := rows.Scan(&subcamp.ID, &subcamp.Name); err != nil {
			return nil, fmt.Errorf("scanning subcamp: %w", err)
		}
		subcamps = append(subcamps, subcamp)
	}
	return subcamps, rows.Err()
}

// SessionSubcampRow includes lock state for a session's participating subcamps.
type SessionSubcampRow struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	LockedAt *time.Time `json:"locked_at,omitempty"`
	LockedBy *string    `json:"locked_by,omitempty"`
}

func (d *DB) ListSessionSubcamps(ctx context.Context, sessionID string) ([]SessionSubcampRow, error) {
	rows, err := d.QueryContext(ctx, `SELECT sc.id, sc.name, ssl.locked_at, locker.display_name
		FROM session_subcamps ss JOIN subcamps sc ON sc.id = ss.subcamp_id
		LEFT JOIN session_subcamp_locks ssl ON ssl.session_id = ss.session_id AND ssl.subcamp_id = ss.subcamp_id
		LEFT JOIN users locker ON locker.id = ssl.locked_by
		WHERE ss.session_id = $1 ORDER BY sc.name ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying session subcamps: %w", err)
	}
	defer rows.Close()
	var subcamps []SessionSubcampRow
	for rows.Next() {
		var subcamp SessionSubcampRow
		if err := rows.Scan(&subcamp.ID, &subcamp.Name, &subcamp.LockedAt, &subcamp.LockedBy); err != nil {
			return nil, fmt.Errorf("scanning session subcamp: %w", err)
		}
		subcamps = append(subcamps, subcamp)
	}
	return subcamps, rows.Err()
}

func (d *DB) LockSessionSubcamp(ctx context.Context, sessionID, subcampID, userID string) error {
	result, err := d.ExecContext(ctx, `INSERT INTO session_subcamp_locks (session_id, subcamp_id, locked_by)
		SELECT v.session_id, v.subcamp_id, v.locked_by
		FROM (SELECT $1::varchar(36) AS session_id, $2::varchar(36) AS subcamp_id, $3::varchar(36) AS locked_by) v
		WHERE EXISTS (SELECT 1 FROM session_subcamps WHERE session_id = v.session_id AND subcamp_id = v.subcamp_id)
		ON CONFLICT (session_id, subcamp_id) DO UPDATE SET locked_at = NOW(), locked_by = EXCLUDED.locked_by`, sessionID, subcampID, userID)
	if err != nil {
		return fmt.Errorf("locking session subcamp: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking locked subcamp: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("subcamp is not part of this session")
	}
	return nil
}

func (d *DB) UnlockSessionSubcamp(ctx context.Context, sessionID, subcampID string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM session_subcamp_locks WHERE session_id = $1 AND subcamp_id = $2`, sessionID, subcampID)
	if err != nil {
		return fmt.Errorf("unlocking session subcamp: %w", err)
	}
	return nil
}

func (d *DB) IsPatrolScoringLocked(ctx context.Context, sessionID, patrolID string) (bool, error) {
	var locked bool
	err := d.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM session_subcamp_locks ssl JOIN patrols p ON p.subcamp_id = ssl.subcamp_id
		WHERE ssl.session_id = $1 AND p.id = $2)`, sessionID, patrolID).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("checking subcamp scoring lock: %w", err)
	}
	return locked, nil
}

func (d *DB) IsSubcampScoringLocked(ctx context.Context, sessionID, subcampID string) (bool, error) {
	var locked bool
	err := d.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM session_subcamp_locks WHERE session_id = $1 AND subcamp_id = $2)`, sessionID, subcampID).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("checking subcamp scoring lock: %w", err)
	}
	return locked, nil
}

// SessionHasPatrol confirms the patrol is included in the session's configured scope.
func (d *DB) SessionHasPatrol(ctx context.Context, sessionID, patrolID string) (bool, error) {
	var included bool
	err := d.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM patrols p
		JOIN session_subcamps ss ON ss.session_id = $1 AND ss.subcamp_id = p.subcamp_id
		WHERE p.id = $2 AND (
			NOT EXISTS (SELECT 1 FROM session_patrols WHERE session_id = $1)
			OR EXISTS (SELECT 1 FROM session_patrols WHERE session_id = $1 AND patrol_id = p.id)
		))`, sessionID, patrolID).Scan(&included)
	if err != nil {
		return false, fmt.Errorf("checking session patrol: %w", err)
	}
	return included, nil
}

// ─── Criteria Template Queries ──────────────────────────────────────

// CriterionRow represents a single scoring criterion.
type CriterionRow struct {
	ID              string
	TemplateID      string
	Title           string
	Description     string
	MinValue        int
	MaxValue        int
	SortOrder       int
	RubricChecklist []string
	RubricBands     []CriterionRubricBand
}

// GetTemplateCriteria returns all criteria for a template, ordered.
func (d *DB) GetTemplateCriteria(ctx context.Context, templateID string) ([]CriterionRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, template_id, title, description, min_value, max_value, sort_order, rubric_checklist, rubric_bands
		 FROM criteria
		 WHERE template_id = $1
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
		var checklistJSON []byte
		var bandsJSON []byte
		if err := rows.Scan(&c.ID, &c.TemplateID, &c.Title, &c.Description, &c.MinValue, &c.MaxValue, &c.SortOrder, &checklistJSON, &bandsJSON); err != nil {
			return nil, fmt.Errorf("scanning criterion: %w", err)
		}
		if len(checklistJSON) > 0 {
			if err := json.Unmarshal(checklistJSON, &c.RubricChecklist); err != nil {
				return nil, fmt.Errorf("decoding criterion rubric checklist: %w", err)
			}
		}
		if len(bandsJSON) > 0 {
			if err := json.Unmarshal(bandsJSON, &c.RubricBands); err != nil {
				return nil, fmt.Errorf("decoding criterion rubric bands: %w", err)
			}
		}
		criteria = append(criteria, c)
	}
	return criteria, rows.Err()
}

// ─── User Completion Queries ────────────────────────────────────────

// GetUserFinalisedSessionIDs returns the set of session IDs where ALL of the
// user's assigned patrols have been submitted (shared model — no user_id on submissions).
func (d *DB) GetUserFinalisedSessionIDs(ctx context.Context, userID string, isAdmin bool) (map[string]bool, error) {
	query := `SELECT s.id AS session_id,
	        COUNT(p.id) AS total_patrols,
	        COUNT(sub.id) AS submitted_patrols
	 FROM sessions s
	 JOIN session_subcamps ss ON ss.session_id = s.id
	 JOIN patrols p ON p.subcamp_id = ss.subcamp_id
	  AND (
	    NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = s.id)
	    OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = s.id AND sp.patrol_id = p.id)
	  )
	 LEFT JOIN submissions sub
	   ON sub.session_id = s.id
	  AND sub.patrol_id = p.id`
	args := []any{}

	if !isAdmin {
		query += `
	 JOIN users u ON u.id = $1
	 WHERE u.subcamp_id = ss.subcamp_id`
		args = append(args, userID)
	}

	query += `
	 GROUP BY s.id
	 HAVING COUNT(p.id) > 0 AND COUNT(p.id) = COUNT(sub.id)`

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying finalised sessions: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var sid string
		var total, submitted int
		if err := rows.Scan(&sid, &total, &submitted); err != nil {
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
	SubcampID   string
	SubcampName string
	PatrolID    string
	PatrolName  string
	SortOrder   int
	Status      string // "not_started", "drafting", "complete", "submitted"
}

// GetSessionProgress returns the scoring progress for all users assigned patrols in a session.
// Drafts are shared (no user_id), so drafting status is per-patrol not per-user.
func (d *DB) GetSessionProgress(ctx context.Context, sessionID string) ([]UserProgressRow, error) {
	rows, err := d.QueryContext(ctx,
		`WITH criteria_total AS (
			SELECT COUNT(*)::int AS total
			FROM sessions sess
			JOIN criteria c ON c.template_id = sess.template_id
			WHERE sess.id = $1
		)
		SELECT u.id, u.display_name, sc.id, sc.name, p.id, p.name, p.sort_order,
		        CASE
		          WHEN s.id IS NOT NULL THEN 'submitted'
		          WHEN COALESCE(ds.scored_count, 0) >= ct.total AND ct.total > 0 THEN 'complete'
		          WHEN COALESCE(ds.score_count, 0) > 0 THEN 'drafting'
		          ELSE 'not_started'
		        END AS status
		 FROM users u
		 JOIN subcamps sc ON sc.id = u.subcamp_id
		 JOIN session_subcamps ss ON ss.subcamp_id = sc.id AND ss.session_id = $1
		 CROSS JOIN criteria_total ct
		 JOIN patrols p ON p.subcamp_id = sc.id
		  AND (
		    NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = $1)
		    OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = $1 AND sp.patrol_id = p.id)
		  )
		 LEFT JOIN submissions s ON s.session_id = $1 AND s.patrol_id = p.id
		 LEFT JOIN LATERAL (
		   SELECT COUNT(*)::int AS score_count,
		          COUNT(*) FILTER (WHERE dsc.value > 0)::int AS scored_count
		   FROM drafts d
		   JOIN draft_scores dsc ON dsc.draft_id = d.id
		   WHERE d.session_id = $1 AND d.patrol_id = p.id
		 ) ds ON TRUE
		 ORDER BY sc.name ASC, u.display_name ASC, p.sort_order ASC, p.name ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying session progress: %w", err)
	}
	defer rows.Close()

	var progress []UserProgressRow
	for rows.Next() {
		var r UserProgressRow
		if err := rows.Scan(&r.UserID, &r.DisplayName, &r.SubcampID, &r.SubcampName, &r.PatrolID, &r.PatrolName, &r.SortOrder, &r.Status); err != nil {
			return nil, fmt.Errorf("scanning progress row: %w", err)
		}
		progress = append(progress, r)
	}
	return progress, rows.Err()
}
