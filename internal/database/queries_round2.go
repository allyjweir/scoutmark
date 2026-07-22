package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

type Round2FinalistRow struct {
	SubcampID       string
	SubcampName     string
	PatrolID        string
	PatrolName      string
	SelectionSource string
}

type Round2WinnerRow struct {
	PatrolID    string
	PatrolName  string
	SubcampID   string
	SubcampName string
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

type sourceSessionInfo struct {
	ID              string
	EventID         string
	TemplateID      string
	Name            string
	StartsAt        time.Time
	EndsAt          time.Time
	LockedAt        *time.Time
	RoundType       string
	AwardBestPatrol bool
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

// EnsureRound2ForSourceSession creates (or reuses) a round 2 session for a closed round 1 session.
// Returns nil,nil when no action is taken (e.g. not eligible yet).
func (d *DB) EnsureRound2ForSourceSession(ctx context.Context, sourceSessionID string) (*SessionDetailRow, error) {
	var out *SessionDetailRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		source, err := loadSourceSessionForRound2(ctx, tx, sourceSessionID)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}

		if source.RoundType != "regular" || !source.AwardBestPatrol {
			return nil
		}

		isClosed := source.LockedAt != nil || time.Now().After(source.EndsAt)
		if !isClosed {
			return nil
		}

		existing, err := d.loadRound2BySourceSessionTx(ctx, tx, source.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			out = existing
			return nil
		}

		subcampIDs, err := sourceSessionSubcampIDs(ctx, tx, source.ID)
		if err != nil {
			return err
		}

		type finalistPick struct {
			subcampID string
			patrolID  string
			source    string
		}
		picks := make([]finalistPick, 0, len(subcampIDs))
		for _, subcampID := range subcampIDs {
			patrolID, pickSource, err := chooseFinalistPatrol(ctx, tx, source.ID, subcampID)
			if err != nil {
				return err
			}
			if patrolID == "" {
				continue
			}
			log.Printf("round 2 finalist selected: source_session=%s subcamp=%s patrol=%s mechanism=%s", source.ID, subcampID, patrolID, pickSource)
			picks = append(picks, finalistPick{subcampID: subcampID, patrolID: patrolID, source: pickSource})
		}

		if len(picks) == 0 {
			return nil
		}

		round2ID := uuid.New().String()
		startsAt := time.Now().UTC()
		endsAt := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

		_, err = tx.ExecContext(ctx,
			`INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at, award_best_patrol, award_most_improved, previous_session_id, round_type, source_session_id)
			 VALUES ($1, $2, $3, $4, $5, $6, TRUE, FALSE, NULL, 'round2', $7)`,
			round2ID,
			source.EventID,
			source.TemplateID,
			fmt.Sprintf("%s - Round 2", source.Name),
			startsAt,
			endsAt,
			source.ID,
		)
		if err != nil {
			return fmt.Errorf("creating round 2 session: %w", err)
		}

		for _, pick := range picks {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO session_subcamps (session_id, subcamp_id)
				 VALUES ($1, $2)
				 ON CONFLICT (session_id, subcamp_id) DO NOTHING`,
				round2ID, pick.subcampID,
			); err != nil {
				return fmt.Errorf("adding round 2 subcamp %s: %w", pick.subcampID, err)
			}

			if _, err := tx.ExecContext(ctx,
				`INSERT INTO session_patrols (session_id, subcamp_id, patrol_id)
				 VALUES ($1, $2, $3)`,
				round2ID, pick.subcampID, pick.patrolID,
			); err != nil {
				return fmt.Errorf("adding round 2 finalist %s: %w", pick.patrolID, err)
			}
		}

		created, err := d.getSessionTx(ctx, tx, round2ID)
		if err != nil {
			return err
		}
		out = created
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
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
		session, err := d.getSessionTx(ctx, tx, sessionID)
		if err != nil {
			return err
		}
		if session.RoundType != "round2" {
			return fmt.Errorf("session is not round 2")
		}
		if session.LockedAt != nil {
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

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO session_subcamps (session_id, subcamp_id)
			 VALUES ($1, $2)
			 ON CONFLICT (session_id, subcamp_id) DO NOTHING`,
			sessionID, subcampID,
		); err != nil {
			return fmt.Errorf("ensuring round 2 subcamp: %w", err)
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

func loadSourceSessionForRound2(ctx context.Context, tx *sql.Tx, sessionID string) (*sourceSessionInfo, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id, event_id, template_id, name, starts_at, ends_at, locked_at, round_type, award_best_patrol
		 FROM sessions
		 WHERE id = $1`,
		sessionID,
	)

	var s sourceSessionInfo
	if err := row.Scan(&s.ID, &s.EventID, &s.TemplateID, &s.Name, &s.StartsAt, &s.EndsAt, &s.LockedAt, &s.RoundType, &s.AwardBestPatrol); err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) loadRound2BySourceSessionTx(ctx context.Context, tx *sql.Tx, sourceSessionID string) (*SessionDetailRow, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id
		 FROM sessions
		 WHERE source_session_id = $1 AND round_type = 'round2'`,
		sourceSessionID,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("checking existing round 2 session: %w", err)
	}
	return d.getSessionTx(ctx, tx, id)
}

func sourceSessionSubcampIDs(ctx context.Context, tx *sql.Tx, sessionID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT subcamp_id
		 FROM session_subcamps
		 WHERE session_id = $1
		 ORDER BY subcamp_id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying source session subcamps: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var subcampID string
		if err := rows.Scan(&subcampID); err != nil {
			return nil, fmt.Errorf("scanning source session subcamp: %w", err)
		}
		out = append(out, subcampID)
	}
	return out, rows.Err()
}

func chooseFinalistPatrol(ctx context.Context, tx *sql.Tx, sessionID, subcampID string) (string, string, error) {
	if patrolID, err := chooseAwardWinner(ctx, tx, sessionID, subcampID); err != nil {
		return "", "", err
	} else if patrolID != "" {
		return patrolID, "award", nil
	}

	if patrolID, err := chooseHighestScoringPatrol(ctx, tx, sessionID, subcampID); err != nil {
		return "", "", err
	} else if patrolID != "" {
		return patrolID, "score", nil
	}

	return "", "", nil
}

func chooseAwardWinner(ctx context.Context, tx *sql.Tx, sessionID, subcampID string) (string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT sa.patrol_id, COUNT(*) AS votes
		 FROM session_awards sa
		 JOIN patrols p ON p.id = sa.patrol_id
		 WHERE sa.session_id = $1
		   AND sa.award_type = 'best_patrol'
		   AND p.subcamp_id = $2
		   AND (
		     NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = $1)
		     OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = $1 AND sp.patrol_id = p.id)
		   )
		 GROUP BY sa.patrol_id
		 ORDER BY votes DESC, sa.patrol_id ASC`,
		sessionID, subcampID,
	)
	if err != nil {
		return "", fmt.Errorf("querying best patrol award votes: %w", err)
	}
	defer rows.Close()

	type voteRow struct {
		patrolID string
		votes    int
	}
	all := []voteRow{}
	for rows.Next() {
		var row voteRow
		if err := rows.Scan(&row.patrolID, &row.votes); err != nil {
			return "", fmt.Errorf("scanning award vote row: %w", err)
		}
		all = append(all, row)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "", nil
	}

	topVotes := all[0].votes
	tied := make([]string, 0, len(all))
	for _, row := range all {
		if row.votes != topVotes {
			break
		}
		tied = append(tied, row.patrolID)
	}
	return tied[rand.Intn(len(tied))], nil
}

func chooseHighestScoringPatrol(ctx context.Context, tx *sql.Tx, sessionID, subcampID string) (string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT sub.patrol_id, COALESCE(SUM(ss.value), 0) AS total
		 FROM submissions sub
		 JOIN patrols p ON p.id = sub.patrol_id
		 LEFT JOIN submission_scores ss ON ss.submission_id = sub.id
		 WHERE sub.session_id = $1
		   AND p.subcamp_id = $2
		   AND (
		     NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = $1)
		     OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = $1 AND sp.patrol_id = p.id)
		   )
		 GROUP BY sub.patrol_id
		 ORDER BY total DESC, sub.patrol_id ASC`,
		sessionID, subcampID,
	)
	if err != nil {
		return "", fmt.Errorf("querying highest scoring patrol: %w", err)
	}
	defer rows.Close()

	type scoreRow struct {
		patrolID string
		total    int
	}
	all := []scoreRow{}
	for rows.Next() {
		var row scoreRow
		if err := rows.Scan(&row.patrolID, &row.total); err != nil {
			return "", fmt.Errorf("scanning patrol score row: %w", err)
		}
		all = append(all, row)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "", nil
	}

	topTotal := all[0].total
	tied := make([]string, 0, len(all))
	for _, row := range all {
		if row.total != topTotal {
			break
		}
		tied = append(tied, row.patrolID)
	}
	return tied[rand.Intn(len(tied))], nil
}

func (d *DB) getSessionTx(ctx context.Context, tx *sql.Tx, sessionID string) (*SessionDetailRow, error) {
	row := tx.QueryRowContext(ctx,
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
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session %q not found", sessionID)
		}
		return nil, fmt.Errorf("loading session %q: %w", sessionID, err)
	}
	return s, nil
}
