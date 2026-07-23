package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/allyjweir/scoutmark/internal/tracing"
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
		`SELECT id, draft_id, criterion_id, value, last_edited_by, last_edited_at
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
		if err := rows.Scan(&s.ID, &s.DraftID, &s.CriterionID, &s.Value, &s.LastEditedBy, &s.LastEditedAt); err != nil {
			return nil, fmt.Errorf("scanning draft score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// SaveDraft upserts a shared draft and its scores. Creates the draft if it doesn't exist,
// then upserts each score with attribution.
func (d *DB) SaveDraft(ctx context.Context, userID, sessionID, patrolID string, scores map[string]int) (*DraftRow, error) {
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
			_, err := tx.ExecContext(ctx,
				`INSERT INTO draft_scores (id, draft_id, criterion_id, value, last_edited_by, last_edited_at)
				 VALUES ($1, $2, $3, $4, $5, NOW())
				 ON CONFLICT (draft_id, criterion_id) DO UPDATE
				   SET value = EXCLUDED.value,
				       last_edited_by = EXCLUDED.last_edited_by, last_edited_at = NOW()`,
				scoreID, draftID, criterionID, value, userID,
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
	ScoredBy     *string
}

// PatrolHistoryRow represents a patrol score in a historical session.
type PatrolHistoryRow struct {
	PatrolID        string
	PatrolName      string
	PatrolSortOrder int
	SubmissionID    *string
	SessionID       *string
	SessionName     *string
	SessionStartsAt *time.Time
	SubmittedAt     *time.Time
	CriterionID     *string
	CriterionTitle  *string
	CriterionMin    *int
	CriterionMax    *int
	CriterionOrder  *int
	Value           *int
}

// PatrolHistoryCommentRow represents a comment made on a historical score.
type PatrolHistoryCommentRow struct {
	ID           string
	SubmissionID string
	CriterionID  string
	DisplayName  string
	Comment      string
}

// GetPatrolHistory returns the patrols a user can access and their submitted scores.
func (d *DB) GetPatrolHistory(ctx context.Context, subcampID *string, isAdmin bool) ([]PatrolHistoryRow, []PatrolHistoryCommentRow, error) {
	if !isAdmin && subcampID == nil {
		return []PatrolHistoryRow{}, []PatrolHistoryCommentRow{}, nil
	}

	scope := "TRUE"
	var args []any
	if !isAdmin {
		scope = `p.subcamp_id = $1`
		args = []any{*subcampID}
	}

	rows, err := d.QueryContext(ctx,
		`SELECT p.id, p.name, p.sort_order,
		        sb.id, se.id, se.name, se.starts_at, sb.submitted_at,
		        ss.criterion_id, c.title, c.min_value, c.max_value, c.sort_order, ss.value
		 FROM patrols p
		 LEFT JOIN submissions sb ON sb.patrol_id = p.id
		 LEFT JOIN sessions se ON se.id = sb.session_id
		 LEFT JOIN submission_scores ss ON ss.submission_id = sb.id
		 LEFT JOIN criteria c ON c.id = ss.criterion_id
		 WHERE `+scope+`
		 ORDER BY p.sort_order ASC, p.name ASC, se.starts_at DESC NULLS LAST, c.sort_order ASC`,
		args...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("querying patrol history: %w", err)
	}
	defer rows.Close()

	var history []PatrolHistoryRow
	for rows.Next() {
		var row PatrolHistoryRow
		if err := rows.Scan(
			&row.PatrolID, &row.PatrolName, &row.PatrolSortOrder,
			&row.SubmissionID, &row.SessionID, &row.SessionName, &row.SessionStartsAt, &row.SubmittedAt,
			&row.CriterionID, &row.CriterionTitle, &row.CriterionMin, &row.CriterionMax, &row.CriterionOrder, &row.Value,
		); err != nil {
			return nil, nil, fmt.Errorf("scanning patrol history: %w", err)
		}
		history = append(history, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	commentRows, err := d.QueryContext(ctx,
		`SELECT sc.id, sc.submission_id, sc.criterion_id, sc.display_name, sc.comment
		 FROM submission_comments sc
		 JOIN submissions sb ON sb.id = sc.submission_id
		 JOIN patrols p ON p.id = sb.patrol_id
		 WHERE `+scope+` AND sc.comment != ''
		 ORDER BY sc.created_at ASC`,
		args...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("querying patrol history comments: %w", err)
	}
	defer commentRows.Close()

	var comments []PatrolHistoryCommentRow
	for commentRows.Next() {
		var comment PatrolHistoryCommentRow
		if err := commentRows.Scan(&comment.ID, &comment.SubmissionID, &comment.CriterionID, &comment.DisplayName, &comment.Comment); err != nil {
			return nil, nil, fmt.Errorf("scanning patrol history comment: %w", err)
		}
		comments = append(comments, comment)
	}
	if err := commentRows.Err(); err != nil {
		return nil, nil, err
	}

	return history, comments, nil
}

// CreateSubmission creates a new submission from scores and deletes the shared draft.
func (d *DB) CreateSubmission(ctx context.Context, submittedBy, sessionID, patrolID string, scores map[string]int) (*SubmissionRow, error) {
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
			scoredBy := scoredByMap[criterionID]
			if scoredBy == nil {
				scoredBy = &submittedBy
			}
			return SubmissionScoreRow{
				ID:           uuid.New().String(),
				SubmissionID: submissionID,
				CriterionID:  criterionID,
				Value:        value,
				ScoredBy:     scoredBy,
			}
		})

		for _, s := range scoreRows {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO submission_scores (id, submission_id, criterion_id, value, scored_by) VALUES ($1, $2, $3, $4, $5)",
				s.ID, s.SubmissionID, s.CriterionID, s.Value, s.ScoredBy,
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
		"SELECT id, submission_id, criterion_id, value, scored_by FROM submission_scores WHERE submission_id = $1",
		submissionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying submission scores: %w", err)
	}
	defer rows.Close()

	var scores []SubmissionScoreRow
	for rows.Next() {
		var s SubmissionScoreRow
		if err := rows.Scan(&s.ID, &s.SubmissionID, &s.CriterionID, &s.Value, &s.ScoredBy); err != nil {
			return nil, fmt.Errorf("scanning submission score: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// UpdateSubmissionScores replaces saved scores for an existing patrol submission.
func (d *DB) UpdateSubmissionScores(ctx context.Context, sessionID, patrolID, editedBy string, scores map[string]int) error {
	return d.InTx(ctx, func(tx *sql.Tx) error {
		var submissionID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM submissions WHERE session_id = $1 AND patrol_id = $2`, sessionID, patrolID).Scan(&submissionID); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("no submitted scores for this patrol")
			}
			return fmt.Errorf("finding submission: %w", err)
		}
		for criterionID, value := range scores {
			result, err := tx.ExecContext(ctx, `UPDATE submission_scores SET value = $1, scored_by = $2 WHERE submission_id = $3 AND criterion_id = $4`, value, editedBy, submissionID, criterionID)
			if err != nil {
				return fmt.Errorf("updating submission score: %w", err)
			}
			affected, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("checking submission score update: %w", err)
			}
			if affected == 0 {
				return fmt.Errorf("criterion is not scored for this patrol")
			}
		}
		return nil
	})
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

