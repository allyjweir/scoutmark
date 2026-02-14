package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
)

// ─── Draft Queries ──────────────────────────────────────────────────

// DraftRow represents a draft record.
type DraftRow struct {
	ID        string
	UserID    string
	SessionID string
	PatrolID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DraftScoreRow represents a single score within a draft.
type DraftScoreRow struct {
	ID          string
	DraftID     string
	CriterionID string
	Value       int
	Comment     string
}

// GetDraft fetches a draft by (user, session, patrol).
func (d *DB) GetDraft(ctx context.Context, userID, sessionID, patrolID string) (*DraftRow, error) {
	row := d.QueryRowContext(ctx,
		`SELECT id, user_id, session_id, patrol_id, created_at, updated_at
		 FROM drafts
		 WHERE user_id = $1 AND session_id = $2 AND patrol_id = $3`,
		userID, sessionID, patrolID,
	)

	dr := &DraftRow{}
	err := row.Scan(&dr.ID, &dr.UserID, &dr.SessionID, &dr.PatrolID, &dr.CreatedAt, &dr.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning draft: %w", err)
	}
	return dr, nil
}

// GetDraftScores returns all scores for a draft.
func (d *DB) GetDraftScores(ctx context.Context, draftID string) ([]DraftScoreRow, error) {
	rows, err := d.QueryContext(ctx,
		"SELECT id, draft_id, criterion_id, value, comment FROM draft_scores WHERE draft_id = $1",
		draftID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying draft scores: %w", err)
	}
	defer rows.Close()

	var scores []DraftScoreRow
	for rows.Next() {
		var s DraftScoreRow
		if err := rows.Scan(&s.ID, &s.DraftID, &s.CriterionID, &s.Value, &s.Comment); err != nil {
			return nil, fmt.Errorf("scanning draft score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// SaveDraft upserts a draft and its scores. Creates the draft if it doesn't exist,
// then upserts each score.
func (d *DB) SaveDraft(ctx context.Context, userID, sessionID, patrolID string, scores map[string]int, comments map[string]string) (*DraftRow, error) {
	var draft *DraftRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		// Upsert the draft record
		var draftID string
		row := tx.QueryRowContext(ctx,
			"SELECT id FROM drafts WHERE user_id = $1 AND session_id = $2 AND patrol_id = $3",
			userID, sessionID, patrolID,
		)
		err := row.Scan(&draftID)
		if err == sql.ErrNoRows {
			draftID = uuid.New().String()
			_, err = tx.ExecContext(ctx,
				"INSERT INTO drafts (id, user_id, session_id, patrol_id) VALUES ($1, $2, $3, $4)",
				draftID, userID, sessionID, patrolID,
			)
			if err != nil {
				return fmt.Errorf("inserting draft: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("checking existing draft: %w", err)
		} else {
			_, err = tx.ExecContext(ctx,
				"UPDATE drafts SET updated_at = NOW() WHERE id = $1", draftID,
			)
			if err != nil {
				return fmt.Errorf("updating draft timestamp: %w", err)
			}
		}

		// Upsert each score
		for criterionID, value := range scores {
			scoreID := uuid.New().String()
			comment := ""
			if comments != nil {
				comment = comments[criterionID]
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO draft_scores (id, draft_id, criterion_id, value, comment) VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT (draft_id, criterion_id) DO UPDATE SET value = EXCLUDED.value, comment = EXCLUDED.comment`,
				scoreID, draftID, criterionID, value, comment,
			)
			if err != nil {
				return fmt.Errorf("upserting draft score for criterion %s: %w", criterionID, err)
			}
		}

		draft = &DraftRow{
			ID:        draftID,
			UserID:    userID,
			SessionID: sessionID,
			PatrolID:  patrolID,
			UpdatedAt: time.Now(),
		}
		return nil
	})

	return draft, err
}

// DeleteDraft removes a draft and its scores (cascade).
func (d *DB) DeleteDraft(ctx context.Context, userID, sessionID, patrolID string) error {
	_, err := d.ExecContext(ctx,
		"DELETE FROM drafts WHERE user_id = $1 AND session_id = $2 AND patrol_id = $3",
		userID, sessionID, patrolID,
	)
	return err
}

// ─── Submission Queries ─────────────────────────────────────────────

// SubmissionRow represents a finalised submission.
type SubmissionRow struct {
	ID          string
	UserID      string
	SessionID   string
	PatrolID    string
	PatrolName  string
	Locked      bool
	SubmittedAt time.Time
}

// SubmissionScoreRow represents a score within a submission.
type SubmissionScoreRow struct {
	ID           string
	SubmissionID string
	CriterionID  string
	Value        int
	Comment      string
}

// CreateSubmission creates a new submission from scores and deletes the draft.
func (d *DB) CreateSubmission(ctx context.Context, userID, sessionID, patrolID string, scores map[string]int, comments map[string]string) (*SubmissionRow, error) {
	var submission *SubmissionRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		submissionID := uuid.New().String()

		_, err := tx.ExecContext(ctx,
			`INSERT INTO submissions (id, user_id, session_id, patrol_id, locked)
			 VALUES ($1, $2, $3, $4, TRUE)
			 ON CONFLICT (user_id, session_id, patrol_id) DO UPDATE SET locked = TRUE, submitted_at = NOW()`,
			submissionID, userID, sessionID, patrolID,
		)
		if err != nil {
			return fmt.Errorf("inserting submission: %w", err)
		}

		// If it was a duplicate key update, get the real ID
		row := tx.QueryRowContext(ctx,
			"SELECT id FROM submissions WHERE user_id = $1 AND session_id = $2 AND patrol_id = $3",
			userID, sessionID, patrolID,
		)
		if err := row.Scan(&submissionID); err != nil {
			return fmt.Errorf("getting submission ID: %w", err)
		}

		// Clear old scores if re-submitting
		_, err = tx.ExecContext(ctx, "DELETE FROM submission_scores WHERE submission_id = $1", submissionID)
		if err != nil {
			return fmt.Errorf("clearing old submission scores: %w", err)
		}

		// Insert new scores
		scoreRows := lo.MapToSlice(scores, func(criterionID string, value int) SubmissionScoreRow {
			comment := ""
			if comments != nil {
				comment = comments[criterionID]
			}
			return SubmissionScoreRow{
				ID:           uuid.New().String(),
				SubmissionID: submissionID,
				CriterionID:  criterionID,
				Value:        value,
				Comment:      comment,
			}
		})

		for _, s := range scoreRows {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO submission_scores (id, submission_id, criterion_id, value, comment) VALUES ($1, $2, $3, $4, $5)",
				s.ID, s.SubmissionID, s.CriterionID, s.Value, s.Comment,
			)
			if err != nil {
				return fmt.Errorf("inserting submission score: %w", err)
			}
		}

		// Delete the draft now that we've submitted
		_, err = tx.ExecContext(ctx,
			"DELETE FROM drafts WHERE user_id = $1 AND session_id = $2 AND patrol_id = $3",
			userID, sessionID, patrolID,
		)
		if err != nil {
			return fmt.Errorf("deleting draft: %w", err)
		}

		submission = &SubmissionRow{
			ID:          submissionID,
			UserID:      userID,
			SessionID:   sessionID,
			PatrolID:    patrolID,
			Locked:      true,
			SubmittedAt: time.Now(),
		}
		return nil
	})

	return submission, err
}

// GetSubmissionsForSession returns all submissions by a user in a session.
func (d *DB) GetSubmissionsForSession(ctx context.Context, userID, sessionID string) ([]SubmissionRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.id, s.user_id, s.session_id, s.patrol_id, p.name, s.locked, s.submitted_at
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 WHERE s.user_id = $1 AND s.session_id = $2
		 ORDER BY s.submitted_at ASC`,
		userID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submissions: %w", err)
	}
	defer rows.Close()

	var subs []SubmissionRow
	for rows.Next() {
		var s SubmissionRow
		if err := rows.Scan(&s.ID, &s.UserID, &s.SessionID, &s.PatrolID, &s.PatrolName, &s.Locked, &s.SubmittedAt); err != nil {
			return nil, fmt.Errorf("scanning submission: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// GetSubmissionScores returns all scores for a submission.
func (d *DB) GetSubmissionScores(ctx context.Context, submissionID string) ([]SubmissionScoreRow, error) {
	rows, err := d.QueryContext(ctx,
		"SELECT id, submission_id, criterion_id, value, comment FROM submission_scores WHERE submission_id = $1",
		submissionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission scores: %w", err)
	}
	defer rows.Close()

	var scores []SubmissionScoreRow
	for rows.Next() {
		var s SubmissionScoreRow
		if err := rows.Scan(&s.ID, &s.SubmissionID, &s.CriterionID, &s.Value, &s.Comment); err != nil {
			return nil, fmt.Errorf("scanning submission score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// UnlockSubmission sets locked=false on a submission (admin only).
func (d *DB) UnlockSubmission(ctx context.Context, submissionID string) (*SubmissionRow, error) {
	_, err := d.ExecContext(ctx, "UPDATE submissions SET locked = FALSE WHERE id = $1", submissionID)
	if err != nil {
		return nil, fmt.Errorf("unlocking submission: %w", err)
	}

	row := d.QueryRowContext(ctx,
		`SELECT s.id, s.user_id, s.session_id, s.patrol_id, p.name, s.locked, s.submitted_at
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 WHERE s.id = $1`,
		submissionID,
	)

	s := &SubmissionRow{}
	if err := row.Scan(&s.ID, &s.UserID, &s.SessionID, &s.PatrolID, &s.PatrolName, &s.Locked, &s.SubmittedAt); err != nil {
		return nil, fmt.Errorf("scanning unlocked submission: %w", err)
	}
	return s, nil
}

// ListAllSubmissionsForSession returns all submissions across all users for a session.
func (d *DB) ListAllSubmissionsForSession(ctx context.Context, sessionID string) ([]SubmissionRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.id, s.user_id, s.session_id, s.patrol_id, p.name, s.locked, s.submitted_at
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 WHERE s.session_id = $1
		 ORDER BY s.submitted_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying all submissions: %w", err)
	}
	defer rows.Close()

	var subs []SubmissionRow
	for rows.Next() {
		var s SubmissionRow
		if err := rows.Scan(&s.ID, &s.UserID, &s.SessionID, &s.PatrolID, &s.PatrolName, &s.Locked, &s.SubmittedAt); err != nil {
			return nil, fmt.Errorf("scanning submission: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// FinaliseSession converts all drafts for a user+session into submissions in one transaction.
// For any patrol the user is assigned to, missing criteria are filled in with zero.
// Patrols with no draft at all are also submitted with all-zero scores.
// Returns the list of newly created submissions.
func (d *DB) FinaliseSession(ctx context.Context, userID, sessionID string) ([]SubmissionRow, error) {
	var submissions []SubmissionRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		// Look up the session's template to get the full criteria list
		var templateID string
		if err := tx.QueryRowContext(ctx,
			"SELECT template_id FROM sessions WHERE id = $1", sessionID,
		).Scan(&templateID); err != nil {
			return fmt.Errorf("looking up session template: %w", err)
		}

		critRows, err := tx.QueryContext(ctx,
			"SELECT id FROM criteria WHERE template_id = $1", templateID,
		)
		if err != nil {
			return fmt.Errorf("querying criteria: %w", err)
		}
		var criterionIDs []string
		for critRows.Next() {
			var id string
			if err := critRows.Scan(&id); err != nil {
				critRows.Close()
				return fmt.Errorf("scanning criterion: %w", err)
			}
			criterionIDs = append(criterionIDs, id)
		}
		critRows.Close()

		// Get all patrols assigned to this user
		patrolRows, err := tx.QueryContext(ctx,
			`SELECT p.id, p.name FROM user_patrols up
			 JOIN patrols p ON p.id = up.patrol_id
			 WHERE up.user_id = $1`,
			userID,
		)
		if err != nil {
			return fmt.Errorf("querying user patrols: %w", err)
		}
		type patrolInfo struct {
			ID   string
			Name string
		}
		var userPatrols []patrolInfo
		for patrolRows.Next() {
			var p patrolInfo
			if err := patrolRows.Scan(&p.ID, &p.Name); err != nil {
				patrolRows.Close()
				return fmt.Errorf("scanning patrol: %w", err)
			}
			userPatrols = append(userPatrols, p)
		}
		patrolRows.Close()

		// Fetch all drafts for this user+session, indexed by patrol
		draftRows, err := tx.QueryContext(ctx,
			"SELECT id, patrol_id FROM drafts WHERE user_id = $1 AND session_id = $2",
			userID, sessionID,
		)
		if err != nil {
			return fmt.Errorf("querying drafts: %w", err)
		}
		type draftInfo struct {
			ID       string
			PatrolID string
		}
		draftsByPatrol := make(map[string]draftInfo)
		for draftRows.Next() {
			var d draftInfo
			if err := draftRows.Scan(&d.ID, &d.PatrolID); err != nil {
				draftRows.Close()
				return fmt.Errorf("scanning draft: %w", err)
			}
			draftsByPatrol[d.PatrolID] = d
		}
		draftRows.Close()
		if err := draftRows.Err(); err != nil {
			return err
		}

		// Process every assigned patrol (even ones with no draft)
		for _, patrol := range userPatrols {
			// Skip patrols that already have a submission
			var existingCount int
			if err := tx.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM submissions WHERE user_id = $1 AND session_id = $2 AND patrol_id = $3",
				userID, sessionID, patrol.ID,
			).Scan(&existingCount); err != nil {
				return fmt.Errorf("checking existing submission: %w", err)
			}
			if existingCount > 0 {
				continue
			}

			// Start with zeros for all criteria
			scores := make(map[string]int, len(criterionIDs))
			comments := make(map[string]string, len(criterionIDs))
			for _, cid := range criterionIDs {
				scores[cid] = 0
			}

			// Overlay draft scores if a draft exists
			if draft, ok := draftsByPatrol[patrol.ID]; ok {
				scoreRows, err := tx.QueryContext(ctx,
					"SELECT criterion_id, value, comment FROM draft_scores WHERE draft_id = $1",
					draft.ID,
				)
				if err != nil {
					return fmt.Errorf("querying draft scores: %w", err)
				}
				for scoreRows.Next() {
					var criterionID string
					var value int
					var comment string
					if err := scoreRows.Scan(&criterionID, &value, &comment); err != nil {
						scoreRows.Close()
						return fmt.Errorf("scanning draft score: %w", err)
					}
					scores[criterionID] = value
					comments[criterionID] = comment
				}
				scoreRows.Close()

				// Delete the draft
				_, err = tx.ExecContext(ctx, "DELETE FROM drafts WHERE id = $1", draft.ID)
				if err != nil {
					return fmt.Errorf("deleting draft: %w", err)
				}
			}

			// Create the submission
			submissionID := uuid.New().String()
			_, err = tx.ExecContext(ctx,
				"INSERT INTO submissions (id, user_id, session_id, patrol_id, locked) VALUES ($1, $2, $3, $4, TRUE)",
				submissionID, userID, sessionID, patrol.ID,
			)
			if err != nil {
				return fmt.Errorf("inserting submission for patrol %s: %w", patrol.ID, err)
			}

			// Insert submission scores (all criteria, defaulting to 0)
			for criterionID, value := range scores {
				_, err := tx.ExecContext(ctx,
					"INSERT INTO submission_scores (id, submission_id, criterion_id, value, comment) VALUES ($1, $2, $3, $4, $5)",
					uuid.New().String(), submissionID, criterionID, value, comments[criterionID],
				)
				if err != nil {
					return fmt.Errorf("inserting submission score: %w", err)
				}
			}

			submissions = append(submissions, SubmissionRow{
				ID:          submissionID,
				UserID:      userID,
				SessionID:   sessionID,
				PatrolID:    patrol.ID,
				PatrolName:  patrol.Name,
				Locked:      true,
				SubmittedAt: time.Now(),
			})
		}

		return nil
	})

	return submissions, err
}

// ReviseSession converts all submissions for a user+session back into drafts so they can be edited.
// Deletes the submissions after creating drafts from them.
func (d *DB) ReviseSession(ctx context.Context, userID, sessionID string) error {
	return d.InTx(ctx, func(tx *sql.Tx) error {
		// Fetch all submissions for this user+session
		subRows, err := tx.QueryContext(ctx,
			"SELECT id, patrol_id FROM submissions WHERE user_id = $1 AND session_id = $2",
			userID, sessionID,
		)
		if err != nil {
			return fmt.Errorf("querying submissions: %w", err)
		}

		type subInfo struct {
			ID       string
			PatrolID string
		}
		var subs []subInfo
		for subRows.Next() {
			var s subInfo
			if err := subRows.Scan(&s.ID, &s.PatrolID); err != nil {
				subRows.Close()
				return fmt.Errorf("scanning submission: %w", err)
			}
			subs = append(subs, s)
		}
		subRows.Close()
		if err := subRows.Err(); err != nil {
			return err
		}

		if len(subs) == 0 {
			return fmt.Errorf("no submissions to revise")
		}

		// Delete award selections when revising (user will re-select after re-scoring)
		_, err = tx.ExecContext(ctx,
			"DELETE FROM session_awards WHERE user_id = $1 AND session_id = $2",
			userID, sessionID,
		)
		if err != nil {
			return fmt.Errorf("deleting awards during revise: %w", err)
		}

		for _, sub := range subs {
			// Load submission scores
			scoreRows, err := tx.QueryContext(ctx,
				"SELECT criterion_id, value, comment FROM submission_scores WHERE submission_id = $1",
				sub.ID,
			)
			if err != nil {
				return fmt.Errorf("querying submission scores: %w", err)
			}

			type scoreWithComment struct {
				Value   int
				Comment string
			}
			scores := make(map[string]scoreWithComment)
			for scoreRows.Next() {
				var criterionID string
				var value int
				var comment string
				if err := scoreRows.Scan(&criterionID, &value, &comment); err != nil {
					scoreRows.Close()
					return fmt.Errorf("scanning submission score: %w", err)
				}
				scores[criterionID] = scoreWithComment{Value: value, Comment: comment}
			}
			scoreRows.Close()

			// Create a draft from the submission scores (or update if one exists)
			var draftID string
			row := tx.QueryRowContext(ctx,
				"SELECT id FROM drafts WHERE user_id = $1 AND session_id = $2 AND patrol_id = $3",
				userID, sessionID, sub.PatrolID,
			)
			err = row.Scan(&draftID)
			if err == sql.ErrNoRows {
				draftID = uuid.New().String()
				_, err = tx.ExecContext(ctx,
					"INSERT INTO drafts (id, user_id, session_id, patrol_id) VALUES ($1, $2, $3, $4)",
					draftID, userID, sessionID, sub.PatrolID,
				)
				if err != nil {
					return fmt.Errorf("inserting draft for patrol %s: %w", sub.PatrolID, err)
				}
			} else if err != nil {
				return fmt.Errorf("checking existing draft: %w", err)
			}

			// Upsert draft scores from submission scores
			for criterionID, sc := range scores {
				scoreID := uuid.New().String()
				_, err := tx.ExecContext(ctx,
					`INSERT INTO draft_scores (id, draft_id, criterion_id, value, comment) VALUES ($1, $2, $3, $4, $5)
					 ON CONFLICT (draft_id, criterion_id) DO UPDATE SET value = EXCLUDED.value, comment = EXCLUDED.comment`,
					scoreID, draftID, criterionID, sc.Value, sc.Comment,
				)
				if err != nil {
					return fmt.Errorf("upserting draft score: %w", err)
				}
			}

			// Delete the submission (cascade deletes submission_scores)
			_, err = tx.ExecContext(ctx,
				"DELETE FROM submissions WHERE id = $1", sub.ID,
			)
			if err != nil {
				return fmt.Errorf("deleting submission: %w", err)
			}
		}

		return nil
	})
}

// GetSubmissionScoresByPatrol returns the scores for a user's submission on a specific patrol.
func (d *DB) GetSubmissionScoresByPatrol(ctx context.Context, userID, sessionID, patrolID string) ([]SubmissionScoreRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT ss.id, ss.submission_id, ss.criterion_id, ss.value, ss.comment
		 FROM submission_scores ss
		 JOIN submissions s ON s.id = ss.submission_id
		 WHERE s.user_id = $1 AND s.session_id = $2 AND s.patrol_id = $3`,
		userID, sessionID, patrolID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission scores by patrol: %w", err)
	}
	defer rows.Close()

	var scores []SubmissionScoreRow
	for rows.Next() {
		var s SubmissionScoreRow
		if err := rows.Scan(&s.ID, &s.SubmissionID, &s.CriterionID, &s.Value, &s.Comment); err != nil {
			return nil, fmt.Errorf("scanning submission score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// ─── Award Queries ──────────────────────────────────────────────────

// SessionAwardRow represents a user's award selection for a session.
type SessionAwardRow struct {
	ID        string
	UserID    string
	SessionID string
	AwardType string // "best_patrol" or "most_improved"
	PatrolID  string
	UpdatedAt time.Time
}

// GetSessionAwards returns all award selections for a user in a session.
func (d *DB) GetSessionAwards(ctx context.Context, userID, sessionID string) ([]SessionAwardRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, user_id, session_id, award_type, patrol_id, updated_at
		 FROM session_awards
		 WHERE user_id = $1 AND session_id = $2
		 ORDER BY award_type`,
		userID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying session awards: %w", err)
	}
	defer rows.Close()

	var awards []SessionAwardRow
	for rows.Next() {
		var a SessionAwardRow
		if err := rows.Scan(&a.ID, &a.UserID, &a.SessionID, &a.AwardType, &a.PatrolID, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning session award: %w", err)
		}
		awards = append(awards, a)
	}
	return awards, rows.Err()
}

// UpsertSessionAward saves or updates a single award selection for a user+session.
func (d *DB) UpsertSessionAward(ctx context.Context, userID, sessionID, awardType, patrolID string) (*SessionAwardRow, error) {
	id := uuid.New().String()
	_, err := d.ExecContext(ctx,
		`INSERT INTO session_awards (id, user_id, session_id, award_type, patrol_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, session_id, award_type) DO UPDATE
		   SET patrol_id = EXCLUDED.patrol_id, updated_at = NOW()`,
		id, userID, sessionID, awardType, patrolID,
	)
	if err != nil {
		return nil, fmt.Errorf("upserting session award: %w", err)
	}

	row := d.QueryRowContext(ctx,
		`SELECT id, user_id, session_id, award_type, patrol_id, updated_at
		 FROM session_awards
		 WHERE user_id = $1 AND session_id = $2 AND award_type = $3`,
		userID, sessionID, awardType,
	)
	a := &SessionAwardRow{}
	if err := row.Scan(&a.ID, &a.UserID, &a.SessionID, &a.AwardType, &a.PatrolID, &a.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scanning upserted award: %w", err)
	}
	return a, nil
}

// DeleteSessionAwards removes all award selections for a user+session (used during revise).
func (d *DB) DeleteSessionAwards(ctx context.Context, userID, sessionID string) error {
	_, err := d.ExecContext(ctx,
		"DELETE FROM session_awards WHERE user_id = $1 AND session_id = $2",
		userID, sessionID,
	)
	return err
}

// PatrolTotalRow represents a patrol's total score from a session.
type PatrolTotalRow struct {
	PatrolID   string
	PatrolName string
	Total      int
}

// GetPreviousSessionTotals returns per-patrol total scores for the current user
// from the previous session (used to calculate "most improved").
func (d *DB) GetPreviousSessionTotals(ctx context.Context, userID, previousSessionID string) ([]PatrolTotalRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.patrol_id, p.name, COALESCE(SUM(ss.value), 0) AS total
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 LEFT JOIN submission_scores ss ON ss.submission_id = s.id
		 WHERE s.user_id = $1 AND s.session_id = $2
		 GROUP BY s.patrol_id, p.name
		 ORDER BY p.name`,
		userID, previousSessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying previous session totals: %w", err)
	}
	defer rows.Close()

	var totals []PatrolTotalRow
	for rows.Next() {
		var t PatrolTotalRow
		if err := rows.Scan(&t.PatrolID, &t.PatrolName, &t.Total); err != nil {
			return nil, fmt.Errorf("scanning patrol total: %w", err)
		}
		totals = append(totals, t)
	}
	return totals, rows.Err()
}

// GetAllSessionAwards returns all award selections for all users in a session (admin view).
func (d *DB) GetAllSessionAwards(ctx context.Context, sessionID string) ([]SessionAwardRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT sa.id, sa.user_id, sa.session_id, sa.award_type, sa.patrol_id, sa.updated_at
		 FROM session_awards sa
		 WHERE sa.session_id = $1
		 ORDER BY sa.user_id, sa.award_type`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying all session awards: %w", err)
	}
	defer rows.Close()

	var awards []SessionAwardRow
	for rows.Next() {
		var a SessionAwardRow
		if err := rows.Scan(&a.ID, &a.UserID, &a.SessionID, &a.AwardType, &a.PatrolID, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning session award: %w", err)
		}
		awards = append(awards, a)
	}
	return awards, rows.Err()
}

// ─── Comment Queries (Admin) ────────────────────────────────────────

// AdminUserSubmissionRow represents a single patrol submission with its scores for admin viewing.
type AdminUserSubmissionRow struct {
	PatrolID   string
	PatrolName string
	Scores     []SubmissionScoreRow
}

// GetAdminUserSubmissions returns all submissions (with scores) for a specific user in a session.
// Admin-only: no ownership check.
func (d *DB) GetAdminUserSubmissions(ctx context.Context, userID, sessionID string) ([]AdminUserSubmissionRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.patrol_id, p.name AS patrol_name,
		        ss.id, ss.submission_id, ss.criterion_id, ss.value, ss.comment
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 JOIN submission_scores ss ON ss.submission_id = s.id
		 JOIN criteria c ON c.id = ss.criterion_id
		 WHERE s.user_id = $1 AND s.session_id = $2
		 ORDER BY p.name, c.sort_order`,
		userID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying admin user submissions: %w", err)
	}
	defer rows.Close()

	patrolMap := make(map[string]*AdminUserSubmissionRow)
	var patrolOrder []string

	for rows.Next() {
		var patrolID, patrolName string
		var sc SubmissionScoreRow
		if err := rows.Scan(&patrolID, &patrolName, &sc.ID, &sc.SubmissionID, &sc.CriterionID, &sc.Value, &sc.Comment); err != nil {
			return nil, fmt.Errorf("scanning admin user submission: %w", err)
		}
		entry, exists := patrolMap[patrolID]
		if !exists {
			entry = &AdminUserSubmissionRow{PatrolID: patrolID, PatrolName: patrolName}
			patrolMap[patrolID] = entry
			patrolOrder = append(patrolOrder, patrolID)
		}
		entry.Scores = append(entry.Scores, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]AdminUserSubmissionRow, 0, len(patrolOrder))
	for _, id := range patrolOrder {
		result = append(result, *patrolMap[id])
	}
	return result, nil
}

// SessionCommentRow represents a single comment left on a submission score.
type SessionCommentRow struct {
	UserID         string
	DisplayName    string
	PatrolID       string
	PatrolName     string
	CriterionID    string
	CriterionTitle string
	Value          int
	Comment        string
}

// GetAllSessionComments returns all non-empty comments across all submissions for a session.
// Used by the admin view to see what commentary users have left.
func (d *DB) GetAllSessionComments(ctx context.Context, sessionID string) ([]SessionCommentRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT u.id, u.display_name, s.patrol_id, p.name, ss.criterion_id, c.title, ss.value, ss.comment
		 FROM submission_scores ss
		 JOIN submissions s ON s.id = ss.submission_id
		 JOIN users u ON u.id = s.user_id
		 JOIN patrols p ON p.id = s.patrol_id
		 JOIN criteria c ON c.id = ss.criterion_id
		 WHERE s.session_id = $1 AND ss.comment != ''
		 ORDER BY p.name, c.sort_order, u.display_name`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying session comments: %w", err)
	}
	defer rows.Close()

	var comments []SessionCommentRow
	for rows.Next() {
		var c SessionCommentRow
		if err := rows.Scan(&c.UserID, &c.DisplayName, &c.PatrolID, &c.PatrolName,
			&c.CriterionID, &c.CriterionTitle, &c.Value, &c.Comment); err != nil {
			return nil, fmt.Errorf("scanning session comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}
