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

// DraftRow represents a shared draft record (keyed by session+patrol, no user_id).
type DraftRow struct {
	ID        string
	SessionID string
	PatrolID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DraftScoreRow represents a single score within a draft, with attribution.
type DraftScoreRow struct {
	ID           string
	DraftID      string
	CriterionID  string
	Value        int
	Comment      string
	LastEditedBy *string
	LastEditedAt time.Time
}

// GetDraft fetches a shared draft by (session, patrol).
func (d *DB) GetDraft(ctx context.Context, sessionID, patrolID string) (*DraftRow, error) {
	row := d.QueryRowContext(ctx,
		`SELECT id, session_id, patrol_id, created_at, updated_at
		 FROM drafts
		 WHERE session_id = $1 AND patrol_id = $2`,
		sessionID, patrolID,
	)

	dr := &DraftRow{}
	err := row.Scan(&dr.ID, &dr.SessionID, &dr.PatrolID, &dr.CreatedAt, &dr.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning draft: %w", err)
	}
	return dr, nil
}

// GetDraftScores returns all scores for a draft, including attribution.
func (d *DB) GetDraftScores(ctx context.Context, draftID string) ([]DraftScoreRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, draft_id, criterion_id, value, comment, last_edited_by, last_edited_at
		 FROM draft_scores WHERE draft_id = $1`,
		draftID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying draft scores: %w", err)
	}
	defer rows.Close()

	var scores []DraftScoreRow
	for rows.Next() {
		var s DraftScoreRow
		if err := rows.Scan(&s.ID, &s.DraftID, &s.CriterionID, &s.Value, &s.Comment, &s.LastEditedBy, &s.LastEditedAt); err != nil {
			return nil, fmt.Errorf("scanning draft score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// SaveDraft upserts a shared draft and its scores. Creates the draft if it doesn't exist,
// then upserts each score with attribution.
func (d *DB) SaveDraft(ctx context.Context, userID, sessionID, patrolID string, scores map[string]int, comments map[string]string) (*DraftRow, error) {
	var draft *DraftRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		// Upsert the shared draft record (no user_id column)
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
		} else {
			_, err = tx.ExecContext(ctx,
				"UPDATE drafts SET updated_at = NOW() WHERE id = $1", draftID,
			)
			if err != nil {
				return fmt.Errorf("updating draft timestamp: %w", err)
			}
		}

		// Upsert each score with attribution
		for criterionID, value := range scores {
			scoreID := uuid.New().String()
			comment := ""
			if comments != nil {
				comment = comments[criterionID]
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO draft_scores (id, draft_id, criterion_id, value, comment, last_edited_by, last_edited_at)
				 VALUES ($1, $2, $3, $4, $5, $6, NOW())
				 ON CONFLICT (draft_id, criterion_id) DO UPDATE
				   SET value = EXCLUDED.value, comment = EXCLUDED.comment,
				       last_edited_by = EXCLUDED.last_edited_by, last_edited_at = NOW()`,
				scoreID, draftID, criterionID, value, comment, userID,
			)
			if err != nil {
				return fmt.Errorf("upserting draft score for criterion %s: %w", criterionID, err)
			}
		}

		draft = &DraftRow{
			ID:        draftID,
			SessionID: sessionID,
			PatrolID:  patrolID,
			UpdatedAt: time.Now(),
		}
		return nil
	})

	return draft, err
}

// DeleteDraft removes a shared draft and its scores (cascade).
func (d *DB) DeleteDraft(ctx context.Context, sessionID, patrolID string) error {
	_, err := d.ExecContext(ctx,
		"DELETE FROM drafts WHERE session_id = $1 AND patrol_id = $2",
		sessionID, patrolID,
	)
	return err
}

// ─── Submission Queries ─────────────────────────────────────────────

// SubmissionRow represents a finalised submission (keyed by session+patrol).
type SubmissionRow struct {
	ID          string
	SubmittedBy string
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
	ScoredBy     *string
}