// FinaliseSession converts all shared drafts for a session+subcamp into submissions.
// Missing criteria are filled with zeros.
func (d *DB) FinaliseSession(ctx context.Context, userID, sessionID, subcampID string) ([]SubmissionRow, error) {
	var submissions []SubmissionRow
	ctx, span := tracing.Tracer().Start(ctx, "db.finalise_session_subcamp")
	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("session.id", sessionID),
		attribute.String("subcamp.requested_id", subcampID),
	)

	err := d.InTx(ctx, func(tx *sql.Tx) error {
		resolvedSubcampID := subcampID
		if resolvedSubcampID == "" {
			if err := tx.QueryRowContext(ctx,
				"SELECT subcamp_id FROM users WHERE id = $1", userID,
			).Scan(&resolvedSubcampID); err != nil {
				return fmt.Errorf("looking up user subcamp: %w", err)
			}
		}
		span.SetAttributes(attribute.String("subcamp.resolved_id", resolvedSubcampID))

		// Ensure the target subcamp participates in this session.
		var inSessionCount int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM session_subcamps WHERE session_id = $1 AND subcamp_id = $2",
			sessionID, resolvedSubcampID,
		).Scan(&inSessionCount); err != nil {
			return fmt.Errorf("checking session subcamp: %w", err)
		}
		if inSessionCount == 0 {
			return fmt.Errorf("subcamp is not part of this session")
		}

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
		if err := critRows.Err(); err != nil {
			return fmt.Errorf("iterating criteria: %w", err)
		}
		span.SetAttributes(attribute.Int("finalise.criteria_count", len(criterionIDs)))

		// Get all patrols for this subcamp that are in-session
		patrolRows, err := tx.QueryContext(ctx,
			`SELECT p.id, p.name
			 FROM patrols p
			 JOIN session_subcamps ss ON ss.session_id = $1 AND ss.subcamp_id = p.subcamp_id
			 WHERE p.subcamp_id = $2
			   AND (
			     NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = $1)
			     OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = $1 AND sp.patrol_id = p.id)
			   )
			 ORDER BY p.sort_order ASC, p.name ASC`,
			sessionID, resolvedSubcampID,
		)
		if err != nil {
			return fmt.Errorf("querying subcamp patrols: %w", err)
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
		if err := patrolRows.Err(); err != nil {
			return fmt.Errorf("iterating patrols: %w", err)
		}
		span.SetAttributes(attribute.Int("finalise.target_patrols_count", len(userPatrols)))

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
		span.SetAttributes(attribute.Int("finalise.available_drafts_count", len(draftsByPatrol)))

		var createdSubmissions int
		var skippedExisting int
		var patrolsWithDraft int
		var patrolsWithoutDraft int
		var totalDraftScoresLoaded int
		var totalSubmissionScoresWritten int

		// Process every patrol in the target subcamp
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
				skippedExisting++
				span.AddEvent("finalise.patrol_skipped_existing_submission", trace.WithAttributes(
					attribute.String("patrol.id", patrol.ID),
				))
				continue
			}

			// Start with zeros for all criteria
			scores := make(map[string]int, len(criterionIDs))
			for _, cid := range criterionIDs {
				scores[cid] = 0
			}

			// Overlay draft scores if a shared draft exists
			draftScoreCount := 0
			hadDraft := false
			if draft, ok := draftsByPatrol[patrol.ID]; ok {
				hadDraft = true
				patrolsWithDraft++
				scoreRows, err := tx.QueryContext(ctx,
					"SELECT criterion_id, value FROM draft_scores WHERE draft_id = $1",
					draft.ID,
				)
				if err != nil {
					return fmt.Errorf("querying draft scores: %w", err)
				}
				for scoreRows.Next() {
					var criterionID string
					var value int
					if err := scoreRows.Scan(&criterionID, &value); err != nil {
						scoreRows.Close()
						return fmt.Errorf("scanning draft score: %w", err)
					}
					scores[criterionID] = value
					draftScoreCount++
				}
				scoreRows.Close()
				if err := scoreRows.Err(); err != nil {
					return fmt.Errorf("iterating draft scores: %w", err)
				}
			} else {
				patrolsWithoutDraft++
			}
			totalDraftScoresLoaded += draftScoreCount

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
					"INSERT INTO submission_scores (id, submission_id, criterion_id, value, scored_by) VALUES ($1, $2, $3, $4, $5)",
					uuid.New().String(), submissionID, criterionID, value, userID,
				)
				if err != nil {
					return fmt.Errorf("inserting submission score: %w", err)
				}
			}
			totalSubmissionScoresWritten += len(scores)

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
			createdSubmissions++
			defaultedScoresCount := len(criterionIDs) - draftScoreCount
			if defaultedScoresCount < 0 {
				span.AddEvent("finalise.draft_scores_exceed_criteria", trace.WithAttributes(
					attribute.String("patrol.id", patrol.ID),
					attribute.Int("finalise.criteria_count", len(criterionIDs)),
					attribute.Int("finalise.draft_scores_loaded_count", draftScoreCount),
				))
				return fmt.Errorf("draft scores exceed criteria count for patrol %s", patrol.ID)
			}
			span.AddEvent("finalise.patrol_submission_created", trace.WithAttributes(
				attribute.String("patrol.id", patrol.ID),
				attribute.Bool("patrol.had_draft", hadDraft),
				attribute.Int("patrol.draft_scores_loaded_count", draftScoreCount),
				attribute.Int("patrol.defaulted_scores_count", defaultedScoresCount),
			))
		}

		span.SetAttributes(
			attribute.Int("finalise.submissions_created_count", createdSubmissions),
			attribute.Int("finalise.patrols_skipped_existing_count", skippedExisting),
			attribute.Int("finalise.patrols_with_draft_count", patrolsWithDraft),
			attribute.Int("finalise.patrols_without_draft_count", patrolsWithoutDraft),
			attribute.Int("finalise.total_draft_scores_loaded_count", totalDraftScoresLoaded),
			attribute.Int("finalise.total_submission_scores_written_count", totalSubmissionScoresWritten),
		)
		return nil
	})

	return submissions, err
}

