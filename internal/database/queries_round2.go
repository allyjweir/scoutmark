package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Round2FinalistRow struct {
	SubcampID       string
	SubcampName     string
	PatrolID        string
	PatrolName      string
	SelectionSource string
}

// GetRound2CandidatePatrols returns all patrols in a standalone Round 2 session's subcamps.
func (d *DB) GetRound2CandidatePatrols(ctx context.Context, sessionID string) ([]UserPatrolRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT p.id, p.name, p.sort_order, sc.id, sc.name
		 FROM patrols p
		 JOIN subcamps sc ON sc.id = p.subcamp_id
		 JOIN session_subcamps ss ON ss.subcamp_id = sc.id
		 WHERE ss.session_id = $1
		 ORDER BY sc.name ASC, p.sort_order ASC, p.name ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying round 2 candidate patrols: %w", err)
	}
	defer rows.Close()

	var patrols []UserPatrolRow
	for rows.Next() {
		var patrol UserPatrolRow
		if err := rows.Scan(&patrol.PatrolID, &patrol.Name, &patrol.SortOrder, &patrol.SubcampID, &patrol.Subcamp); err != nil {
			return nil, fmt.Errorf("scanning round 2 candidate patrol: %w", err)
		}
		patrols = append(patrols, patrol)
	}
	return patrols, rows.Err()
}

type Round2WinnerRow struct {
	PatrolID    string
	PatrolName  string
	SubcampID   string
	SubcampName string
}

// IsSessionFullySubmitted checks whether every scoped patrol in a session has a submission.
func (d *DB) IsSessionFullySubmitted(ctx context.Context, sessionID string) (bool, error) {
	row := d.QueryRowContext(ctx,
		`SELECT
		   COUNT(p.id) AS total_patrols,
		   COUNT(sub.id) AS submitted_patrols
		 FROM session_subcamps ss
		 JOIN patrols p ON p.subcamp_id = ss.subcamp_id
		   AND (
		     NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = ss.session_id)
		     OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = ss.session_id AND sp.patrol_id = p.id)
		   )
		 LEFT JOIN submissions sub ON sub.session_id = ss.session_id AND sub.patrol_id = p.id
		 WHERE ss.session_id = $1`,
		sessionID,
	)

	var total int
	var submitted int
	if err := row.Scan(&total, &submitted); err != nil {
		return false, fmt.Errorf("checking session completion: %w", err)
	}
	if total == 0 {
		return false, nil
	}
	return total == submitted, nil
}

// GetRound2Finalists returns the currently configured finalists for a round 2 session.
func (d *DB) GetRound2Finalists(ctx context.Context, sessionID string) ([]Round2FinalistRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT sp.subcamp_id, sc.name, sp.patrol_id, p.name,
		        'configured'
		 FROM session_patrols sp
		 JOIN subcamps sc ON sc.id = sp.subcamp_id
		 JOIN patrols p ON p.id = sp.patrol_id
		 WHERE sp.session_id = $1
		 ORDER BY sc.name ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying round 2 finalists: %w", err)
	}
	defer rows.Close()

	var finalists []Round2FinalistRow
	for rows.Next() {
		var row Round2FinalistRow
		if err := rows.Scan(&row.SubcampID, &row.SubcampName, &row.PatrolID, &row.PatrolName, &row.SelectionSource); err != nil {
			return nil, fmt.Errorf("scanning round 2 finalist: %w", err)
		}
		finalists = append(finalists, row)
	}
	return finalists, rows.Err()
}

// SetRound2Finalist sets or replaces the finalist patrol for a subcamp in a round 2 session.
func (d *DB) SetRound2Finalist(ctx context.Context, sessionID, subcampID, patrolID string) error {
	return d.InTx(ctx, func(tx *sql.Tx) error {
		var roundType string
		var lockedAt *time.Time
		if err := tx.QueryRowContext(ctx, `SELECT round_type, locked_at FROM sessions WHERE id = $1`, sessionID).Scan(&roundType, &lockedAt); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("session not found")
			}
			return fmt.Errorf("loading session: %w", err)
		}
		if roundType != "round2" {
			return fmt.Errorf("session is not round 2")
		}
		if lockedAt != nil {
			return fmt.Errorf("round 2 session is locked")
		}

		var submissionCount int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM submissions WHERE session_id = $1", sessionID).Scan(&submissionCount); err != nil {
			return fmt.Errorf("checking existing round 2 submissions: %w", err)
		}
		if submissionCount > 0 {
			return fmt.Errorf("cannot change finalists after scoring has started")
		}

		var patrolSubcampID string
		if err := tx.QueryRowContext(ctx, "SELECT subcamp_id FROM patrols WHERE id = $1", patrolID).Scan(&patrolSubcampID); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("patrol not found")
			}
			return fmt.Errorf("checking patrol subcamp: %w", err)
		}
		if patrolSubcampID != subcampID {
			return fmt.Errorf("patrol does not belong to subcamp")
		}

		var included bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (
				SELECT 1 FROM session_subcamps WHERE session_id = $1 AND subcamp_id = $2
			)`,
			sessionID, subcampID,
		).Scan(&included); err != nil {
			return fmt.Errorf("checking round 2 subcamp: %w", err)
		}
		if !included {
			return fmt.Errorf("subcamp is not part of this round 2 session")
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO session_patrols (session_id, subcamp_id, patrol_id)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (session_id, subcamp_id) DO UPDATE
			   SET patrol_id = EXCLUDED.patrol_id`,
			sessionID, subcampID, patrolID,
		); err != nil {
			return fmt.Errorf("setting round 2 finalist: %w", err)
		}

		return nil
	})
}

// GetRound2Winner returns the current round2 winner selection if present.
func (d *DB) GetRound2Winner(ctx context.Context, sessionID string) (*Round2WinnerRow, error) {
	row := d.QueryRowContext(ctx,
		`SELECT sa.patrol_id, p.name, sc.id, sc.name
		 FROM session_awards sa
		 JOIN patrols p ON p.id = sa.patrol_id
		 JOIN subcamps sc ON sc.id = p.subcamp_id
		 WHERE sa.session_id = $1
		   AND sa.award_type = 'best_patrol'
		 ORDER BY sa.updated_at DESC
		 LIMIT 1`,
		sessionID,
	)

	var winner Round2WinnerRow
	if err := row.Scan(&winner.PatrolID, &winner.PatrolName, &winner.SubcampID, &winner.SubcampName); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("querying round 2 winner: %w", err)
	}

	return &winner, nil
}
