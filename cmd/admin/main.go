// Scoutmark admin CLI — user management tools.
//
// Usage:
//
//	go run ./cmd/admin create-user
//	go run ./cmd/admin change-password
//	go run ./cmd/admin list-users
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/allyjweir/scoutmark/internal/database"
)

const usage = `Scoutmark admin CLI

Usage:
  admin <command>

Commands:
  create-user       Create a new user (interactive or with flags)
	change-password   Change a user's password
	rename-user       Rename a user or update their display name
	list-users        List all users
  query             Execute an arbitrary PostgreSQL query
	create-subcamp    Create a subcamp
	update-subcamp    Update a subcamp name
  create-event      Create an event
  create-template   Create a criteria template
  add-criterion     Add a criterion to a template
  create-patrol     Create a patrol
	assign-user-subcamp Assign a user to a subcamp
	assign-patrol-subcamp Assign a patrol to a subcamp
	assign-session-subcamp Assign a session to a subcamp
  create-session    Create a scoring session
	upsert-ba-structure Replace BA subcamps and patrols from scripts/subcamps-patrols.yaml
	upsert-ba-criteria Upsert Blair Atholl criteria from scripts/scoring-categories.yaml
  update-session    Update session settings (awards, previous session)
  list-sessions     List all sessions with status
	apply-ba-rubric   Apply the Blair Atholl rubric to known criterion IDs
  seed-scores       Seed random submission scores for all patrols in a session
	seed-ba-demo      Seed the Blair Atholl demo data set

Environment:
  DATABASE_URL      PostgreSQL connection string (default: postgres://scoutmark:scoutmark@localhost:5432/scoutmark?sslmode=disable)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "create-user":
		err = createUser()
	case "change-password":
		err = changePassword()
	case "rename-user":
		err = renameUser()
	case "list-users":
		err = listUsers()
	case "query":
		err = query()
	case "create-subcamp":
		err = createSubcamp()
	case "update-subcamp":
		err = updateSubcamp()
	case "create-event":
		err = createEvent()
	case "create-template":
		err = createTemplate()
	case "add-criterion":
		err = addCriterion()
	case "create-patrol":
		err = createPatrol()
	case "assign-user-subcamp":
		err = assignUserSubcamp()
	case "assign-patrol-subcamp":
		err = assignPatrolSubcamp()
	case "assign-session-subcamp":
		err = assignSessionSubcamp()
	case "create-session":
		err = createSession()
	case "upsert-ba-structure":
		err = upsertBAStructure()
	case "upsert-ba-criteria":
		err = upsertBACriteria()
	case "update-session":
		err = updateSession()
	case "list-sessions":
		err = listSessions()
	case "apply-ba-rubric":
		err = applyBARubric()
	case "seed-scores":
		err = seedScores()
	case "seed-ba-demo":
		err = seedBADemo()
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		fmt.Print(usage)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// ─── Database ───────────────────────────────────────────────────────

func connectDB() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://scoutmark:scoutmark@localhost:5432/scoutmark?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return db, nil
}

// ─── Commands ───────────────────────────────────────────────────────

func createUser() error {
	fs := flag.NewFlagSet("create-user", flag.ExitOnError)

	flagUsername := fs.String("username", "", "Username (non-interactive mode)")
	flagPassword := fs.String("password", "", "Password (non-interactive mode)")
	flagDisplay := fs.String("display-name", "", "Display name (non-interactive mode)")
	flagAdmin := fs.Bool("admin", false, "Make admin user (non-interactive mode)")
	flagSubcamp := fs.String("subcamp", "", "Subcamp ID (optional; ignored for admin users)")
	flagID := fs.String("id", "", "User ID (default: auto-generated UUID)")
	flagForcePasswordChange := fs.Bool("force-password-change", false, "Require password change on first login")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	var username, password, displayName, subcampID string
	var isAdmin, forcePasswordChange bool

	if *flagUsername != "" {
		// Non-interactive (flag-driven) mode for scripting
		username = *flagUsername
		password = *flagPassword
		displayName = *flagDisplay
		isAdmin = *flagAdmin
		subcampID = *flagSubcamp
		forcePasswordChange = *flagForcePasswordChange
		if password == "" {
			return fmt.Errorf("-password is required in non-interactive mode")
		}
		if displayName == "" {
			displayName = username
		}
	} else {
		// Interactive mode
		reader := bufio.NewReader(os.Stdin)

		var err error
		username, err = prompt(reader, "Username")
		if err != nil {
			return err
		}

		password, err = promptPassword("Password")
		if err != nil {
			return err
		}

		confirm, err := promptPassword("Confirm password")
		if err != nil {
			return err
		}
		if password != confirm {
			return fmt.Errorf("passwords do not match")
		}

		displayName, err = prompt(reader, "Display name")
		if err != nil {
			return err
		}

		adminInput, err := promptDefault(reader, "Admin user?", "N")
		if err != nil {
			return err
		}
		isAdmin = strings.HasPrefix(strings.ToLower(adminInput), "y")

		forceChangeInput, err := promptDefault(reader, "Force password change on first login?", "N")
		if err != nil {
			return err
		}
		forcePasswordChange = strings.HasPrefix(strings.ToLower(forceChangeInput), "y")

		if !isAdmin {
			subcampInput, err := promptDefault(reader, "Subcamp ID (optional)", "")
			if err != nil {
				return err
			}
			subcampID = strings.TrimSpace(subcampInput)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	var exists bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists); err != nil {
		return fmt.Errorf("checking existing user: %w", err)
	}
	if exists {
		return fmt.Errorf("user %q already exists", username)
	}

	if isAdmin {
		subcampID = ""
	}
	if subcampID != "" {
		var subcampExists bool
		if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM subcamps WHERE id = $1)", subcampID).Scan(&subcampExists); err != nil {
			return fmt.Errorf("checking subcamp: %w", err)
		}
		if !subcampExists {
			return fmt.Errorf("subcamp %q not found", subcampID)
		}
	}

	userID := *flagID
	if userID == "" {
		userID = uuid.New().String()
	}
	_, err = db.Exec(
		"INSERT INTO users (id, username, password_hash, display_name, is_admin, subcamp_id, password_change_required) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		userID, username, string(hash), displayName, isAdmin, nullableString(subcampID), forcePasswordChange,
	)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ User created")
	fmt.Printf("  ID:                        %s\n", userID)
	fmt.Printf("  Username:                  %s\n", username)
	fmt.Printf("  Display name:              %s\n", displayName)
	fmt.Printf("  Admin:                     %v\n", isAdmin)
	fmt.Printf("  Subcamp:                   %s\n", emptyAsDash(subcampID))
	fmt.Printf("  Force password change:     %v\n", forcePasswordChange)
	return nil
}

func changePassword() error {
	reader := bufio.NewReader(os.Stdin)

	username, err := prompt(reader, "Username")
	if err != nil {
		return err
	}

	password, err := promptPassword("New password")
	if err != nil {
		return err
	}

	confirm, err := promptPassword("Confirm new password")
	if err != nil {
		return err
	}
	if password != confirm {
		return fmt.Errorf("passwords do not match")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	result, err := db.Exec("UPDATE users SET password_hash = $1 WHERE username = $2", string(hash), username)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user %q not found", username)
	}

	fmt.Printf("\n✓ Password updated for user %q\n", username)
	return nil
}

func renameUser() error {
	fs := flag.NewFlagSet("rename-user", flag.ExitOnError)
	username := fs.String("username", "", "Current username (required)")
	newUsername := fs.String("new-username", "", "New username")
	displayName := fs.String("display-name", "", "New display name")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *username == "" {
		return fmt.Errorf("required flag: -username")
	}
	if *newUsername == "" && *displayName == "" {
		return fmt.Errorf("provide at least one of: -new-username, -display-name")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	var result sql.Result
	if *newUsername != "" && *displayName != "" {
		result, err = db.Exec(
			"UPDATE users SET username = $1, display_name = $2 WHERE username = $3",
			*newUsername, *displayName, *username,
		)
	} else if *newUsername != "" {
		result, err = db.Exec("UPDATE users SET username = $1 WHERE username = $2", *newUsername, *username)
	} else {
		result, err = db.Exec("UPDATE users SET display_name = $1 WHERE username = $2", *displayName, *username)
	}
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("user %q already exists", *newUsername)
		}
		return fmt.Errorf("updating user: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user %q not found", *username)
	}

	if *newUsername != "" && *displayName != "" {
		fmt.Printf("\n✓ User %q renamed to %q and display name updated\n", *username, *newUsername)
	} else if *newUsername != "" {
		fmt.Printf("\n✓ User %q renamed to %q\n", *username, *newUsername)
	} else {
		fmt.Printf("\n✓ Display name updated for user %q\n", *username)
	}
	return nil
}

func listUsers() error {
	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT u.id, u.username, u.display_name, u.is_admin, COALESCE(sc.name, ''), u.created_at
		 FROM users u
		 LEFT JOIN subcamps sc ON sc.id = u.subcamp_id
		 ORDER BY u.created_at ASC`,
	)
	if err != nil {
		return fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tDISPLAY NAME\tADMIN\tSUBCAMP\tCREATED")
	fmt.Fprintln(w, "──\t────────\t────────────\t─────\t───────\t───────")

	count := 0
	for rows.Next() {
		var id, username, displayName, subcampName, createdAt string
		var isAdmin bool
		if err := rows.Scan(&id, &username, &displayName, &isAdmin, &subcampName, &createdAt); err != nil {
			return fmt.Errorf("scanning user: %w", err)
		}

		admin := ""
		if isAdmin {
			admin = "✓"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", id, username, displayName, admin, emptyAsDash(subcampName), createdAt)
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}

	w.Flush()
	fmt.Printf("\n%d user(s)\n", count)
	return nil
}

func query() error {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	sqlQuery := fs.String("sql", "", "PostgreSQL query to execute (required)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *sqlQuery == "" {
		return fmt.Errorf("required flag: -sql")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query(*sqlQuery)
	if err != nil {
		return fmt.Errorf("executing query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("getting query columns: %w", err)
	}
	if len(columns) == 0 {
		fmt.Println("✓ Query executed")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(columns, "\t"))
	fmt.Fprintln(w, strings.Repeat("──\t", len(columns)-1)+"──")

	values := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for i := range values {
		destinations[i] = &values[i]
	}

	count := 0
	for rows.Next() {
		if err := rows.Scan(destinations...); err != nil {
			return fmt.Errorf("scanning query result: %w", err)
		}
		formatted := make([]string, len(values))
		for i, value := range values {
			formatted[i] = formatQueryValue(value)
		}
		fmt.Fprintln(w, strings.Join(formatted, "\t"))
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading query result: %w", err)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing query result: %w", err)
	}
	fmt.Printf("\n%d row(s)\n", count)
	return nil
}

func formatQueryValue(value any) string {
	if value == nil {
		return "NULL"
	}
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	return fmt.Sprint(value)
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func createSubcamp() error {
	fs := flag.NewFlagSet("create-subcamp", flag.ExitOnError)
	id := fs.String("id", "", "Subcamp ID (default: auto-generated UUID)")
	name := fs.String("name", "", "Subcamp name (required)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("required flag: -name")
	}

	subcampID := *id
	if subcampID == "" {
		subcampID = uuid.New().String()
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec("INSERT INTO subcamps (id, name) VALUES ($1, $2)", subcampID, *name); err != nil {
		return fmt.Errorf("creating subcamp: %w", err)
	}

	fmt.Println("✓ Subcamp created")
	fmt.Printf("  ID:   %s\n", subcampID)
	fmt.Printf("  Name: %s\n", *name)
	return nil
}

func updateSubcamp() error {
	fs := flag.NewFlagSet("update-subcamp", flag.ExitOnError)
	id := fs.String("id", "", "Subcamp ID (required)")
	name := fs.String("name", "", "New subcamp name (required)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *id == "" || *name == "" {
		return fmt.Errorf("required flags: -id, -name")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	result, err := db.Exec("UPDATE subcamps SET name = $2 WHERE id = $1", *id, *name)
	if err != nil {
		return fmt.Errorf("updating subcamp: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking update result: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("subcamp %q not found", *id)
	}

	fmt.Println("✓ Subcamp updated")
	fmt.Printf("  ID:   %s\n", *id)
	fmt.Printf("  Name: %s\n", *name)
	return nil
}

func createSession() error {
	fs := flag.NewFlagSet("create-session", flag.ExitOnError)

	eventID := fs.String("event", "", "Event ID (required)")
	templateID := fs.String("template", "", "Criteria template ID (required for regular sessions)")
	name := fs.String("name", "", "Session name (required)")
	startStr := fs.String("start", "", `Start time in RFC3339 or "now" (default: now)`)
	durationStr := fs.String("duration", "3h", "Duration from start (e.g. 2h, 6h, 30m)")
	sessionID := fs.String("id", "", "Session ID (default: auto-generated UUID)")
	awardBestPatrol := fs.Bool("award-best-patrol", false, "Enable Best Patrol award")
	awardMostImproved := fs.Bool("award-most-improved", false, "Enable Most Improved award")
	previousSessionID := fs.String("previous-session", "", "ID of the previous session (for chaining / Most Improved)")
	subcampsCSV := fs.String("subcamps", "", "Comma-separated subcamp IDs to include (default: all subcamps)")
	roundType := fs.String("round-type", "regular", `Session type: "regular" or "round2" (Round 2 uses its dedicated overall-score template)`)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Create a scoring session

Usage:
  admin create-session [flags]

Examples:
  admin create-session -event evt-weekly-2026 -template tpl-weekly -name "Week 8 Meeting"
  admin create-session -event evt-weekly-2026 -template tpl-weekly -name "Week 8" -duration 6h
  admin create-session -event evt-camp-2026 -template tpl-camp -name "Day 4 Inspection" -start now -duration 2h

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	if *eventID == "" || *name == "" || (*roundType == "regular" && *templateID == "") {
		fs.Usage()
		return fmt.Errorf("required flags: -event, -name, and -template for regular sessions")
	}
	if *roundType != "regular" && *roundType != "round2" {
		return fmt.Errorf("invalid round type %q; use \"regular\" or \"round2\"", *roundType)
	}
	if *roundType == "round2" {
		*templateID = database.Round2TemplateID
	}
	bestPatrolEnabled := *awardBestPatrol || *roundType == "round2"

	duration, err := time.ParseDuration(*durationStr)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", *durationStr, err)
	}

	var startsAt time.Time
	switch {
	case *startStr == "" || *startStr == "now":
		startsAt = time.Now()
	default:
		startsAt, err = time.Parse(time.RFC3339, *startStr)
		if err != nil {
			return fmt.Errorf("invalid start time %q (use RFC3339 or \"now\"): %w", *startStr, err)
		}
	}

	endsAt := startsAt.Add(duration)

	id := *sessionID
	if id == "" {
		id = uuid.New().String()
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Verify event exists
	var eventName string
	if err := db.QueryRow("SELECT name FROM events WHERE id = $1", *eventID).Scan(&eventName); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("event %q not found\n\nAvailable events:\n%s", *eventID, listAvailable(db, "SELECT id, name FROM events ORDER BY name"))
		}
		return fmt.Errorf("checking event: %w", err)
	}

	// Verify template exists
	var templateName string
	if err := db.QueryRow("SELECT name FROM criteria_templates WHERE id = $1", *templateID).Scan(&templateName); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("template %q not found\n\nAvailable templates:\n%s", *templateID, listAvailable(db, "SELECT id, name FROM criteria_templates ORDER BY name"))
		}
		return fmt.Errorf("checking template: %w", err)
	}

	var prevID *string
	if *previousSessionID != "" {
		// Verify previous session exists
		var prevName string
		if err := db.QueryRow("SELECT name FROM sessions WHERE id = $1", *previousSessionID).Scan(&prevName); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("previous session %q not found", *previousSessionID)
			}
			return fmt.Errorf("checking previous session: %w", err)
		}
		prevID = previousSessionID
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at, award_best_patrol, award_most_improved, previous_session_id, round_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, *eventID, *templateID, *name, startsAt, endsAt, bestPatrolEnabled, *awardMostImproved, prevID, *roundType,
	)
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}

	var subcampIDs []string
	if strings.TrimSpace(*subcampsCSV) == "" {
		rows, err := tx.Query("SELECT id FROM subcamps ORDER BY name")
		if err != nil {
			return fmt.Errorf("querying subcamps: %w", err)
		}
		for rows.Next() {
			var sid string
			if err := rows.Scan(&sid); err != nil {
				rows.Close()
				return fmt.Errorf("scanning subcamp: %w", err)
			}
			subcampIDs = append(subcampIDs, sid)
		}
		rows.Close()
	} else {
		for _, sid := range strings.Split(*subcampsCSV, ",") {
			t := strings.TrimSpace(sid)
			if t != "" {
				subcampIDs = append(subcampIDs, t)
			}
		}
	}

	if len(subcampIDs) == 0 {
		return fmt.Errorf("no subcamps available for session association")
	}

	for _, sid := range subcampIDs {
		var exists bool
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM subcamps WHERE id = $1)", sid).Scan(&exists); err != nil {
			return fmt.Errorf("checking subcamp %q: %w", sid, err)
		}
		if !exists {
			return fmt.Errorf("subcamp %q not found", sid)
		}
		if _, err := tx.Exec(
			"INSERT INTO session_subcamps (session_id, subcamp_id) VALUES ($1, $2)",
			id, sid,
		); err != nil {
			return fmt.Errorf("associating subcamp %q with session: %w", sid, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing session transaction: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Session created")
	fmt.Printf("  ID:       %s\n", id)
	fmt.Printf("  Name:     %s\n", *name)
	fmt.Printf("  Type:     %s\n", *roundType)
	fmt.Printf("  Event:    %s (%s)\n", eventName, *eventID)
	fmt.Printf("  Template: %s (%s)\n", templateName, *templateID)
	fmt.Printf("  Starts:   %s\n", startsAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Ends:     %s\n", endsAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Duration: %s\n", duration)
	fmt.Printf("  Subcamps: %d\n", len(subcampIDs))
	if bestPatrolEnabled || *awardMostImproved {
		fmt.Printf("  Awards:   best_patrol=%v most_improved=%v\n", bestPatrolEnabled, *awardMostImproved)
	}
	if prevID != nil {
		fmt.Printf("  Previous: %s\n", *prevID)
	}
	return nil
}

func updateSession() error {
	fs := flag.NewFlagSet("update-session", flag.ExitOnError)

	sessionID := fs.String("id", "", "Session ID (required)")
	awardBestPatrol := fs.Bool("award-best-patrol", false, "Enable Best Patrol award")
	awardMostImproved := fs.Bool("award-most-improved", false, "Enable Most Improved award")
	previousSessionID := fs.String("previous-session", "", "ID of the previous session (for chaining)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Update a session's settings

Usage:
  admin update-session -id <session-id> [flags]

Examples:
  admin update-session -id ses-mon -award-best-patrol -award-most-improved
  admin update-session -id ses-tue -previous-session ses-mon

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	if *sessionID == "" {
		fs.Usage()
		return fmt.Errorf("required flag: -id")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Verify session exists
	var name string
	if err := db.QueryRow("SELECT name FROM sessions WHERE id = $1", *sessionID).Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("session %q not found", *sessionID)
		}
		return fmt.Errorf("checking session: %w", err)
	}

	var prevID *string
	if *previousSessionID != "" {
		var prevName string
		if err := db.QueryRow("SELECT name FROM sessions WHERE id = $1", *previousSessionID).Scan(&prevName); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("previous session %q not found", *previousSessionID)
			}
			return fmt.Errorf("checking previous session: %w", err)
		}
		prevID = previousSessionID
	}

	_, err = db.Exec(
		`UPDATE sessions
		 SET award_best_patrol = $2, award_most_improved = $3, previous_session_id = $4
		 WHERE id = $1`,
		*sessionID, *awardBestPatrol, *awardMostImproved, prevID,
	)
	if err != nil {
		return fmt.Errorf("updating session: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Session updated")
	fmt.Printf("  ID:     %s\n", *sessionID)
	fmt.Printf("  Name:   %s\n", name)
	fmt.Printf("  Awards: best_patrol=%v most_improved=%v\n", *awardBestPatrol, *awardMostImproved)
	if prevID != nil {
		fmt.Printf("  Previous: %s\n", *prevID)
	}
	return nil
}

