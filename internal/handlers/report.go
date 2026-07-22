package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/tracing"
)

// ReportHandler handles report generation endpoints.
type ReportHandler struct {
	db      *database.DB
	logoPNG []byte // optional embedded logo; nil means no logo
}

type reportPatrolEntry struct {
	name   string
	scores map[string]int // criterion_id -> value
	total  int
}

type reportCriterionComment struct {
	displayName string
	comment     string
}

// NewReportHandler creates a new ReportHandler.
// logoPNG may be nil if no logo is available.
func NewReportHandler(db *database.DB, logoPNG []byte) *ReportHandler {
	return &ReportHandler{db: db, logoPNG: logoPNG}
}

// GetReportCard generates a PDF score summary for a closed session.
// GET /api/sessions/{session_id}/report-card
func (h *ReportHandler) GetReportCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracing.Tracer().Start(ctx, "handler.get_report_card")
	defer span.End()

	user := auth.UserFromContext(ctx)
	sessionID := r.PathValue("session_id")

	// Fetch session details
	session, err := h.db.GetSession(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "session not found")
		return
	}

	// Only allow report generation for closed sessions
	if session.ComputeStatus() != "CLOSED" {
		writeError(w, r, http.StatusBadRequest, "report card is only available after the session has ended")
		return
	}

	// Fetch criteria for column headers
	criteria, err := h.db.GetTemplateCriteria(ctx, session.TemplateID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load criteria")
		return
	}
	if len(criteria) == 0 {
		writeError(w, r, http.StatusInternalServerError, "no criteria found for session template")
		return
	}

	// Fetch score data
	rows, err := h.db.GetReportCardData(ctx, user.ID, sessionID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load scores")
		return
	}

	// Build patrol→criterion→value map and ordered patrol list
	patrolOrder := []string{}
	patrolMap := map[string]*reportPatrolEntry{}

	for _, row := range rows {
		pe, exists := patrolMap[row.PatrolID]
		if !exists {
			pe = &reportPatrolEntry{name: row.PatrolName, scores: map[string]int{}}
			patrolMap[row.PatrolID] = pe
			patrolOrder = append(patrolOrder, row.PatrolID)
		}
		pe.scores[row.CriterionID] = row.Value
		pe.total += row.Value
	}

	// Load per-user comments for each visible patrol/criterion.
	allComments, err := h.db.GetAllSessionComments(ctx, sessionID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load comments")
		return
	}
	visiblePatrols := make(map[string]bool, len(patrolOrder))
	for _, pid := range patrolOrder {
		visiblePatrols[pid] = true
	}
	commentsByPatrolCriterion := map[string]map[string][]reportCriterionComment{}
	for _, c := range allComments {
		if !visiblePatrols[c.PatrolID] {
			continue
		}
		commentText := strings.TrimSpace(c.Comment)
		if commentText == "" {
			continue
		}
		if commentsByPatrolCriterion[c.PatrolID] == nil {
			commentsByPatrolCriterion[c.PatrolID] = map[string][]reportCriterionComment{}
		}
		commentsByPatrolCriterion[c.PatrolID][c.CriterionID] = append(
			commentsByPatrolCriterion[c.PatrolID][c.CriterionID],
			reportCriterionComment{displayName: c.DisplayName, comment: commentText},
		)
	}

	// Generate PDF
	pdf := fpdf.New("L", "mm", "A4", "") // landscape for wide tables
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	// ─── Header ─────────────────────────────────────────────────
	headerY := 10.0

	// Optional logo in top-left
	if h.logoPNG != nil {
		logoOpt := fpdf.ImageOptions{ImageType: "PNG"}
		pdf.RegisterImageOptionsReader("logo", logoOpt, bytes.NewReader(h.logoPNG))
		pdf.ImageOptions("logo", 10, headerY, 20, 0, false, logoOpt, 0, "")
	}

	// Title block (offset if logo present)
	titleX := 10.0
	if h.logoPNG != nil {
		titleX = 35.0
	}

	pdf.SetXY(titleX, headerY)
	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 8, session.Name+" - Score Report", "", 1, "L", false, 0, "")

	pdf.SetX(titleX)
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 5, fmt.Sprintf("Date: %s", session.StartsAt.Format("Monday 2 January 2006")), "", 1, "L", false, 0, "")

	pdf.SetX(titleX)
	pdf.CellFormat(0, 5, fmt.Sprintf("Event: %s", session.EventName), "", 1, "L", false, 0, "")

	pdf.Ln(6)

	// ─── Table ──────────────────────────────────────────────────
	// Calculate column widths
	pageWidth, _ := pdf.GetPageSize()
	marginL, _, marginR, _ := pdf.GetMargins()
	usableWidth := pageWidth - marginL - marginR

	// Columns: Patrol name | criteria... | Total
	numCols := len(criteria) + 3 // rank + patrol + criteria + total
	rankColW := 10.0
	patrolColW := 40.0
	totalColW := 18.0
	remainingW := usableWidth - rankColW - patrolColW - totalColW
	criteriaColW := remainingW / float64(len(criteria))
	if criteriaColW < 12 {
		criteriaColW = 12
	}

	// Header row
	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(230, 230, 230)

	pdf.CellFormat(rankColW, 7, "#", "1", 0, "C", true, 0, "")
	pdf.CellFormat(patrolColW, 7, "Patrol", "1", 0, "C", true, 0, "")
	for _, c := range criteria {
		title := truncate(c.Title, int(criteriaColW/2))
		pdf.CellFormat(criteriaColW, 7, title, "1", 0, "C", true, 0, "")
	}
	pdf.CellFormat(totalColW, 7, "Total", "1", 1, "C", true, 0, "")
	_ = numCols

	// Determine ranking by total score
	type rankedPatrol struct {
		id    string
		total int
	}
	ranked := make([]rankedPatrol, 0, len(patrolOrder))
	for _, pid := range patrolOrder {
		ranked = append(ranked, rankedPatrol{id: pid, total: patrolMap[pid].total})
	}
	// Sort descending by total
	for i := range ranked {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].total > ranked[i].total {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	// Map patrol ID → position (top 5 only)
	rankPosition := map[string]int{}
	for i, rp := range ranked {
		if i >= 5 {
			break
		}
		rankPosition[rp.id] = i + 1
	}

	// Data rows
	pdf.SetFont("Arial", "", 9)
	for _, pid := range patrolOrder {
		pe := patrolMap[pid]
		posStr := ""
		if pos, ok := rankPosition[pid]; ok {
			posStr = fmt.Sprintf("%d", pos)
		}
		pdf.CellFormat(rankColW, 6, posStr, "1", 0, "C", false, 0, "")
		pdf.CellFormat(patrolColW, 6, pe.name, "1", 0, "L", false, 0, "")
		for _, c := range criteria {
			val := pe.scores[c.ID]
			pdf.CellFormat(criteriaColW, 6, fmt.Sprintf("%d", val), "1", 0, "C", false, 0, "")
		}
		pdf.CellFormat(totalColW, 6, fmt.Sprintf("%d", pe.total), "1", 1, "C", false, 0, "")
	}

	// ─── Footer with generation timestamp ───────────────────────
	pdf.SetFont("Arial", "I", 7)
	pdf.CellFormat(0, 4, fmt.Sprintf("Generated %s", time.Now().Format("2 Jan 2006 15:04")), "", 0, "R", false, 0, "")

	// ─── Per-patrol scorecards (2 per page) ─────────────────────
	for i, pid := range patrolOrder {
		if i%2 == 0 {
			pdf.AddPageFormat("P", fpdf.SizeType{Wd: 210, Ht: 297})
		}

		cardMargin := 10.0
		cardGap := 6.0
		pageW, pageH := pdf.GetPageSize()
		cardW := pageW - (2 * cardMargin)
		cardH := (pageH - (2 * cardMargin) - cardGap) / 2
		cardY := cardMargin
		if i%2 == 1 {
			cardY += cardH + cardGap
		}

		renderPatrolScorecard(
			pdf,
			session.Name,
			patrolMap[pid],
			criteria,
			commentsByPatrolCriterion[pid],
			cardMargin,
			cardY,
			cardW,
			cardH,
		)
	}

	// Write PDF to response
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to generate PDF")
		return
	}

	filename := fmt.Sprintf("%s-scores-%s.pdf", session.Name, session.StartsAt.Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.Write(buf.Bytes())
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func renderPatrolScorecard(
	pdf *fpdf.Fpdf,
	sessionName string,
	patrol *reportPatrolEntry,
	criteria []database.CriterionRow,
	commentsByCriterion map[string][]reportCriterionComment,
	x, y, w, h float64,
) {
	if patrol == nil {
		return
	}

	// Card frame
	pdf.SetDrawColor(170, 170, 170)
	pdf.SetFillColor(255, 255, 255)
	pdf.Rect(x, y, w, h, "D")

	pad := 4.0
	innerX := x + pad
	innerY := y + pad
	innerW := w - (2 * pad)
	innerBottom := y + h - pad

	// Title block
	pdf.SetXY(innerX, innerY)
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(innerW, 6, truncate(sessionName+" - "+patrol.name, 64), "", 1, "L", false, 0, "")
	pdf.SetX(innerX)
	pdf.SetFont("Arial", "", 9)
	pdf.CellFormat(innerW, 5, fmt.Sprintf("Total Score: %d", patrol.total), "", 1, "L", false, 0, "")

	pdf.Ln(1)

	// Criterion table headers
	titleW := 64.0
	scoreW := 18.0
	commentW := innerW - titleW - scoreW
	headerH := 6.0
	rowH := 6.0

	pdf.SetX(innerX)
	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(235, 235, 235)
	pdf.CellFormat(titleW, headerH, "Category", "1", 0, "L", true, 0, "")
	pdf.CellFormat(scoreW, headerH, "Score", "1", 0, "C", true, 0, "")
	pdf.CellFormat(commentW, headerH, "Comments", "1", 1, "L", true, 0, "")

	pdf.SetFont("Arial", "", 8)
	for _, c := range criteria {
		if pdf.GetY()+rowH > innerBottom {
			break
		}

		score := patrol.scores[c.ID]
		commentSummary := summarizeComments(commentsByCriterion[c.ID], 100)

		pdf.SetX(innerX)
		pdf.CellFormat(titleW, rowH, truncate(c.Title, 42), "1", 0, "L", false, 0, "")
		pdf.CellFormat(scoreW, rowH, fmt.Sprintf("%d/%d", score, c.MaxValue), "1", 0, "C", false, 0, "")
		pdf.CellFormat(commentW, rowH, commentSummary, "1", 1, "L", false, 0, "")
	}

}

func summarizeComments(comments []reportCriterionComment, maxLen int) string {
	if len(comments) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(comments))
	for _, c := range comments {
		name := strings.TrimSpace(c.displayName)
		text := strings.Join(strings.Fields(c.comment), " ")
		if name != "" {
			parts = append(parts, name+": "+text)
		} else {
			parts = append(parts, text)
		}
	}
	return truncate(strings.Join(parts, " | "), maxLen)
}
