package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ─── Chief Round Types ─────────────────────────────────────────────

// ChiefRoundRow represents a camp chief scoring round.
type ChiefRoundRow struct {
	ID             string
	SessionID      string
	Status         string // pending, scoring, completed
	WinnerPatrolID *string
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

// ChiefRoundPatrolRow represents a winning patrol in a chief round.
type ChiefRoundPatrolRow struct {
	ID           string
	ChiefRoundID string
	PatrolID     string
	PatrolName   string
	SubcampID    string
	SubcampName  string
	TotalScore   int
}

// ChiefScoreRow represents the camp chief's score for a patrol criterion.
type ChiefScoreRow struct {
	ID           string
	ChiefRoundID string
	PatrolID     string
	CriterionID  string
	Value        int
}

// ─── Chief Round Queries ───────────────────────────────────────────

// GetChiefRound returns the chief round for a session, if one exists.
func (d *DB) GetChiefRound(ctx context.Context, sessionID string) (*ChiefRoundRow, error) {
	row := d.QueryRowContext(ctx,
		`SELECT id, session_id, status, winner_patrol_id, created_at, completed_at
		 FROM chief_rounds
		 WHERE session_id = $1`,
		sessionID,
	)

	cr := &ChiefRoundRow{}
	err := row.Scan(&cr.ID, &cr.SessionID, &cr.Status, &cr.WinnerPatrolID, &cr.CreatedAt, &cr.CompletedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("scanning chief round: %w", err)
	}
	return cr, nil
}

// CreateChiefRound creates a chief round for a session by finding the top-scoring
// patrol per subcamp. Returns the created chief round.
func (d *DB) CreateChiefRound(ctx context.Context, sessionID string) (*ChiefRoundRow, error) {
	roundID := uuid.New().String()

	_, err := d.ExecContext(ctx,
		`INSERT INTO chief_rounds (id, session_id, status) VALUES ($1, $2, 'pending')`,
		roundID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting chief round: %w", err)
	}

	// Find the top-scoring patrol per subcamp for this session. Patrol totals
	// aggregate across all scorers; the highest-total patrol in each subcamp
	// enters the chief round.
	rows, err := d.QueryContext(ctx,
		`WITH patrol_totals AS (
			SELECT p.subcamp_id, s.patrol_id, SUM(ss.value) AS total
			FROM submissions s
			JOIN submission_scores ss ON ss.submission_id = s.id
			JOIN patrols p ON p.id = s.patrol_id
			WHERE s.session_id = $1
			GROUP BY p.subcamp_id, s.patrol_id
		),
		ranked AS (
			SELECT subcamp_id, patrol_id, total,
			       ROW_NUMBER() OVER (PARTITION BY subcamp_id ORDER BY total DESC, patrol_id ASC) AS rn
			FROM patrol_totals
		)
		SELECT subcamp_id, patrol_id, total
		FROM ranked
		WHERE rn = 1`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying top patrols: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var subcampID, patrolID string
		var total int
		if err := rows.Scan(&subcampID, &patrolID, &total); err != nil {
			return nil, fmt.Errorf("scanning top patrol: %w", err)
		}

		_, err := d.ExecContext(ctx,
			`INSERT INTO chief_round_patrols (id, chief_round_id, patrol_id, subcamp_id, total_score)
			 VALUES ($1, $2, $3, $4, $5)`,
			uuid.New().String(), roundID, patrolID, subcampID, total,
		)
		if err != nil {
			return nil, fmt.Errorf("inserting chief round patrol: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ChiefRoundRow{
		ID:        roundID,
		SessionID: sessionID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}, nil
}

// GetChiefRoundPatrols returns the patrols in a chief round with names.
func (d *DB) GetChiefRoundPatrols(ctx context.Context, chiefRoundID string) ([]ChiefRoundPatrolRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT crp.id, crp.chief_round_id, crp.patrol_id, p.name, crp.subcamp_id, sc.name, crp.total_score
		 FROM chief_round_patrols crp
		 JOIN patrols p ON p.id = crp.patrol_id
		 JOIN subcamps sc ON sc.id = crp.subcamp_id
		 WHERE crp.chief_round_id = $1
		 ORDER BY sc.name ASC`,
		chiefRoundID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying chief round patrols: %w", err)
	}
	defer rows.Close()

	var patrols []ChiefRoundPatrolRow
	for rows.Next() {
		var p ChiefRoundPatrolRow
		if err := rows.Scan(&p.ID, &p.ChiefRoundID, &p.PatrolID, &p.PatrolName, &p.SubcampID, &p.SubcampName, &p.TotalScore); err != nil {
			return nil, fmt.Errorf("scanning chief round patrol: %w", err)
		}
		patrols = append(patrols, p)
	}
	return patrols, rows.Err()
}

// GetChiefScores returns the camp chief's scores for a chief round.
func (d *DB) GetChiefScores(ctx context.Context, chiefRoundID string) ([]ChiefScoreRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, chief_round_id, patrol_id, criterion_id, value
		 FROM chief_scores
		 WHERE chief_round_id = $1
		 ORDER BY patrol_id, criterion_id`,
		chiefRoundID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying chief scores: %w", err)
	}
	defer rows.Close()

	var scores []ChiefScoreRow
	for rows.Next() {
		var s ChiefScoreRow
		if err := rows.Scan(&s.ID, &s.ChiefRoundID, &s.PatrolID, &s.CriterionID, &s.Value); err != nil {
			return nil, fmt.Errorf("scanning chief score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// SaveChiefScores upserts the camp chief's scores for a patrol in a chief round.
func (d *DB) SaveChiefScores(ctx context.Context, chiefRoundID, patrolID string, scores map[string]int) error {
	for criterionID, value := range scores {
		_, err := d.ExecContext(ctx,
			`INSERT INTO chief_scores (id, chief_round_id, patrol_id, criterion_id, value)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (chief_round_id, patrol_id, criterion_id)
			 DO UPDATE SET value = EXCLUDED.value`,
			uuid.New().String(), chiefRoundID, patrolID, criterionID, value,
		)
		if err != nil {
			return fmt.Errorf("upserting chief score: %w", err)
		}
	}

	// Update status to scoring if still pending
	_, err := d.ExecContext(ctx,
		`UPDATE chief_rounds SET status = 'scoring' WHERE id = $1 AND status = 'pending'`,
		chiefRoundID,
	)
	if err != nil {
		return fmt.Errorf("updating chief round status: %w", err)
	}

	return nil
}

// CompleteChiefRound marks a chief round as completed with a winning patrol.
func (d *DB) CompleteChiefRound(ctx context.Context, chiefRoundID, winnerPatrolID string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE chief_rounds
		 SET status = 'completed', winner_patrol_id = $1, completed_at = NOW()
		 WHERE id = $2`,
		winnerPatrolID, chiefRoundID,
	)
	if err != nil {
		return fmt.Errorf("completing chief round: %w", err)
	}
	return nil
}

// IsSessionFullySubmitted checks if all scorers have submitted for all their patrols.
func (d *DB) IsSessionFullySubmitted(ctx context.Context, sessionID string) (bool, error) {
	var total, submitted int
	err := d.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT p.id) AS total,
		        COUNT(DISTINCT sub.patrol_id) AS submitted
		 FROM user_subcamps us
		 JOIN users u ON u.id = us.user_id
		 JOIN patrols p ON p.subcamp_id = us.subcamp_id
		 LEFT JOIN submissions sub
		   ON sub.session_id = $1 AND sub.patrol_id = p.id
		 WHERE u.role = 'scorer'`,
		sessionID,
	).Scan(&total, &submitted)
	if err != nil {
		return false, fmt.Errorf("checking session completion: %w", err)
	}
	return total > 0 && total == submitted, nil
}