func listSessions() error {
	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT s.id, s.name, e.name, ct.name, s.starts_at, s.ends_at
		FROM sessions s
		JOIN events e ON e.id = s.event_id
		JOIN criteria_templates ct ON ct.id = s.template_id
		ORDER BY s.starts_at DESC`)
	if err != nil {
		return fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tEVENT\tTEMPLATE\tSTATUS\tSTART\tEND")
	fmt.Fprintln(w, "──\t────\t─────\t────────\t──────\t─────\t───")

	now := time.Now()
	count := 0
	for rows.Next() {
		var id, name, eventName, templateName string
		var startsAt, endsAt time.Time
		if err := rows.Scan(&id, &name, &eventName, &templateName, &startsAt, &endsAt); err != nil {
			return fmt.Errorf("scanning session: %w", err)
		}

		status := "upcoming"
		if now.After(endsAt) {
			status = "closed"
		} else if now.After(startsAt) {
			status = "● active"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			id, name, eventName, templateName, status,
			startsAt.Format("Jan 02 15:04"),
			endsAt.Format("Jan 02 15:04"))
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}

	w.Flush()
	fmt.Printf("\n%d session(s)\n", count)
	return nil
}

// ─── Event, Template, Criterion, Patrol Commands ────────────────────

func createEvent() error {
	fs := flag.NewFlagSet("create-event", flag.ExitOnError)

	id := fs.String("id", "", "Event ID (default: auto-generated UUID)")
	name := fs.String("name", "", "Event name (required)")
	desc := fs.String("description", "", "Event description")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Create an event

Usage:
  admin create-event -name "Blair Atholl 2026" [-id evt-ba-2026] [-description "..."]

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *name == "" {
		fs.Usage()
		return fmt.Errorf("required flag: -name")
	}

	eventID := *id
	if eventID == "" {
		eventID = uuid.New().String()
	}
	if *desc == "" {
		*desc = *name
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(
		"INSERT INTO events (id, name, description) VALUES ($1, $2, $3)",
		eventID, *name, *desc,
	)
	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}

	fmt.Println("✓ Event created")
	fmt.Printf("  ID:   %s\n", eventID)
	fmt.Printf("  Name: %s\n", *name)
	return nil
}

func createTemplate() error {
	fs := flag.NewFlagSet("create-template", flag.ExitOnError)

	id := fs.String("id", "", "Template ID (default: auto-generated UUID)")
	name := fs.String("name", "", "Template name (required)")
	desc := fs.String("description", "", "Template description")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Create a criteria template

Usage:
  admin create-template -name "Camp Inspection" [-id tpl-camp] [-description "..."]

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *name == "" {
		fs.Usage()
		return fmt.Errorf("required flag: -name")
	}

	templateID := *id
	if templateID == "" {
		templateID = uuid.New().String()
	}
	if *desc == "" {
		*desc = *name
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(
		"INSERT INTO criteria_templates (id, name, description) VALUES ($1, $2, $3)",
		templateID, *name, *desc,
	)
	if err != nil {
		return fmt.Errorf("inserting template: %w", err)
	}

	fmt.Println("✓ Template created")
	fmt.Printf("  ID:   %s\n", templateID)
	fmt.Printf("  Name: %s\n", *name)
	return nil
}

func addCriterion() error {
	fs := flag.NewFlagSet("add-criterion", flag.ExitOnError)

	id := fs.String("id", "", "Criterion ID (default: auto-generated UUID)")
	templateID := fs.String("template", "", "Template ID (required)")
	title := fs.String("title", "", "Criterion title (required)")
	desc := fs.String("description", "", "Criterion description")
	minVal := fs.Int("min", 0, "Minimum score value")
	maxVal := fs.Int("max", 10, "Maximum score value")
	sortOrder := fs.Int("order", 0, "Sort order (default: auto-increment)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Add a criterion to a template

Usage:
  admin add-criterion -template tpl-camp -title "Tent & Bedding" [-min 0] [-max 10] [-order 1]

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *templateID == "" || *title == "" {
		fs.Usage()
		return fmt.Errorf("required flags: -template, -title")
	}

	criterionID := *id
	if criterionID == "" {
		criterionID = uuid.New().String()
	}
	if *desc == "" {
		*desc = *title
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Auto-increment sort order if not specified
	if *sortOrder == 0 {
		var maxOrder int
		err := db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) FROM criteria WHERE template_id = $1", *templateID).Scan(&maxOrder)
		if err != nil {
			return fmt.Errorf("querying max sort order: %w", err)
		}
		*sortOrder = maxOrder + 1
	}

	_, err = db.Exec(
		"INSERT INTO criteria (id, template_id, title, description, min_value, max_value, sort_order) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		criterionID, *templateID, *title, *desc, *minVal, *maxVal, *sortOrder,
	)
	if err != nil {
		return fmt.Errorf("inserting criterion: %w", err)
	}

	fmt.Println("✓ Criterion added")
	fmt.Printf("  ID:       %s\n", criterionID)
	fmt.Printf("  Template: %s\n", *templateID)
	fmt.Printf("  Title:    %s\n", *title)
	fmt.Printf("  Range:    %d–%d\n", *minVal, *maxVal)
	fmt.Printf("  Order:    %d\n", *sortOrder)
	return nil
}

func createPatrol() error {
	fs := flag.NewFlagSet("create-patrol", flag.ExitOnError)

	id := fs.String("id", "", "Patrol ID (default: auto-generated UUID)")
	name := fs.String("name", "", "Patrol name (required)")
	subcampID := fs.String("subcamp", "", "Subcamp ID (required)")
	sortOrder := fs.Int("order", 0, "Sort order within subcamp (default: auto-increment)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Create a patrol

Usage:
  admin create-patrol -name "France" [-id pat-mor-1]

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *name == "" || *subcampID == "" {
		fs.Usage()
		return fmt.Errorf("required flags: -name, -subcamp")
	}

	patrolID := *id
	if patrolID == "" {
		patrolID = uuid.New().String()
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	var subcampExists bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM subcamps WHERE id = $1)", *subcampID).Scan(&subcampExists); err != nil {
		return fmt.Errorf("checking subcamp: %w", err)
	}
	if !subcampExists {
		return fmt.Errorf("subcamp %q not found", *subcampID)
	}

	if *sortOrder == 0 {
		if err := db.QueryRow(
			"SELECT COALESCE(MAX(sort_order), 0) FROM patrols WHERE subcamp_id = $1",
			*subcampID,
		).Scan(sortOrder); err != nil {
			return fmt.Errorf("querying max patrol order: %w", err)
		}
		*sortOrder += 1
	}

	_, err = db.Exec(
		"INSERT INTO patrols (id, name, subcamp_id, sort_order) VALUES ($1, $2, $3, $4)",
		patrolID, *name, *subcampID, *sortOrder,
	)
	if err != nil {
		return fmt.Errorf("inserting patrol: %w", err)
	}

	fmt.Println("✓ Patrol created")
	fmt.Printf("  ID:   %s\n", patrolID)
	fmt.Printf("  Name: %s\n", *name)
	fmt.Printf("  Subcamp: %s\n", *subcampID)
	fmt.Printf("  Order: %d\n", *sortOrder)
	return nil
}

func assignUserSubcamp() error {
	fs := flag.NewFlagSet("assign-user-subcamp", flag.ExitOnError)
	userID := fs.String("user", "", "User ID (required)")
	subcampID := fs.String("subcamp", "", "Subcamp ID (required)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *userID == "" || *subcampID == "" {
		return fmt.Errorf("required flags: -user, -subcamp")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec("UPDATE users SET subcamp_id = $2 WHERE id = $1", *userID, *subcampID); err != nil {
		return fmt.Errorf("assigning user subcamp: %w", err)
	}

	fmt.Println("✓ User assigned to subcamp")
	fmt.Printf("  User:    %s\n", *userID)
	fmt.Printf("  Subcamp: %s\n", *subcampID)
	return nil
}

func assignPatrolSubcamp() error {
	fs := flag.NewFlagSet("assign-patrol-subcamp", flag.ExitOnError)
	patrolID := fs.String("patrol", "", "Patrol ID (required)")
	subcampID := fs.String("subcamp", "", "Subcamp ID (required)")
	sortOrder := fs.Int("order", 0, "Sort order in subcamp (optional)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *patrolID == "" || *subcampID == "" {
		return fmt.Errorf("required flags: -patrol, -subcamp")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if *sortOrder == 0 {
		if err := db.QueryRow("SELECT sort_order FROM patrols WHERE id = $1", *patrolID).Scan(sortOrder); err != nil {
			return fmt.Errorf("querying patrol sort order: %w", err)
		}
	}

	if _, err := db.Exec(
		"UPDATE patrols SET subcamp_id = $2, sort_order = $3 WHERE id = $1",
		*patrolID, *subcampID, *sortOrder,
	); err != nil {
		return fmt.Errorf("assigning patrol subcamp: %w", err)
	}

	fmt.Println("✓ Patrol assigned to subcamp")
	fmt.Printf("  Patrol:  %s\n", *patrolID)
	fmt.Printf("  Subcamp: %s\n", *subcampID)
	fmt.Printf("  Order:   %d\n", *sortOrder)
	return nil
}

func assignSessionSubcamp() error {
	fs := flag.NewFlagSet("assign-session-subcamp", flag.ExitOnError)
	sessionID := fs.String("session", "", "Session ID (required)")
	subcampID := fs.String("subcamp", "", "Subcamp ID (required)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *sessionID == "" || *subcampID == "" {
		return fmt.Errorf("required flags: -session, -subcamp")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec(
		"INSERT INTO session_subcamps (session_id, subcamp_id) VALUES ($1, $2) ON CONFLICT (session_id, subcamp_id) DO NOTHING",
		*sessionID, *subcampID,
	); err != nil {
		return fmt.Errorf("assigning session subcamp: %w", err)
	}

	fmt.Println("✓ Session assigned to subcamp")
	fmt.Printf("  Session: %s\n", *sessionID)
	fmt.Printf("  Subcamp: %s\n", *subcampID)
	return nil
}

func listAvailable(db *sql.DB, query string) string {
	rows, err := db.Query(query)
	if err != nil {
		return "  (could not list)"
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		fmt.Fprintf(&sb, "  %s  %s\n", id, name)
	}
	if sb.Len() == 0 {
		return "  (none)"
	}
	return sb.String()
}

// ─── Seed Scores ────────────────────────────────────────────────────

func seedScores() error {
	fs := flag.NewFlagSet("seed-scores", flag.ExitOnError)

	sessionID := fs.String("session", "", "Session ID (required)")
	userID := fs.String("user", "", "User ID to attribute submissions to (required)")
	minScore := fs.Int("min", 3, "Minimum random score")
	maxScore := fs.Int("max", 10, "Maximum random score")
	commentedCategories := fs.Int("commented-categories", 0, "Number of criteria per patrol to receive random comments")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Seed random scores for all patrols in a session

Creates submissions with randomised scores for every patrol assigned to a user.
Useful for populating test data.

Usage:
  admin seed-scores -session ses-thu -user usr-morrison -min 3 -max 10

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	if *sessionID == "" || *userID == "" {
		fs.Usage()
		return fmt.Errorf("required flags: -session, -user")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	patrolCount, criterionCount, err := seedScoresForUser(db, *sessionID, *userID, *minScore, *maxScore, *commentedCategories)
	if err != nil {
		return err
	}

	fmt.Printf("\nSeeded %d patrols × %d criteria with random scores [%d-%d]",
		patrolCount, criterionCount, *minScore, *maxScore)
	if *commentedCategories > 0 {
		fmt.Printf(" and %d random comments per patrol", *commentedCategories)
	}
	fmt.Println()
	return nil
}

func seedScoresForUser(db *sql.DB, sessionID, userID string, minScore, maxScore, commentedCategories int) (int, int, error) {
	if maxScore < minScore {
		return 0, 0, fmt.Errorf("max score must be greater than or equal to min score")
	}

	var templateID string
	if err := db.QueryRow("SELECT template_id FROM sessions WHERE id = $1", sessionID).Scan(&templateID); err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, fmt.Errorf("session %q not found", sessionID)
		}
		return 0, 0, fmt.Errorf("looking up session: %w", err)
	}

	rows, err := db.Query("SELECT id FROM criteria WHERE template_id = $1 ORDER BY sort_order", templateID)
	if err != nil {
		return 0, 0, fmt.Errorf("querying criteria: %w", err)
	}
	var criterionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("scanning criterion: %w", err)
		}
		criterionIDs = append(criterionIDs, id)
	}
	rows.Close()
	if len(criterionIDs) == 0 {
		return 0, 0, fmt.Errorf("no criteria found for template %q", templateID)
	}
	if commentedCategories < 0 {
		return 0, 0, fmt.Errorf("commented-categories must be greater than or equal to 0")
	}
	if commentedCategories > len(criterionIDs) {
		commentedCategories = len(criterionIDs)
	}

	patrolRows, err := db.Query(
		`SELECT p.id
		 FROM users u
		 JOIN patrols p ON p.subcamp_id = u.subcamp_id
		 JOIN session_subcamps ss ON ss.session_id = $2 AND ss.subcamp_id = p.subcamp_id
		 WHERE u.id = $1
		 ORDER BY p.sort_order ASC, p.name ASC`,
		userID, sessionID,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("querying subcamp patrols: %w", err)
	}
	var patrolIDs []string
	for patrolRows.Next() {
		var id string
		if err := patrolRows.Scan(&id); err != nil {
			patrolRows.Close()
			return 0, 0, fmt.Errorf("scanning patrol: %w", err)
		}
		patrolIDs = append(patrolIDs, id)
	}
	patrolRows.Close()
	if len(patrolIDs) == 0 {
		return 0, 0, fmt.Errorf("user %q has no patrols in this session", userID)
	}

	var displayName string
	if err := db.QueryRow("SELECT display_name FROM users WHERE id = $1", userID).Scan(&displayName); err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, fmt.Errorf("user %q not found", userID)
		}
		return 0, 0, fmt.Errorf("looking up user display name: %w", err)
	}

	scoreRange := maxScore - minScore + 1
	for _, patrolID := range patrolIDs {
		commentedSet := map[string]bool{}
		if commentedCategories > 0 {
			perm := rand.Perm(len(criterionIDs))
			for _, idx := range perm[:commentedCategories] {
				commentedSet[criterionIDs[idx]] = true
			}
		}

		submissionID := uuid.New().String()

		_, err := db.Exec(
			`INSERT INTO submissions (id, submitted_by, session_id, patrol_id, locked)
			 VALUES ($1, $2, $3, $4, TRUE)
			 ON CONFLICT (session_id, patrol_id) DO UPDATE SET locked = TRUE, submitted_at = NOW(), submitted_by = $2`,
			submissionID, userID, sessionID, patrolID,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("inserting submission for patrol %s: %w", patrolID, err)
		}

		if err := db.QueryRow("SELECT id FROM submissions WHERE session_id = $1 AND patrol_id = $2", sessionID, patrolID).Scan(&submissionID); err != nil {
			return 0, 0, fmt.Errorf("getting submission ID: %w", err)
		}

		if _, err := db.Exec("DELETE FROM submission_scores WHERE submission_id = $1", submissionID); err != nil {
			return 0, 0, fmt.Errorf("clearing old scores: %w", err)
		}
		if _, err := db.Exec("DELETE FROM submission_comments WHERE submission_id = $1", submissionID); err != nil {
			return 0, 0, fmt.Errorf("clearing old comments: %w", err)
		}

		for _, criterionID := range criterionIDs {
			value := minScore + rand.Intn(scoreRange)
			_, err := db.Exec(
				`INSERT INTO submission_scores (id, submission_id, criterion_id, value, scored_by)
				 VALUES ($1, $2, $3, $4, $5)`,
				uuid.New().String(), submissionID, criterionID, value, userID,
			)
			if err != nil {
				return 0, 0, fmt.Errorf("inserting score for criterion %s: %w", criterionID, err)
			}

			if commentedSet[criterionID] {
				if _, err := db.Exec(
					`INSERT INTO submission_comments (id, submission_id, criterion_id, user_id, display_name, comment)
					 VALUES ($1, $2, $3, $4, $5, $6)`,
					uuid.New().String(), submissionID, criterionID, userID, displayName, randomSeedComment(),
				); err != nil {
					return 0, 0, fmt.Errorf("inserting comment for criterion %s: %w", criterionID, err)
				}
			}
		}

		fmt.Printf("  ✓ Seeded scores for patrol %s\n", patrolID)
	}

	return len(patrolIDs), len(criterionIDs), nil
}

func randomSeedComment() string {
	comments := []string{
		"Good standard overall.",
		"Needs a quick tidy before next inspection.",
		"Strong effort from the patrol.",
		"A few small issues to fix.",
		"Consistent and well presented.",
		"Improving steadily day by day.",
	}
	return comments[rand.Intn(len(comments))]
}

// ─── Blair Atholl Demo Seed ─────────────────────────────────────────

type baCriterion struct {
	ID              string
	Title           string
	Description     string
	MinValue        int
	MaxValue        int
	SortOrder       int
	RubricChecklist []string
	RubricBands     []database.CriterionRubricBand
}

type yamlRubricBand struct {
	Label   string   `yaml:"label"`
	Title   string   `yaml:"title"`
	Min     int      `yaml:"min"`
	Max     int      `yaml:"max"`
	Bullets []string `yaml:"bullets"`
}

type yamlCriterionRubric struct {
	Checklist []string         `yaml:"checklist"`
	Bands     []yamlRubricBand `yaml:"bands"`
}

type yamlScoringCategory struct {
	ID          string               `yaml:"id"`
	Name        string               `yaml:"name"`
	Description string               `yaml:"description"`
	Min         int                  `yaml:"min"`
	Max         int                  `yaml:"max"`
	Order       int                  `yaml:"order"`
	Rubric      *yamlCriterionRubric `yaml:"rubric"`
}

type yamlBAPatrol struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	SortOrder int    `yaml:"sort_order"`
}

type yamlBASubcamp struct {
	ID        string         `yaml:"id"`
	Name      string         `yaml:"name"`
	SortOrder int            `yaml:"sort_order"`
	Patrols   []yamlBAPatrol `yaml:"patrols"`
}

type yamlBAStructure struct {
	Version  int             `yaml:"version"`
	Scope    string          `yaml:"scope"`
	Subcamps []yamlBASubcamp `yaml:"subcamps"`
}

type baUser struct {
	ID          string
	Username    string
	DisplayName string
	Subcamp     string
	IsAdmin     bool
	IsCampChief bool
}

type baPatrol struct {
	ID        string
	Name      string
	Subcamp   string
	SortOrder int
}

type baSession struct {
	ID       string
	Name     string
	StartsAt time.Time
	Duration time.Duration
}

func seedBADemo() error {
	fs := flag.NewFlagSet("seed-ba-demo", flag.ExitOnError)

	password := fs.String("password", envOrDefaultAdmin("SCOUTMARK_DEMO_PASSWORD", "password"), "Password for seeded users")
	resetNonUserData := fs.Bool("reset-non-user-data", false, "Delete all non-user data before reseeding (keeps users only)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Seed Blair Atholl demo data

Creates/updates the event, template, criteria, patrols, assignments, and the
July 2026 scoring schedule.
Users are created only if missing and left unchanged when they already exist.
Use -reset-non-user-data to retain users while resetting all other data.

Usage:
  admin seed-ba-demo [-password password]

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	users := baDemoUsers()
	patrols := baDemoPatrols()
	criteria, err := loadBACriteriaFromYAML()
	if err != nil {
		return err
	}
	sessions := baDemoSessions()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning seed transaction: %w", err)
	}
	defer tx.Rollback()

	if *resetNonUserData {
		if err := resetBANonUserData(tx); err != nil {
			return err
		}
	}

	if err := upsertBAEvent(tx); err != nil {
		return err
	}
	if err := upsertBATemplate(tx); err != nil {
		return err
	}
	for _, criterion := range criteria {
		if err := upsertBACriterion(tx, criterion); err != nil {
			return err
		}
	}
	resolvedUsers := make([]baUser, 0, len(users))
	for _, user := range users {
		resolvedUser, err := ensureBAUser(tx, user, string(passwordHash))
		if err != nil {
			return err
		}
		resolvedUsers = append(resolvedUsers, resolvedUser)
	}
	for _, slug := range baDemoSubcamps() {
		if err := upsertBASubcamp(tx, slug); err != nil {
			return err
		}
	}
	for _, patrol := range patrols {
		if err := upsertBAPatrol(tx, patrol); err != nil {
			return err
		}
	}
	for _, user := range resolvedUsers {
		if user.Subcamp == "" {
			continue
		}
		if err := upsertBAUserSubcamp(tx, user.ID, user.Subcamp); err != nil {
			return err
		}
	}
	sessionIDs := make([]string, 0, len(sessions))
	for i, session := range sessions {
		var previousSessionID *string
		awardMostImproved := false
		if i > 0 {
			previousSessionID = strPtr(sessions[i-1].ID)
			awardMostImproved = true
		}

		if err := upsertBASession(
			tx,
			session.ID,
			session.Name,
			session.StartsAt,
			session.Duration,
			true,
			awardMostImproved,
			previousSessionID,
		); err != nil {
			return err
		}
		sessionIDs = append(sessionIDs, session.ID)
	}

	for _, sid := range sessionIDs {
		for _, slug := range baDemoSubcamps() {
			if err := upsertBASessionSubcamp(tx, sid, slug); err != nil {
				return err
			}
		}
	}

	if err := resetBADemoSessionData(tx, sessionIDs); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing seed transaction: %w", err)
	}

	fmt.Println("✓ Blair Atholl demo data seeded")
	fmt.Printf("  Event:       Blair Atholl 2026 (evt-ba-2026)\n")
	fmt.Printf("  Users:       %d leaders plus campchief (existing users left unchanged)\n", len(users)-1)
	fmt.Printf("  Patrols:     %d patrols across 6 subcamps\n", len(patrols))
	fmt.Printf("  Sessions:    %d sessions (18-30 July 2026)\n", len(sessions))
	fmt.Printf("  Reset mode:  %v\n", *resetNonUserData)
	fmt.Printf("  Password:    %s\n", *password)
	return nil
}

func upsertBACriteria() error {
	criteria, err := loadBACriteriaFromYAML()
	if err != nil {
		return err
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	for _, criterion := range criteria {
		if err := upsertBACriterion(tx, criterion); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing criteria upsert transaction: %w", err)
	}

	fmt.Printf("✓ Upserted %d Blair Atholl criteria from scripts/scoring-categories.yaml\n", len(criteria))
	return nil
}

func upsertBAStructure() error {
	structure, err := loadBAStructureFromYAML()
	if err != nil {
		return err
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning structure upsert transaction: %w", err)
	}
	defer tx.Rollback()

	for _, subcamp := range structure.Subcamps {
		if _, err := tx.Exec(
			`INSERT INTO subcamps (id, name)
			 VALUES ($1, $2)
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`,
			subcamp.ID, subcamp.Name,
		); err != nil {
			return fmt.Errorf("upserting subcamp %s: %w", subcamp.ID, err)
		}

		for i, patrol := range subcamp.Patrols {
			sortOrder := patrol.SortOrder
			if sortOrder <= 0 {
				sortOrder = i + 1
			}

			if _, err := tx.Exec(
				`INSERT INTO patrols (id, name, subcamp_id, sort_order)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (id) DO UPDATE SET
				   name = EXCLUDED.name,
				   subcamp_id = EXCLUDED.subcamp_id,
				   sort_order = EXCLUDED.sort_order`,
				patrol.ID, patrol.Name, subcamp.ID, sortOrder,
			); err != nil {
				return fmt.Errorf("upserting patrol %s: %w", patrol.ID, err)
			}
		}
	}

	newSubcampIDs := make([]string, 0, len(structure.Subcamps))
	newPatrolCount := 0
	for _, subcamp := range structure.Subcamps {
		newSubcampIDs = append(newSubcampIDs, subcamp.ID)
		newPatrolCount += len(subcamp.Patrols)
	}

	blockedUsers, err := usersOutsideSubcampSet(tx, newSubcampIDs)
	if err != nil {
		return err
	}
	if len(blockedUsers) > 0 {
		var preview []string
		for i, b := range blockedUsers {
			if i == 8 {
				preview = append(preview, fmt.Sprintf("... and %d more", len(blockedUsers)-i))
				break
			}
			preview = append(preview, fmt.Sprintf("%s(subcamp=%s)", b.username, b.subcampID))
		}
		return fmt.Errorf(
			"refusing to replace structure: %d users are assigned to subcamps missing from YAML: %s",
			len(blockedUsers),
			strings.Join(preview, ", "),
		)
	}

	newPatrolIDs := make([]string, 0, newPatrolCount)
	for _, subcamp := range structure.Subcamps {
		for _, patrol := range subcamp.Patrols {
			newPatrolIDs = append(newPatrolIDs, patrol.ID)
		}
	}

	if _, err := tx.Exec(`DELETE FROM patrols WHERE NOT (id = ANY($1::text[]))`, pq.Array(newPatrolIDs)); err != nil {
		return fmt.Errorf("deleting patrols not present in YAML: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM subcamps WHERE NOT (id = ANY($1::text[]))`, pq.Array(newSubcampIDs)); err != nil {
		return fmt.Errorf("deleting subcamps not present in YAML: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing structure upsert: %w", err)
	}

	fmt.Printf("✓ Replaced BA structure from scripts/subcamps-patrols.yaml\n")
	fmt.Printf("  Subcamps: %d\n", len(newSubcampIDs))
	fmt.Printf("  Patrols:  %d\n", newPatrolCount)
	return nil
}

