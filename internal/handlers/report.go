package handlers

import (
	"bytes"
	"fmt"
	"net/http"
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

	// Only allow report generation once the session is no longer accepting scores
	status := session.ComputeStatus()
	if status != "ENDED" && status != "CLOSED" {
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
	type patrolEntry struct {
		name   string
		scores map[string]int // criterion_id → value
		total  int
	}
	patrolOrder := []string{}
	patrolMap := map[string]*patrolEntry{}

	for _, row := range rows {
		pe, exists := patrolMap[row.PatrolID]
		if !exists {
			pe = &patrolEntry{name: row.PatrolName, scores: map[string]int{}}
			patrolMap[row.PatrolID] = pe
			patrolOrder = append(patrolOrder, row.PatrolID)
		}
		pe.scores[row.CriterionID] = row.Value
		pe.total += row.Value
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

	// Columns: rank | patrol | criteria... | total
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
	pdf.Ln(4)
	pdf.SetFont("Arial", "I", 7)
	pdf.CellFormat(0, 4, fmt.Sprintf("Generated %s", time.Now().Format("2 Jan 2006 15:04")), "", 0, "R", false, 0, "")

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

// truncate shortens a string to maxLen characters, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}
