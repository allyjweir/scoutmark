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
}

// GetDraft fetches a draft by (user, session, patrol).
func (d *DB) GetDraft(ctx context.Context, userID, sessionID, patrolID string) (*DraftRow, error) {
	row := d.QueryRowContext(ctx,
		`SELECT id, user_id, session_id, patrol_id, created_at, updated_at
		 FROM drafts
		 WHERE user_id = ? AND session_id = ? AND patrol_id = ?`,
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
		"SELECT id, draft_id, criterion_id, value FROM draft_scores WHERE draft_id = ?",
		draftID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying draft scores: %w", err)
	}
	defer rows.Close()

	var scores []DraftScoreRow
	for rows.Next() {
		var s DraftScoreRow
		if err := rows.Scan(&s.ID, &s.DraftID, &s.CriterionID, &s.Value); err != nil {
			return nil, fmt.Errorf("scanning draft score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// SaveDraft upserts a draft and its scores. Creates the draft if it doesn't exist,
// then upserts each score.
func (d *DB) SaveDraft(ctx context.Context, userID, sessionID, patrolID string, scores map[string]int) (*DraftRow, error) {
	var draft *DraftRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		// Upsert the draft record
		var draftID string
		row := tx.QueryRowContext(ctx,
			"SELECT id FROM drafts WHERE user_id = ? AND session_id = ? AND patrol_id = ?",
			userID, sessionID, patrolID,
		)
		err := row.Scan(&draftID)
		if err == sql.ErrNoRows {
			draftID = uuid.New().String()
			_, err = tx.ExecContext(ctx,
				"INSERT INTO drafts (id, user_id, session_id, patrol_id) VALUES (?, ?, ?, ?)",
				draftID, userID, sessionID, patrolID,
			)
			if err != nil {
				return fmt.Errorf("inserting draft: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("checking existing draft: %w", err)
		} else {
			_, err = tx.ExecContext(ctx,
				"UPDATE drafts SET updated_at = NOW() WHERE id = ?", draftID,
			)
			if err != nil {
				return fmt.Errorf("updating draft timestamp: %w", err)
			}
		}

		// Upsert each score
		for criterionID, value := range scores {
			scoreID := uuid.New().String()
			_, err := tx.ExecContext(ctx,
				`INSERT INTO draft_scores (id, draft_id, criterion_id, value) VALUES (?, ?, ?, ?)
				 ON DUPLICATE KEY UPDATE value = VALUES(value)`,
				scoreID, draftID, criterionID, value,
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
		"DELETE FROM drafts WHERE user_id = ? AND session_id = ? AND patrol_id = ?",
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
}

// CreateSubmission creates a new submission from scores and deletes the draft.
func (d *DB) CreateSubmission(ctx context.Context, userID, sessionID, patrolID string, scores map[string]int) (*SubmissionRow, error) {
	var submission *SubmissionRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		submissionID := uuid.New().String()

		_, err := tx.ExecContext(ctx,
			`INSERT INTO submissions (id, user_id, session_id, patrol_id, locked)
			 VALUES (?, ?, ?, ?, TRUE)
			 ON DUPLICATE KEY UPDATE locked = TRUE, submitted_at = NOW()`,
			submissionID, userID, sessionID, patrolID,
		)
		if err != nil {
			return fmt.Errorf("inserting submission: %w", err)
		}

		// If it was a duplicate key update, get the real ID
		row := tx.QueryRowContext(ctx,
			"SELECT id FROM submissions WHERE user_id = ? AND session_id = ? AND patrol_id = ?",
			userID, sessionID, patrolID,
		)
		if err := row.Scan(&submissionID); err != nil {
			return fmt.Errorf("getting submission ID: %w", err)
		}

		// Clear old scores if re-submitting
		_, err = tx.ExecContext(ctx, "DELETE FROM submission_scores WHERE submission_id = ?", submissionID)
		if err != nil {
			return fmt.Errorf("clearing old submission scores: %w", err)
		}

		// Insert new scores
		scoreRows := lo.MapToSlice(scores, func(criterionID string, value int) SubmissionScoreRow {
			return SubmissionScoreRow{
				ID:           uuid.New().String(),
				SubmissionID: submissionID,
				CriterionID:  criterionID,
				Value:        value,
			}
		})

		for _, s := range scoreRows {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO submission_scores (id, submission_id, criterion_id, value) VALUES (?, ?, ?, ?)",
				s.ID, s.SubmissionID, s.CriterionID, s.Value,
			)
			if err != nil {
				return fmt.Errorf("inserting submission score: %w", err)
			}
		}

		// Delete the draft now that we've submitted
		_, err = tx.ExecContext(ctx,
			"DELETE FROM drafts WHERE user_id = ? AND session_id = ? AND patrol_id = ?",
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
		 WHERE s.user_id = ? AND s.session_id = ?
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
		"SELECT id, submission_id, criterion_id, value FROM submission_scores WHERE submission_id = ?",
		submissionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission scores: %w", err)
	}
	defer rows.Close()

	var scores []SubmissionScoreRow
	for rows.Next() {
		var s SubmissionScoreRow
		if err := rows.Scan(&s.ID, &s.SubmissionID, &s.CriterionID, &s.Value); err != nil {
			return nil, fmt.Errorf("scanning submission score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// UnlockSubmission sets locked=false on a submission (admin only).
func (d *DB) UnlockSubmission(ctx context.Context, submissionID string) (*SubmissionRow, error) {
	_, err := d.ExecContext(ctx, "UPDATE submissions SET locked = FALSE WHERE id = ?", submissionID)
	if err != nil {
		return nil, fmt.Errorf("unlocking submission: %w", err)
	}

	row := d.QueryRowContext(ctx,
		`SELECT s.id, s.user_id, s.session_id, s.patrol_id, p.name, s.locked, s.submitted_at
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 WHERE s.id = ?`,
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
		 WHERE s.session_id = ?
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