// CreateSubmission creates a new submission from scores and deletes the shared draft.
func (d *DB) CreateSubmission(ctx context.Context, submittedBy, sessionID, patrolID string, scores map[string]int, comments map[string]string) (*SubmissionRow, error) {
	var submission *SubmissionRow

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		submissionID := uuid.New().String()

		_, err := tx.ExecContext(ctx,
			`INSERT INTO submissions (id, submitted_by, session_id, patrol_id, locked)
			 VALUES ($1, $2, $3, $4, TRUE)
			 ON CONFLICT (session_id, patrol_id) DO UPDATE SET locked = TRUE, submitted_at = NOW(), submitted_by = $2`,
			submissionID, submittedBy, sessionID, patrolID,
		)
		if err != nil {
			return fmt.Errorf("inserting submission: %w", err)
		}

		// If it was a duplicate key update, get the real ID
		row := tx.QueryRowContext(ctx,
			"SELECT id FROM submissions WHERE session_id = $1 AND patrol_id = $2",
			sessionID, patrolID,
		)
		if err := row.Scan(&submissionID); err != nil {
			return fmt.Errorf("getting submission ID: %w", err)
		}

		// Clear old scores if re-submitting
		_, err = tx.ExecContext(ctx, "DELETE FROM submission_scores WHERE submission_id = $1", submissionID)
		if err != nil {
			return fmt.Errorf("clearing old submission scores: %w", err)
		}

		// Look up attribution data from the shared draft
		var draftID string
		draftRow := tx.QueryRowContext(ctx,
			"SELECT id FROM drafts WHERE session_id = $1 AND patrol_id = $2",
			sessionID, patrolID,
		)
		hasDraft := draftRow.Scan(&draftID) == nil

		scoredByMap := make(map[string]*string)
		if hasDraft {
			attrRows, err := tx.QueryContext(ctx,
				"SELECT criterion_id, last_edited_by FROM draft_scores WHERE draft_id = $1",
				draftID,
			)
			if err == nil {
				for attrRows.Next() {
					var criterionID string
					var editedBy *string
					if err := attrRows.Scan(&criterionID, &editedBy); err == nil {
						scoredByMap[criterionID] = editedBy
					}
				}
				attrRows.Close()
			}
		}

		// Insert new scores
		scoreRows := lo.MapToSlice(scores, func(criterionID string, value int) SubmissionScoreRow {
			comment := ""
			if comments != nil {
				comment = comments[criterionID]
			}
			scoredBy := scoredByMap[criterionID]
			if scoredBy == nil {
				scoredBy = &submittedBy
			}
			return SubmissionScoreRow{
				ID:           uuid.New().String(),
				SubmissionID: submissionID,
				CriterionID:  criterionID,
				Value:        value,
				Comment:      comment,
				ScoredBy:     scoredBy,
			}
		})

		for _, s := range scoreRows {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO submission_scores (id, submission_id, criterion_id, value, comment, scored_by) VALUES ($1, $2, $3, $4, $5, $6)",
				s.ID, s.SubmissionID, s.CriterionID, s.Value, s.Comment, s.ScoredBy,
			)
			if err != nil {
				return fmt.Errorf("inserting submission score: %w", err)
			}
		}

		// Copy per-user comments from draft to submission (before draft deletion cascades them)
		if hasDraft {
			// Clear old submission comments if re-submitting
			_, err = tx.ExecContext(ctx, "DELETE FROM submission_comments WHERE submission_id = $1", submissionID)
			if err != nil {
				return fmt.Errorf("clearing old submission comments: %w", err)
			}
			if err := d.CopyDraftCommentsToSubmission(ctx, tx, sessionID, patrolID, submissionID); err != nil {
				return fmt.Errorf("copying per-user comments: %w", err)
			}
		}

		// Delete the shared draft
		_, err = tx.ExecContext(ctx,
			"DELETE FROM drafts WHERE session_id = $1 AND patrol_id = $2",
			sessionID, patrolID,
		)
		if err != nil {
			return fmt.Errorf("deleting draft: %w", err)
		}

		submission = &SubmissionRow{
			ID:          submissionID,
			SubmittedBy: submittedBy,
			SessionID:   sessionID,
			PatrolID:    patrolID,
			Locked:      true,
			SubmittedAt: time.Now(),
		}
		return nil
	})

	return submission, err
}