// ReviseSession converts all submissions for a user's subcamp patrols back into shared drafts.
func (d *DB) ReviseSession(ctx context.Context, userID, sessionID string) error {
	var subcampID string
	if err := d.QueryRowContext(ctx, "SELECT subcamp_id FROM users WHERE id = $1", userID).Scan(&subcampID); err != nil {
		return fmt.Errorf("querying user subcamp: %w", err)
	}
	return d.ReviseSessionSubcamp(ctx, userID, sessionID, subcampID)
}

// ReviseSessionSubcamp converts all submissions for a subcamp's patrols back into shared drafts.
func (d *DB) ReviseSessionSubcamp(ctx context.Context, userID, sessionID, subcampID string) error {
	return d.InTx(ctx, func(tx *sql.Tx) error {
		// Get patrols in the subcamp that are included in this session.
		patrolRows, err := tx.QueryContext(ctx,
			`SELECT p.id
			 FROM patrols p
			 JOIN session_subcamps ss ON ss.session_id = $1 AND ss.subcamp_id = p.subcamp_id
			 WHERE p.subcamp_id = $2
			   AND (
			     NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = $1)
			     OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = $1 AND sp.patrol_id = p.id)
			   )`,
			sessionID, subcampID,
		)
		if err != nil {
			return fmt.Errorf("querying subcamp patrols: %w", err)
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
		if len(patrolIDs) == 0 {
			return nil
		}

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
				"SELECT criterion_id, value, scored_by FROM submission_scores WHERE submission_id = $1",
				sub.ID,
			)
			if err != nil {
				return fmt.Errorf("querying submission scores: %w", err)
			}

			type scoreWithAttr struct {
				Value    int
				ScoredBy *string
			}
			scores := make(map[string]scoreWithAttr)
			for scoreRows.Next() {
				var criterionID string
				var s scoreWithAttr
				if err := scoreRows.Scan(&criterionID, &s.Value, &s.ScoredBy); err != nil {
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
					`INSERT INTO draft_scores (id, draft_id, criterion_id, value, last_edited_by, last_edited_at)
					 VALUES ($1, $2, $3, $4, $5, NOW())
					 ON CONFLICT (draft_id, criterion_id) DO UPDATE
					   SET value = EXCLUDED.value,
					       last_edited_by = EXCLUDED.last_edited_by, last_edited_at = NOW()`,
					scoreID, draftID, criterionID, sc.Value, sc.ScoredBy,
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
		`SELECT ss.id, ss.submission_id, ss.criterion_id, ss.value, ss.scored_by
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
		if err := rows.Scan(&s.ID, &s.SubmissionID, &s.CriterionID, &s.Value, &s.ScoredBy); err != nil {
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
		        ss.id, ss.submission_id, ss.criterion_id, ss.value
		 FROM submissions s
		 JOIN patrols p ON p.id = s.patrol_id
		 JOIN submission_scores ss ON ss.submission_id = s.id
		 JOIN criteria c ON c.id = ss.criterion_id
		 JOIN users u ON u.id = $1 AND u.subcamp_id = p.subcamp_id
		 WHERE s.session_id = $2
		 ORDER BY p.sort_order, p.name, c.sort_order`,
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
		if err := rows.Scan(&patrolID, &patrolName, &sc.ID, &sc.SubmissionID, &sc.CriterionID, &sc.Value); err != nil {
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
		`SELECT p.id, p.name, p.sort_order, ss.criterion_id, ss.value
		 FROM users u
		 JOIN patrols p ON (
		   (u.is_admin = TRUE AND EXISTS (
		     SELECT 1 FROM session_subcamps ssc
		     WHERE ssc.session_id = $2 AND ssc.subcamp_id = p.subcamp_id
		   ))
		   OR (u.is_admin = FALSE AND u.subcamp_id = p.subcamp_id AND EXISTS (
		     SELECT 1 FROM session_subcamps ssc
		     WHERE ssc.session_id = $2 AND ssc.subcamp_id = u.subcamp_id
		   ))
		 )
		 JOIN submissions s ON s.session_id = $2 AND s.patrol_id = p.id
		 JOIN submission_scores ss ON ss.submission_id = s.id
		 WHERE u.id = $1
		 ORDER BY p.sort_order ASC, ss.criterion_id ASC`,
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
