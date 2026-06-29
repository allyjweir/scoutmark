package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ─── Draft Comment Queries ──────────────────────────────────────────

// DraftCommentRow represents a single per-user comment on a criterion within a draft.
type DraftCommentRow struct {
	ID          string
	DraftID     string
	CriterionID string
	UserID      string
	DisplayName string
	Comment     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SaveDraftComment upserts a per-user comment on a draft criterion.
// Creates the shared draft if it doesn't already exist.
func (d *DB) SaveDraftComment(ctx context.Context, userID, displayName, sessionID, patrolID, criterionID, comment string) (*DraftCommentRow, error) {
	var result *DraftCommentRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		// Ensure the shared draft exists
		var draftID string
		row := tx.QueryRowContext(ctx,
			"SELECT id FROM drafts WHERE session_id = $1 AND patrol_id = $2",
			sessionID, patrolID,
		)
		err := row.Scan(&draftID)
		if err == sql.ErrNoRows {
			draftID = uuid.New().String()
			_, err = tx.ExecContext(ctx,
				"INSERT INTO drafts (id, session_id, patrol_id) VALUES ($1, $2, $3)",
				draftID, sessionID, patrolID,
			)
			if err != nil {
				return fmt.Errorf("inserting draft: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("checking existing draft: %w", err)
		}

		// Upsert the comment
		id := uuid.New().String()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO draft_comments (id, draft_id, criterion_id, user_id, display_name, comment)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (draft_id, criterion_id, user_id) DO UPDATE
			   SET comment = EXCLUDED.comment, display_name = EXCLUDED.display_name, updated_at = NOW()`,
			id, draftID, criterionID, userID, displayName, comment,
		)
		if err != nil {
			return fmt.Errorf("upserting draft comment: %w", err)
		}

		// Read back the row
		r := tx.QueryRowContext(ctx,
			`SELECT id, draft_id, criterion_id, user_id, display_name, comment, created_at, updated_at
			 FROM draft_comments
			 WHERE draft_id = $1 AND criterion_id = $2 AND user_id = $3`,
			draftID, criterionID, userID,
		)
		result = &DraftCommentRow{}
		if err := r.Scan(&result.ID, &result.DraftID, &result.CriterionID, &result.UserID,
			&result.DisplayName, &result.Comment, &result.CreatedAt, &result.UpdatedAt); err != nil {
			return fmt.Errorf("reading back draft comment: %w", err)
		}
		return nil
	})

	return result, err
}

// DeleteDraftComment removes a user's comment on a specific criterion.
func (d *DB) DeleteDraftComment(ctx context.Context, userID, sessionID, patrolID, criterionID string) error {
	_, err := d.ExecContext(ctx,
		`DELETE FROM draft_comments
		 WHERE user_id = $1
		   AND criterion_id = $2
		   AND draft_id = (SELECT id FROM drafts WHERE session_id = $3 AND patrol_id = $4)`,
		userID, criterionID, sessionID, patrolID,
	)
	return err
}

// GetDraftComments returns all per-user comments for a draft (by session+patrol).
func (d *DB) GetDraftComments(ctx context.Context, sessionID, patrolID string) ([]DraftCommentRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT dc.id, dc.draft_id, dc.criterion_id, dc.user_id, dc.display_name, dc.comment, dc.created_at, dc.updated_at
		 FROM draft_comments dc
		 JOIN drafts d ON d.id = dc.draft_id
		 WHERE d.session_id = $1 AND d.patrol_id = $2 AND dc.comment != ''
		 ORDER BY dc.criterion_id, dc.created_at`,
		sessionID, patrolID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying draft comments: %w", err)
	}
	defer rows.Close()

	var comments []DraftCommentRow
	for rows.Next() {
		var c DraftCommentRow
		if err := rows.Scan(&c.ID, &c.DraftID, &c.CriterionID, &c.UserID, &c.DisplayName,
			&c.Comment, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning draft comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// CopyDraftCommentsToSubmission copies all draft_comments for a patrol into submission_comments.
func (d *DB) CopyDraftCommentsToSubmission(ctx context.Context, tx *sql.Tx, sessionID, patrolID, submissionID string) error {
	// First, get the draft ID
	var draftID string
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM drafts WHERE session_id = $1 AND patrol_id = $2",
		sessionID, patrolID,
	).Scan(&draftID)
	if err == sql.ErrNoRows {
		return nil // no draft, no comments to copy
	}
	if err != nil {
		return fmt.Errorf("looking up draft for comment copy: %w", err)
	}

	// Query all comments for this draft
	rows, err := tx.QueryContext(ctx,
		`SELECT criterion_id, user_id, display_name, comment, created_at
		 FROM draft_comments
		 WHERE draft_id = $1 AND comment != ''`,
		draftID,
	)
	if err != nil {
		return fmt.Errorf("querying draft comments for copy: %w", err)
	}

	type draftComment struct {
		criterionID, userID, displayName, comment string
		createdAt                                 time.Time
	}
	var pending []draftComment
	for rows.Next() {
		var c draftComment
		if err := rows.Scan(&c.criterionID, &c.userID, &c.displayName, &c.comment, &c.createdAt); err != nil {
			rows.Close()
			return fmt.Errorf("scanning draft comment for copy: %w", err)
		}
		pending = append(pending, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, c := range pending {
		id := uuid.New().String()
		_, err := tx.ExecContext(ctx,
			`INSERT INTO submission_comments (id, submission_id, criterion_id, user_id, display_name, comment, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, submissionID, c.criterionID, c.userID, c.displayName, c.comment, c.createdAt,
		)
		if err != nil {
			return fmt.Errorf("inserting submission comment: %w", err)
		}
	}
	return nil
}

// GetSubmissionComments returns all per-user comments for a submission.
func (d *DB) GetSubmissionComments(ctx context.Context, submissionID string) ([]DraftCommentRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, submission_id, criterion_id, user_id, display_name, comment, created_at, created_at
		 FROM submission_comments
		 WHERE submission_id = $1 AND comment != ''
		 ORDER BY criterion_id, created_at`,
		submissionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission comments: %w", err)
	}
	defer rows.Close()

	var comments []DraftCommentRow
	for rows.Next() {
		var c DraftCommentRow
		if err := rows.Scan(&c.ID, &c.DraftID, &c.CriterionID, &c.UserID, &c.DisplayName,
			&c.Comment, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning submission comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// GetSubmissionCommentsByPatrol returns all per-user comments for a patrol's submission.
func (d *DB) GetSubmissionCommentsByPatrol(ctx context.Context, sessionID, patrolID string) ([]DraftCommentRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT sc.id, sc.submission_id, sc.criterion_id, sc.user_id, sc.display_name, sc.comment, sc.created_at, sc.created_at
		 FROM submission_comments sc
		 JOIN submissions s ON s.id = sc.submission_id
		 WHERE s.session_id = $1 AND s.patrol_id = $2 AND sc.comment != ''
		 ORDER BY sc.criterion_id, sc.created_at`,
		sessionID, patrolID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission comments by patrol: %w", err)
	}
	defer rows.Close()

	var comments []DraftCommentRow
	for rows.Next() {
		var c DraftCommentRow
		if err := rows.Scan(&c.ID, &c.DraftID, &c.CriterionID, &c.UserID, &c.DisplayName,
			&c.Comment, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning submission comment by patrol: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// GetSubmissionCommentsBySession returns all per-user submission comments
// for patrols a specific user is assigned to in a session.
func (d *DB) GetSubmissionCommentsBySession(ctx context.Context, userID, sessionID string) ([]DraftCommentRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT sc.id, sc.submission_id, sc.criterion_id, sc.user_id, sc.display_name, sc.comment, sc.created_at, sc.created_at
		 FROM submission_comments sc
		 JOIN submissions s ON s.id = sc.submission_id
		 JOIN user_subcamps us ON us.user_id = $1
		 JOIN patrols up ON up.subcamp_id = us.subcamp_id AND up.id = s.patrol_id
		 WHERE s.session_id = $2 AND sc.comment != ''
		 ORDER BY s.patrol_id, sc.criterion_id, sc.created_at`,
		userID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission comments by session: %w", err)
	}
	defer rows.Close()

	var comments []DraftCommentRow
	for rows.Next() {
		var c DraftCommentRow
		if err := rows.Scan(&c.ID, &c.DraftID, &c.CriterionID, &c.UserID, &c.DisplayName,
			&c.Comment, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning submission comment by session: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}