// GetSubmissionsForPatrols returns all submissions for a set of patrols in a session.
func (d *DB) GetSubmissionsForPatrols(ctx context.Context, sessionID string, patrolIDs []string) ([]SubmissionRow, error) {
	if len(patrolIDs) == 0 {
		return nil, nil
	}

	query := `SELECT s.id, s.submitted_by, s.session_id, s.patrol_id, p.name, s.locked, s.submitted_at
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 WHERE s.session_id = $1 AND s.patrol_id = ANY($2::text[])
		 ORDER BY s.submitted_at ASC`

	rows, err := d.QueryContext(ctx, query, sessionID, fmt.Sprintf("{%s}", joinIDs(patrolIDs)))
	if err != nil {
		return nil, fmt.Errorf("querying submissions: %w", err)
	}
	defer rows.Close()

	var subs []SubmissionRow
	for rows.Next() {
		var s SubmissionRow
		if err := rows.Scan(&s.ID, &s.SubmittedBy, &s.SessionID, &s.PatrolID, &s.PatrolName, &s.Locked, &s.SubmittedAt); err != nil {
			return nil, fmt.Errorf("scanning submission: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// GetSubmissionScores returns all scores for a submission.
func (d *DB) GetSubmissionScores(ctx context.Context, submissionID string) ([]SubmissionScoreRow, error) {
	rows, err := d.QueryContext(ctx,
		"SELECT id, submission_id, criterion_id, value, comment, scored_by FROM submission_scores WHERE submission_id = $1",
		submissionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission scores: %w", err)
	}
	defer rows.Close()

	var scores []SubmissionScoreRow
	for rows.Next() {
		var s SubmissionScoreRow
		if err := rows.Scan(&s.ID, &s.SubmissionID, &s.CriterionID, &s.Value, &s.Comment, &s.ScoredBy); err != nil {
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
		`SELECT s.id, s.submitted_by, s.session_id, s.patrol_id, p.name, s.locked, s.submitted_at
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 WHERE s.id = $1`,
		submissionID,
	)

	s := &SubmissionRow{}
	if err := row.Scan(&s.ID, &s.SubmittedBy, &s.SessionID, &s.PatrolID, &s.PatrolName, &s.Locked, &s.SubmittedAt); err != nil {
		return nil, fmt.Errorf("scanning unlocked submission: %w", err)
	}
	return s, nil
}

// ListAllSubmissionsForSession returns all submissions for a session.
func (d *DB) ListAllSubmissionsForSession(ctx context.Context, sessionID string) ([]SubmissionRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.id, s.submitted_by, s.session_id, s.patrol_id, p.name, s.locked, s.submitted_at
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
		if err := rows.Scan(&s.ID, &s.SubmittedBy, &s.SessionID, &s.PatrolID, &s.PatrolName, &s.Locked, &s.SubmittedAt); err != nil {
			return nil, fmt.Errorf("scanning submission: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// FinaliseSession converts all shared drafts for a user's assigned patrols into submissions.
// Missing criteria are filled with zeros.
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
			`SELECT p.id, p.name FROM user_subcamps us
			 JOIN patrols p ON p.subcamp_id = us.subcamp_id
			 WHERE us.user_id = $1`,
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

		// Fetch all shared drafts for this session
		draftRows, err := tx.QueryContext(ctx,
			"SELECT id, patrol_id FROM drafts WHERE session_id = $1",
			sessionID,
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
			var di draftInfo
			if err := draftRows.Scan(&di.ID, &di.PatrolID); err != nil {
				draftRows.Close()
				return fmt.Errorf("scanning draft: %w", err)
			}
			draftsByPatrol[di.PatrolID] = di
		}
		draftRows.Close()
		if err := draftRows.Err(); err != nil {
			return err
		}

		// Process every assigned patrol
		for _, patrol := range userPatrols {
			// Skip patrols that already have a submission
			var existingCount int
			if err := tx.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM submissions WHERE session_id = $1 AND patrol_id = $2",
				sessionID, patrol.ID,
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

			// Overlay draft scores if a shared draft exists
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
			}

			// Create the submission
			submissionID := uuid.New().String()
			_, err = tx.ExecContext(ctx,
				"INSERT INTO submissions (id, submitted_by, session_id, patrol_id, locked) VALUES ($1, $2, $3, $4, TRUE)",
				submissionID, userID, sessionID, patrol.ID,
			)
			if err != nil {
				return fmt.Errorf("inserting submission for patrol %s: %w", patrol.ID, err)
			}

			for criterionID, value := range scores {
				_, err := tx.ExecContext(ctx,
					"INSERT INTO submission_scores (id, submission_id, criterion_id, value, comment, scored_by) VALUES ($1, $2, $3, $4, $5, $6)",
					uuid.New().String(), submissionID, criterionID, value, comments[criterionID], userID,
				)
				if err != nil {
					return fmt.Errorf("inserting submission score: %w", err)
				}
			}

			// Copy per-user comments from draft to submission (BEFORE draft deletion cascades them)
			if _, ok := draftsByPatrol[patrol.ID]; ok {
				if err := d.CopyDraftCommentsToSubmission(ctx, tx, sessionID, patrol.ID, submissionID); err != nil {
					return fmt.Errorf("copying per-user comments for patrol %s: %w", patrol.ID, err)
				}
			}

			// Delete the shared draft (cascades to draft_comments)
			if _, ok := draftsByPatrol[patrol.ID]; ok {
				_, err = tx.ExecContext(ctx, "DELETE FROM drafts WHERE id = $1", draftsByPatrol[patrol.ID].ID)
				if err != nil {
					return fmt.Errorf("deleting draft: %w", err)
				}
			}

			submissions = append(submissions, SubmissionRow{
				ID:          submissionID,
				SubmittedBy: userID,
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

// ReviseSession converts all submissions for a user's patrols back into shared drafts.
func (d *DB) ReviseSession(ctx context.Context, userID, sessionID string) error {
	return d.InTx(ctx, func(tx *sql.Tx) error {
		// Get the user's assigned patrols
		patrolRows, err := tx.QueryContext(ctx,
			`SELECT p.id FROM user_subcamps us JOIN patrols p ON p.subcamp_id = us.subcamp_id WHERE us.user_id = $1`, userID,
		)
		if err != nil {
			return fmt.Errorf("querying user patrols: %w", err)
		}
		var patrolIDs []string
		for patrolRows.Next() {
			var id string
			if err := patrolRows.Scan(&id); err != nil {
				patrolRows.Close()
				return fmt.Errorf("scanning patrol: %w", err)
			}
			patrolIDs = append(patrolIDs, id)
		}
		patrolRows.Close()

		// Fetch submissions for these patrols
		subRows, err := tx.QueryContext(ctx,
			fmt.Sprintf(
				"SELECT id, patrol_id FROM submissions WHERE session_id = $1 AND patrol_id IN (%s)",
				placeholders(len(patrolIDs), 2),
			),
			append([]interface{}{sessionID}, toInterfaceSlice(patrolIDs)...)...,
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

		// Delete award selections when revising
		_, err = tx.ExecContext(ctx,
			"DELETE FROM session_awards WHERE user_id = $1 AND session_id = $2",
			userID, sessionID,
		)
		if err != nil {
			return fmt.Errorf("deleting awards during revise: %w", err)
		}

		for _, sub := range subs {
			// Load submission scores with attribution
			scoreRows, err := tx.QueryContext(ctx,
				"SELECT criterion_id, value, comment, scored_by FROM submission_scores WHERE submission_id = $1",
				sub.ID,
			)
			if err != nil {
				return fmt.Errorf("querying submission scores: %w", err)
			}

			type scoreWithAttr struct {
				Value    int
				Comment  string
				ScoredBy *string
			}
			scores := make(map[string]scoreWithAttr)
			for scoreRows.Next() {
				var criterionID string
				var s scoreWithAttr
				if err := scoreRows.Scan(&criterionID, &s.Value, &s.Comment, &s.ScoredBy); err != nil {
					scoreRows.Close()
					return fmt.Errorf("scanning submission score: %w", err)
				}
				scores[criterionID] = s
			}
			scoreRows.Close()

			// Create a shared draft (or update if one exists)
			var draftID string
			row := tx.QueryRowContext(ctx,
				"SELECT id FROM drafts WHERE session_id = $1 AND patrol_id = $2",
				sessionID, sub.PatrolID,
			)
			err = row.Scan(&draftID)
			if err == sql.ErrNoRows {
				draftID = uuid.New().String()
				_, err = tx.ExecContext(ctx,
					"INSERT INTO drafts (id, session_id, patrol_id) VALUES ($1, $2, $3)",
					draftID, sessionID, sub.PatrolID,
				)
				if err != nil {
					return fmt.Errorf("inserting draft for patrol %s: %w", sub.PatrolID, err)
				}
			} else if err != nil {
				return fmt.Errorf("checking existing draft: %w", err)
			}

			// Upsert draft scores, preserving attribution
			for criterionID, sc := range scores {
				scoreID := uuid.New().String()
				_, err := tx.ExecContext(ctx,
					`INSERT INTO draft_scores (id, draft_id, criterion_id, value, comment, last_edited_by, last_edited_at)
					 VALUES ($1, $2, $3, $4, $5, $6, NOW())
					 ON CONFLICT (draft_id, criterion_id) DO UPDATE
					   SET value = EXCLUDED.value, comment = EXCLUDED.comment,
					       last_edited_by = EXCLUDED.last_edited_by, last_edited_at = NOW()`,
					scoreID, draftID, criterionID, sc.Value, sc.Comment, sc.ScoredBy,
				)
				if err != nil {
					return fmt.Errorf("upserting draft score: %w", err)
				}
			}

			// Copy submission_comments back to draft_comments
			commentRows, err := tx.QueryContext(ctx,
				"SELECT criterion_id, user_id, display_name, comment, created_at FROM submission_comments WHERE submission_id = $1 AND comment != ''",
				sub.ID,
			)
			if err != nil {
				return fmt.Errorf("querying submission comments for revise: %w", err)
			}
			for commentRows.Next() {
				var criterionID, commentUserID, displayName, commentText string
				var createdAt time.Time
				if err := commentRows.Scan(&criterionID, &commentUserID, &displayName, &commentText, &createdAt); err != nil {
					commentRows.Close()
					return fmt.Errorf("scanning submission comment for revise: %w", err)
				}
				commentID := uuid.New().String()
				_, err := tx.ExecContext(ctx,
					`INSERT INTO draft_comments (id, draft_id, criterion_id, user_id, display_name, comment, created_at, updated_at)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
					 ON CONFLICT (draft_id, criterion_id, user_id) DO UPDATE
					   SET comment = EXCLUDED.comment, display_name = EXCLUDED.display_name, updated_at = NOW()`,
					commentID, draftID, criterionID, commentUserID, displayName, commentText, createdAt,
				)
				if err != nil {
					commentRows.Close()
					return fmt.Errorf("inserting draft comment during revise: %w", err)
				}
			}
			commentRows.Close()

			// Delete the submission (cascade deletes submission_scores and submission_comments)
			_, err = tx.ExecContext(ctx, "DELETE FROM submissions WHERE id = $1", sub.ID)
			if err != nil {
				return fmt.Errorf("deleting submission: %w", err)
			}
		}

		return nil
	})
}

// placeholders generates SQL placeholder strings like "$2,$3,$4" starting at startIdx.
func placeholders(count, startIdx int) string {
	result := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("$%d", startIdx+i)
	}
	return result
}

// toInterfaceSlice converts []string to []interface{}.
func toInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}

// joinIDs joins string IDs with commas.
func joinIDs(ids []string) string {
	result := ""
	for i, id := range ids {
		if i > 0 {
			result += ","
		}
		result += id
	}
	return result
}

// GetSubmissionScoresByPatrol returns the scores for a patrol's submission in a session.
func (d *DB) GetSubmissionScoresByPatrol(ctx context.Context, sessionID, patrolID string) ([]SubmissionScoreRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT ss.id, ss.submission_id, ss.criterion_id, ss.value, ss.comment, ss.scored_by
		 FROM submission_scores ss
		 JOIN submissions s ON s.id = ss.submission_id
		 WHERE s.session_id = $1 AND s.patrol_id = $2`,
		sessionID, patrolID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission scores by patrol: %w", err)
	}
	defer rows.Close()

	var scores []SubmissionScoreRow
	for rows.Next() {
		var s SubmissionScoreRow
		if err := rows.Scan(&s.ID, &s.SubmissionID, &s.CriterionID, &s.Value, &s.Comment, &s.ScoredBy); err != nil {
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

// DeleteSessionAwards removes all award selections for a user+session.
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

// GetPreviousSessionTotals returns per-patrol total scores from a previous session
// for the user's assigned patrols.
func (d *DB) GetPreviousSessionTotals(ctx context.Context, previousSessionID string, patrolIDs []string) ([]PatrolTotalRow, error) {
	if len(patrolIDs) == 0 {
		return nil, nil
	}

	query := fmt.Sprintf(
		`SELECT s.patrol_id, p.name, COALESCE(SUM(ss.value), 0) AS total
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 LEFT JOIN submission_scores ss ON ss.submission_id = s.id
		 WHERE s.session_id = $1 AND s.patrol_id IN (%s)
		 GROUP BY s.patrol_id, p.name
		 ORDER BY p.name`,
		placeholders(len(patrolIDs), 2),
	)

	rows, err := d.QueryContext(ctx, query, append([]interface{}{previousSessionID}, toInterfaceSlice(patrolIDs)...)...)
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

// ─── Admin Queries ──────────────────────────────────────────────────

// AdminUserSubmissionRow represents a single patrol submission with its scores for admin viewing.
type AdminUserSubmissionRow struct {
	PatrolID   string
	PatrolName string
	Scores     []SubmissionScoreRow
}

// GetAdminUserSubmissions returns all submissions for a specific user's patrols in a session.
// In the shared model, looks up submissions by the user's assigned patrols.
func (d *DB) GetAdminUserSubmissions(ctx context.Context, userID, sessionID string) ([]AdminUserSubmissionRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.patrol_id, p.name AS patrol_name,
		        ss.id, ss.submission_id, ss.criterion_id, ss.value, ss.comment
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 JOIN submission_scores ss ON ss.submission_id = s.id
		 JOIN criteria c ON c.id = ss.criterion_id
		 JOIN user_subcamps us ON us.user_id = $1
		 JOIN patrols up ON up.subcamp_id = us.subcamp_id AND up.id = s.patrol_id
		 WHERE s.session_id = $2
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

// GetAllSessionComments returns all non-empty per-user comments across all submissions for a session.
// Reads from the submission_comments table (per-user comment model).
func (d *DB) GetAllSessionComments(ctx context.Context, sessionID string) ([]SessionCommentRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT sc.user_id, sc.display_name,
		        s.patrol_id, p.name, sc.criterion_id, c.title,
		        COALESCE(ss.value, 0), sc.comment
		 FROM submission_comments sc
		 JOIN submissions s ON s.id = sc.submission_id
		 JOIN patrols p ON p.id = s.patrol_id
		 JOIN criteria c ON c.id = sc.criterion_id
		 LEFT JOIN submission_scores ss ON ss.submission_id = s.id AND ss.criterion_id = sc.criterion_id
		 WHERE s.session_id = $1 AND sc.comment != ''
		 ORDER BY p.name, c.sort_order, sc.display_name`,
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

// ─── Report Card Queries ────────────────────────────────────────────

// ReportCardRow represents a single score for the report card table.
type ReportCardRow struct {
	PatrolID    string
	PatrolName  string
	SortOrder   int
	CriterionID string
	Value       int
}

// GetReportCardData returns all submission scores for a session, ordered by
// the requesting user's patrol sort_order. Each row is one (patrol, criterion) score.
func (d *DB) GetReportCardData(ctx context.Context, userID, sessionID string) ([]ReportCardRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT p.id, p.name, us.sort_order, ss.criterion_id, ss.value
		 FROM user_subcamps us
		 JOIN patrols p ON p.subcamp_id = us.subcamp_id
		 JOIN submissions s ON s.session_id = $2 AND s.patrol_id = p.id
		 JOIN submission_scores ss ON ss.submission_id = s.id
		 WHERE us.user_id = $1
		 ORDER BY us.sort_order ASC, ss.criterion_id ASC`,
		userID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying report card data: %w", err)
	}
	defer rows.Close()

	var results []ReportCardRow
	for rows.Next() {
		var r ReportCardRow
		if err := rows.Scan(&r.PatrolID, &r.PatrolName, &r.SortOrder, &r.CriterionID, &r.Value); err != nil {
			return nil, fmt.Errorf("scanning report card row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