type userSubcampRef struct {
	username  string
	subcampID string
}

func usersOutsideSubcampSet(tx *sql.Tx, allowedSubcampIDs []string) ([]userSubcampRef, error) {
	if len(allowedSubcampIDs) == 0 {
		return nil, fmt.Errorf("subcamp list cannot be empty")
	}

	rows, err := tx.Query(
		`SELECT username, subcamp_id
		 FROM users
		 WHERE subcamp_id IS NOT NULL
		   AND NOT (subcamp_id = ANY($1::text[]))
		 ORDER BY username`,
		pq.Array(allowedSubcampIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("checking users outside YAML subcamps: %w", err)
	}
	defer rows.Close()

	var out []userSubcampRef
	for rows.Next() {
		var ref userSubcampRef
		if err := rows.Scan(&ref.username, &ref.subcampID); err != nil {
			return nil, fmt.Errorf("scanning user subcamp check row: %w", err)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user subcamp check rows: %w", err)
	}

	return out, nil
}

func loadBAStructureFromYAML() (*yamlBAStructure, error) {
	path := filepath.Join("scripts", "subcamps-patrols.yaml")
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var structure yamlBAStructure
	if err := yaml.Unmarshal(bytes, &structure); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if structure.Version != 1 {
		return nil, fmt.Errorf("%s must have version: 1", path)
	}
	if len(structure.Subcamps) == 0 {
		return nil, fmt.Errorf("no subcamps found in %s", path)
	}

	subcampIDs := make(map[string]struct{}, len(structure.Subcamps))
	patrolIDs := map[string]string{}

	for i := range structure.Subcamps {
		subcamp := &structure.Subcamps[i]
		subcamp.ID = strings.TrimSpace(subcamp.ID)
		subcamp.Name = strings.TrimSpace(subcamp.Name)
		if subcamp.ID == "" {
			return nil, fmt.Errorf("subcamp %d is missing id", i)
		}
		if subcamp.Name == "" {
			return nil, fmt.Errorf("subcamp %q is missing name", subcamp.ID)
		}
		if _, exists := subcampIDs[subcamp.ID]; exists {
			return nil, fmt.Errorf("duplicate subcamp id %q", subcamp.ID)
		}
		subcampIDs[subcamp.ID] = struct{}{}

		for j := range subcamp.Patrols {
			patrol := &subcamp.Patrols[j]
			patrol.ID = strings.TrimSpace(patrol.ID)
			patrol.Name = strings.TrimSpace(patrol.Name)
			if patrol.ID == "" {
				return nil, fmt.Errorf("subcamp %q patrol %d is missing id", subcamp.ID, j)
			}
			if patrol.Name == "" {
				return nil, fmt.Errorf("patrol %q is missing name", patrol.ID)
			}
			if owner, exists := patrolIDs[patrol.ID]; exists {
				return nil, fmt.Errorf("duplicate patrol id %q in subcamps %q and %q", patrol.ID, owner, subcamp.ID)
			}
			patrolIDs[patrol.ID] = subcamp.ID
		}
	}

	if len(patrolIDs) == 0 {
		return nil, fmt.Errorf("no patrols found in %s", path)
	}

	return &structure, nil
}

func loadBACriteriaFromYAML() ([]baCriterion, error) {
	path := filepath.Join("scripts", "scoring-categories.yaml")
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var categories []yamlScoringCategory
	if err := yaml.Unmarshal(bytes, &categories); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(categories) == 0 {
		return nil, fmt.Errorf("no categories found in %s", path)
	}

	criteria := make([]baCriterion, 0, len(categories))
	for i, category := range categories {
		if strings.TrimSpace(category.ID) == "" {
			return nil, fmt.Errorf("category %d is missing id", i)
		}
		if strings.TrimSpace(category.Name) == "" {
			return nil, fmt.Errorf("category %q is missing name", category.ID)
		}
		if category.Max < category.Min {
			return nil, fmt.Errorf("category %q has invalid min/max", category.ID)
		}

		criterion := baCriterion{
			ID:              category.ID,
			Title:           category.Name,
			Description:     category.Description,
			MinValue:        category.Min,
			MaxValue:        category.Max,
			SortOrder:       category.Order,
			RubricChecklist: []string{},
			RubricBands:     []database.CriterionRubricBand{},
		}

		if category.Rubric != nil {
			criterion.RubricChecklist = append(criterion.RubricChecklist, category.Rubric.Checklist...)
			criterion.RubricBands = make([]database.CriterionRubricBand, 0, len(category.Rubric.Bands))
			for _, band := range category.Rubric.Bands {
				if band.Max < band.Min {
					return nil, fmt.Errorf("category %q band %q has invalid min/max", category.ID, band.Label)
				}
				criterion.RubricBands = append(criterion.RubricBands, database.CriterionRubricBand{
					Label:    band.Label,
					Title:    band.Title,
					MinValue: band.Min,
					MaxValue: band.Max,
					Bullets:  append([]string{}, band.Bullets...),
				})
			}
		}

		criteria = append(criteria, criterion)
	}

	return criteria, nil
}

func upsertBAEvent(tx *sql.Tx) error {
	_, err := tx.Exec(
		`INSERT INTO events (id, name, description)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description`,
		"evt-ba-2026", "Blair Atholl 2026", "Blair Atholl Jamborette 2026",
	)
	if err != nil {
		return fmt.Errorf("upserting event: %w", err)
	}
	return nil
}

func upsertBATemplate(tx *sql.Tx) error {
	_, err := tx.Exec(
		`INSERT INTO criteria_templates (id, name, description)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description`,
		"tpl-camp", "Camp Inspection", "Daily camp inspection criteria",
	)
	if err != nil {
		return fmt.Errorf("upserting template: %w", err)
	}
	return nil
}

func upsertBACriterion(tx *sql.Tx, criterion baCriterion) error {
	checklistJSON, err := json.Marshal(criterion.RubricChecklist)
	if err != nil {
		return fmt.Errorf("encoding criterion checklist %s: %w", criterion.ID, err)
	}
	bandsJSON, err := json.Marshal(criterion.RubricBands)
	if err != nil {
		return fmt.Errorf("encoding criterion bands %s: %w", criterion.ID, err)
	}
	_, err = tx.Exec(
		`INSERT INTO criteria (id, template_id, title, description, min_value, max_value, sort_order, rubric_checklist, rubric_bands)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb)
		 ON CONFLICT (id) DO UPDATE SET
		   template_id = EXCLUDED.template_id,
		   title = EXCLUDED.title,
		   description = EXCLUDED.description,
		   min_value = EXCLUDED.min_value,
		   max_value = EXCLUDED.max_value,
		   sort_order = EXCLUDED.sort_order,
		   rubric_checklist = EXCLUDED.rubric_checklist,
		   rubric_bands = EXCLUDED.rubric_bands`,
		criterion.ID, "tpl-camp", criterion.Title, criterion.Description, criterion.MinValue, criterion.MaxValue, criterion.SortOrder, string(checklistJSON), string(bandsJSON),
	)
	if err != nil {
		return fmt.Errorf("upserting criterion %s: %w", criterion.ID, err)
	}
	return nil
}

func applyBARubric() error {
	criteria, err := loadBACriteriaFromYAML()
	if err != nil {
		return err
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	for _, criterion := range criteria {
		checklistJSON, err := json.Marshal(criterion.RubricChecklist)
		if err != nil {
			return fmt.Errorf("encoding checklist for %s: %w", criterion.ID, err)
		}
		bandsJSON, err := json.Marshal(criterion.RubricBands)
		if err != nil {
			return fmt.Errorf("encoding bands for %s: %w", criterion.ID, err)
		}
		result, err := tx.Exec(
			`UPDATE criteria
			 SET rubric_checklist = $2::jsonb, rubric_bands = $3::jsonb
			 WHERE id = $1`,
			criterion.ID, string(checklistJSON), string(bandsJSON),
		)
		if err != nil {
			return fmt.Errorf("updating rubric for %s: %w", criterion.ID, err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("reading rows affected for %s: %w", criterion.ID, err)
		}
		if rowsAffected == 0 {
			return fmt.Errorf("criterion %q not found", criterion.ID)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing rubric updates: %w", err)
	}

	fmt.Println("✓ Blair Atholl rubric applied")
	return nil
}

func ensureBAUser(tx *sql.Tx, user baUser, passwordHash string) (baUser, error) {
	_, err := tx.Exec(
		`INSERT INTO users (id, username, password_hash, display_name, is_admin, is_camp_chief, subcamp_id, password_change_required)
		 VALUES ($1, $2, $3, $4, $5, $6, NULL, FALSE)
		 ON CONFLICT (username) DO UPDATE SET is_camp_chief = EXCLUDED.is_camp_chief`,
		user.ID, user.Username, passwordHash, user.DisplayName, user.IsAdmin, user.IsCampChief,
	)
	if err != nil {
		return baUser{}, fmt.Errorf("ensuring user %s: %w", user.Username, err)
	}

	if err := tx.QueryRow("SELECT id FROM users WHERE username = $1", user.Username).Scan(&user.ID); err != nil {
		return baUser{}, fmt.Errorf("looking up user %s: %w", user.Username, err)
	}
	return user, nil
}

func upsertBASubcamp(tx *sql.Tx, slug string) error {
	_, err := tx.Exec(
		`INSERT INTO subcamps (id, name)
		 VALUES ($1, $2)
		 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`,
		subcampIDFromSlug(slug), displayNameFromUsernamePart(slug),
	)
	if err != nil {
		return fmt.Errorf("upserting subcamp %s: %w", slug, err)
	}
	return nil
}

func lookupBAUserID(users []baUser, username string) (string, error) {
	for _, user := range users {
		if user.Username == username {
			return user.ID, nil
		}
	}
	return "", fmt.Errorf("seed score username %q not found", username)
}

func upsertBAPatrol(tx *sql.Tx, patrol baPatrol) error {
	_, err := tx.Exec(
		`INSERT INTO patrols (id, name, subcamp_id, sort_order)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET
		   name = EXCLUDED.name,
		   subcamp_id = EXCLUDED.subcamp_id,
		   sort_order = EXCLUDED.sort_order`,
		patrol.ID, patrol.Name, subcampIDFromSlug(patrol.Subcamp), patrol.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("upserting patrol %s: %w", patrol.ID, err)
	}
	return nil
}

func upsertBAUserSubcamp(tx *sql.Tx, userID, subcampSlug string) error {
	_, err := tx.Exec(
		`UPDATE users SET subcamp_id = $2 WHERE id = $1`,
		userID, subcampIDFromSlug(subcampSlug),
	)
	if err != nil {
		return fmt.Errorf("assigning subcamp %s to user %s: %w", subcampSlug, userID, err)
	}
	return nil
}

func upsertBASessionSubcamp(tx *sql.Tx, sessionID, subcampSlug string) error {
	_, err := tx.Exec(
		`INSERT INTO session_subcamps (session_id, subcamp_id)
		 VALUES ($1, $2)
		 ON CONFLICT (session_id, subcamp_id) DO NOTHING`,
		sessionID, subcampIDFromSlug(subcampSlug),
	)
	if err != nil {
		return fmt.Errorf("assigning subcamp %s to session %s: %w", subcampSlug, sessionID, err)
	}
	return nil
}

func upsertBASession(tx *sql.Tx, id, name string, startsAt time.Time, duration time.Duration, awardBestPatrol, awardMostImproved bool, previousSessionID *string) error {
	_, err := tx.Exec(
		`INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at, award_best_patrol, award_most_improved, previous_session_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (id) DO UPDATE SET
		   event_id = EXCLUDED.event_id,
		   template_id = EXCLUDED.template_id,
		   name = EXCLUDED.name,
		   starts_at = EXCLUDED.starts_at,
		   ends_at = EXCLUDED.ends_at,
		   award_best_patrol = EXCLUDED.award_best_patrol,
		   award_most_improved = EXCLUDED.award_most_improved,
		   previous_session_id = EXCLUDED.previous_session_id`,
		id, "evt-ba-2026", "tpl-camp", name, startsAt, startsAt.Add(duration), awardBestPatrol, awardMostImproved, previousSessionID,
	)
	if err != nil {
		return fmt.Errorf("upserting session %s: %w", id, err)
	}
	return nil
}

func resetBADemoSessionData(tx *sql.Tx, sessionIDs []string) error {
	for _, sessionID := range sessionIDs {
		if _, err := tx.Exec("DELETE FROM session_awards WHERE session_id = $1", sessionID); err != nil {
			return fmt.Errorf("clearing awards for session %s: %w", sessionID, err)
		}
		if _, err := tx.Exec("DELETE FROM drafts WHERE session_id = $1", sessionID); err != nil {
			return fmt.Errorf("clearing drafts for session %s: %w", sessionID, err)
		}
		if _, err := tx.Exec("DELETE FROM submissions WHERE session_id = $1", sessionID); err != nil {
			return fmt.Errorf("clearing submissions for session %s: %w", sessionID, err)
		}
	}
	return nil
}

func resetBANonUserData(tx *sql.Tx) error {
	if _, err := tx.Exec(`
		TRUNCATE TABLE
			user_sessions,
			session_awards,
			submission_comments,
			draft_comments,
			submission_scores,
			submissions,
			draft_scores,
			drafts,
			session_subcamps,
			sessions,
			criteria,
			criteria_templates,
			patrols,
			subcamps,
			events
		RESTART IDENTITY
		CASCADE`); err != nil {
		return fmt.Errorf("resetting non-user data: %w", err)
	}
	return nil
}

func seedBAPastScores(tx *sql.Tx, sessionID, userID string, criteria []baCriterion, patrols []baPatrol) error {
	var userExists bool
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", userID).Scan(&userExists); err != nil {
		return fmt.Errorf("checking seed score user: %w", err)
	}
	if !userExists {
		return fmt.Errorf("seed score user %q not found", userID)
	}

	for _, patrol := range patrols {
		submissionID := fmt.Sprintf("sub-%s-%s", sessionID, patrol.ID)
		_, err := tx.Exec(
			`INSERT INTO submissions (id, submitted_by, session_id, patrol_id, locked)
			 VALUES ($1, $2, $3, $4, TRUE)
			 ON CONFLICT (session_id, patrol_id) DO UPDATE SET locked = TRUE, submitted_at = NOW(), submitted_by = $2`,
			submissionID, userID, sessionID, patrol.ID,
		)
		if err != nil {
			return fmt.Errorf("upserting submission for patrol %s: %w", patrol.ID, err)
		}

		if err := tx.QueryRow("SELECT id FROM submissions WHERE session_id = $1 AND patrol_id = $2", sessionID, patrol.ID).Scan(&submissionID); err != nil {
			return fmt.Errorf("getting submission ID for patrol %s: %w", patrol.ID, err)
		}

		if _, err := tx.Exec("DELETE FROM submission_scores WHERE submission_id = $1", submissionID); err != nil {
			return fmt.Errorf("clearing scores for patrol %s: %w", patrol.ID, err)
		}

		for _, criterion := range criteria {
			value := 3 + rand.Intn(8)
			_, err := tx.Exec(
				`INSERT INTO submission_scores (id, submission_id, criterion_id, value, scored_by)
				 VALUES ($1, $2, $3, $4, $5)`,
				uuid.New().String(), submissionID, criterion.ID, value, userID,
			)
			if err != nil {
				return fmt.Errorf("inserting score for patrol %s criterion %s: %w", patrol.ID, criterion.ID, err)
			}
		}
	}
	return nil
}

func baDemoSessions() []baSession {
	mustParse := func(value string) time.Time {
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			panic(fmt.Sprintf("invalid BA demo session timestamp %q: %v", value, err))
		}
		return t
	}

	return []baSession{
		{ID: "ses-2026-07-18", Name: "Saturday 18 July", StartsAt: mustParse("2026-07-18T09:00:00+01:00"), Duration: 11 * time.Hour},
		{ID: "ses-2026-07-22", Name: "Wednesday 22 July", StartsAt: mustParse("2026-07-22T07:00:00+01:00"), Duration: 4 * time.Hour},
		{ID: "ses-2026-07-23", Name: "Thursday 23 July", StartsAt: mustParse("2026-07-23T07:00:00+01:00"), Duration: 4 * time.Hour},
		{ID: "ses-2026-07-24", Name: "Friday 24 July", StartsAt: mustParse("2026-07-24T07:00:00+01:00"), Duration: 4 * time.Hour},
		{ID: "ses-2026-07-25", Name: "Saturday 25 July", StartsAt: mustParse("2026-07-25T07:00:00+01:00"), Duration: 4 * time.Hour},
		{ID: "ses-2026-07-27", Name: "Monday 27 July", StartsAt: mustParse("2026-07-27T07:00:00+01:00"), Duration: 4 * time.Hour},
		{ID: "ses-2026-07-28", Name: "Tuesday 28 July", StartsAt: mustParse("2026-07-28T07:00:00+01:00"), Duration: 4 * time.Hour},
		{ID: "ses-2026-07-29", Name: "Wednesday 29 July", StartsAt: mustParse("2026-07-29T07:00:00+01:00"), Duration: 4 * time.Hour},
		{ID: "ses-2026-07-30", Name: "Thursday 30 July", StartsAt: mustParse("2026-07-30T07:00:00+01:00"), Duration: 4 * time.Hour},
	}
}

func baDemoPatrols() []baPatrol {
	subcamps := baDemoSubcamps()
	patrols := make([]baPatrol, 0, len(subcamps)*3)
	for _, subcamp := range subcamps {
		for i := 1; i <= 3; i++ {
			patrols = append(patrols, baPatrol{
				ID:        fmt.Sprintf("pat-%s-%d", subcamp, i),
				Name:      fmt.Sprintf("%s Site %d", displayNameFromUsernamePart(subcamp), i),
				Subcamp:   subcamp,
				SortOrder: i,
			})
		}
	}
	return patrols
}

func baDemoSubcamps() []string {
	return []string{"mcdonald", "morrison", "robertson", "stewart", "murray", "mclean"}
}

func baDemoUsers() []baUser {
	usernames := []string{
		"mcdonald.mark",
		"mcdonald.lee",
		"mcdonald.heather.g",
		"mcdonald.heather.w",
		"mcdonald.sam",
		"mcdonald.joe",
		"mcdonald.sarah",
		"mcdonald.gemma",
		"mcdonald.tara",
		"mcdonald.kerry",
		"morrison.stacey",
		"morrison.john",
		"morrison.gill",
		"morrison.iona",
		"morrison.joyce",
		"morrison.marc",
		"morrison.graham",
		"morrison.brodie",
		"morrison.sj",
		"morrison.abby",
		"morrison.ally",
		"morrison.nicholas",
		"robertson.gemma",
		"robertson.rachel",
		"robertson.paula",
		"robertson.emma",
		"robertson.matt",
		"robertson.euan",
		"robertson.kieran",
		"robertson.callum",
		"robertson.theresa",
		"robertson.abby",
		"robertson.james",
		"stewart.jamie",
		"stewart.meghan",
		"stewart.amanda",
		"stewart.ross",
		"stewart.leanne",
		"stewart.kieran",
		"stewart.ewan",
		"stewart.amy",
		"stewart.mike",
		"stewart.belen",
		"stewart.mieke",
		"murray.ross",
		"murray.ryan",
		"murray.ea",
		"murray.iain",
		"murray.daniel",
		"murray.caroline",
		"murray.fiona",
		"murray.jackie",
		"murray.patri",
		"murray.hamish",
		"murray.leslie",
		"mclean.may",
		"mclean.jonny",
		"mclean.hollie",
		"mclean.morvin",
		"mclean.gauldy",
		"mclean.graeme",
		"mclean.lisa",
		"mclean.julie",
		"mclean.mathew",
		"mclean.rafa",
		"mclean.tanner",
	}

	users := []baUser{{ID: "usr-campchief", Username: "campchief", DisplayName: "Camp Chief", IsAdmin: true, IsCampChief: true}}
	for _, username := range usernames {
		parts := strings.Split(username, ".")
		subcamp := parts[0]
		users = append(users, baUser{
			ID:          "usr-" + strings.ReplaceAll(username, ".", "-"),
			Username:    username,
			DisplayName: displayNameFromUsername(username),
			Subcamp:     subcamp,
		})
	}
	return users
}

func displayNameFromUsername(username string) string {
	parts := strings.Split(username, ".")
	for i, part := range parts {
		parts[i] = displayNameFromUsernamePart(part)
	}
	return strings.Join(parts, " ")
}

func displayNameFromUsernamePart(part string) string {
	if part == "" {
		return ""
	}
	if len(part) <= 2 {
		return strings.ToUpper(part)
	}
	return strings.ToUpper(part[:1]) + part[1:]
}

func strPtr(value string) *string {
	return &value
}

func subcampIDFromSlug(slug string) string {
	return "sub-" + slug
}

func envOrDefaultAdmin(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func emptyAsDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// ─── Input Helpers ──────────────────────────────────────────────────

func prompt(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("%s cannot be empty", strings.ToLower(label))
	}
	return input, nil
}

func promptDefault(reader *bufio.Reader, label, fallback string) (string, error) {
	fmt.Printf("%s [%s]: ", label, fallback)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return fallback, nil
	}
	return input, nil
}

func promptPassword(label string) (string, error) {
	fmt.Printf("%s: ", label)
	bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	if len(bytes) == 0 {
		return "", fmt.Errorf("password cannot be empty")
	}
	return string(bytes), nil
}
