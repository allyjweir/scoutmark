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
	crand "crypto/rand"
	"database/sql"
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

const usage = `Scoutmark admin CLI

Usage:
  admin <command>

Commands:
  create-user       Create a new user (interactive or with flags)
  change-password   Change a user's password
  list-users        List all users
  create-event      Create an event
  create-template   Create a criteria template
  add-criterion     Add a criterion to a template
  create-subcamp    Create a subcamp
  create-patrol     Create a patrol (within a subcamp)
  assign-subcamp    Assign a subcamp to a user
  create-session    Create a scoring session
  update-session    Update session settings (awards, previous session)
  list-sessions     List all sessions with status
  seed-scores       Seed random submission scores for all patrols in a session

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
	case "list-users":
		err = listUsers()
	case "create-event":
		err = createEvent()
	case "create-template":
		err = createTemplate()
	case "add-criterion":
		err = addCriterion()
	case "create-subcamp":
		err = createSubcamp()
	case "create-patrol":
		err = createPatrol()
	case "assign-subcamp":
		err = assignSubcamp()
	case "create-session":
		err = createSession()
	case "update-session":
		err = updateSession()
	case "list-sessions":
		err = listSessions()
	case "seed-scores":
		err = seedScores()
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
	flagRole := fs.String("role", "scorer", "User role: scorer, camp_chief, admin")
	flagID := fs.String("id", "", "User ID (default: auto-generated UUID)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	var username, password, displayName, role string

	if *flagUsername != "" {
		// Non-interactive (flag-driven) mode for scripting
		username = *flagUsername
		password = *flagPassword
		displayName = *flagDisplay
		role = *flagRole
		if password == "" {
			return fmt.Errorf("-password is required in non-interactive mode")
		}
		if displayName == "" {
			displayName = username
		}
		// Validate role
		switch role {
		case "scorer", "camp_chief", "admin":
		default:
			return fmt.Errorf("invalid role %q: must be scorer, camp_chief, or admin", role)
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

		roleInput, err := promptDefault(reader, "Role (scorer/camp_chief/admin)", "scorer")
		if err != nil {
			return err
		}
		role = strings.TrimSpace(roleInput)
		switch role {
		case "scorer", "camp_chief", "admin":
		default:
			return fmt.Errorf("invalid role %q: must be scorer, camp_chief, or admin", role)
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

	userID := *flagID
	if userID == "" {
		userID = uuid.New().String()
	}
	_, err = db.Exec(
		"INSERT INTO users (id, username, password_hash, display_name, role) VALUES ($1, $2, $3, $4, $5)",
		userID, username, string(hash), displayName, role,
	)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ User created")
	fmt.Printf("  ID:           %s\n", userID)
	fmt.Printf("  Username:     %s\n", username)
	fmt.Printf("  Display name: %s\n", displayName)
	fmt.Printf("  Role:         %s\n", role)
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

func listUsers() error {
	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, username, display_name, role, created_at FROM users ORDER BY created_at ASC")
	if err != nil {
		return fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tDISPLAY NAME\tROLE\tCREATED")
	fmt.Fprintln(w, "──\t────────\t────────────\t────\t───────")

	count := 0
	for rows.Next() {
		var id, username, displayName, createdAt, role string
		if err := rows.Scan(&id, &username, &displayName, &role, &createdAt); err != nil {
			return fmt.Errorf("scanning user: %w", err)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, username, displayName, role, createdAt)
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}

	w.Flush()
	fmt.Printf("\n%d user(s)\n", count)
	return nil
}

func createSession() error {
	fs := flag.NewFlagSet("create-session", flag.ExitOnError)

	eventID := fs.String("event", "", "Event ID (required)")
	templateID := fs.String("template", "", "Criteria template ID (required)")
	name := fs.String("name", "", "Session name (required)")
	startStr := fs.String("start", "", `Start time in RFC3339 or "now" (default: now)`)
	durationStr := fs.String("duration", "3h", "Duration from start (e.g. 2h, 6h, 30m)")
	sessionID := fs.String("id", "", "Session ID (default: auto-generated UUID)")
	awardBestPatrol := fs.Bool("award-best-patrol", false, "Enable Best Patrol award")
	awardMostImproved := fs.Bool("award-most-improved", false, "Enable Most Improved award")
	previousSessionID := fs.String("previous-session", "", "ID of the previous session (for chaining / Most Improved)")

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

	if *eventID == "" || *templateID == "" || *name == "" {
		fs.Usage()
		return fmt.Errorf("required flags: -event, -template, -name")
	}

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

	_, err = db.Exec(
		`INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at, award_best_patrol, award_most_improved, previous_session_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, *eventID, *templateID, *name, startsAt, endsAt, *awardBestPatrol, *awardMostImproved, prevID,
	)
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Session created")
	fmt.Printf("  ID:       %s\n", id)
	fmt.Printf("  Name:     %s\n", *name)
	fmt.Printf("  Event:    %s (%s)\n", eventName, *eventID)
	fmt.Printf("  Template: %s (%s)\n", templateName, *templateID)
	fmt.Printf("  Starts:   %s\n", startsAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Ends:     %s\n", endsAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Duration: %s\n", duration)
	if *awardBestPatrol || *awardMostImproved {
		fmt.Printf("  Awards:   best_patrol=%v most_improved=%v\n", *awardBestPatrol, *awardMostImproved)
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

func createSubcamp() error {
	fs := flag.NewFlagSet("create-subcamp", flag.ExitOnError)

	id := fs.String("id", "", "Subcamp ID (default: auto-generated UUID)")
	name := fs.String("name", "", "Subcamp name (required)")
	eventID := fs.String("event", "", "Event ID (required)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Create a subcamp

Usage:
  admin create-subcamp -event evt-min -name "Morrison" [-id sub-mor]

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *name == "" || *eventID == "" {
		fs.Usage()
		return fmt.Errorf("required flags: -name, -event")
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

	_, err = db.Exec(
		"INSERT INTO subcamps (id, event_id, name) VALUES ($1, $2, $3)",
		subcampID, *eventID, *name,
	)
	if err != nil {
		return fmt.Errorf("inserting subcamp: %w", err)
	}

	fmt.Println("✓ Subcamp created")
	fmt.Printf("  ID:    %s\n", subcampID)
	fmt.Printf("  Name:  %s\n", *name)
	fmt.Printf("  Event: %s\n", *eventID)
	return nil
}

func createPatrol() error {
	fs := flag.NewFlagSet("create-patrol", flag.ExitOnError)

	id := fs.String("id", "", "Patrol ID (default: auto-generated UUID)")
	name := fs.String("name", "", "Patrol name (required)")
	subcampID := fs.String("subcamp", "", "Subcamp ID (required)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Create a patrol

Usage:
  admin create-patrol -subcamp sub-mor -name "France" [-id pat-mor-1]

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

	_, err = db.Exec(
		"INSERT INTO patrols (id, name, subcamp_id) VALUES ($1, $2, $3)",
		patrolID, *name, *subcampID,
	)
	if err != nil {
		return fmt.Errorf("inserting patrol: %w", err)
	}

	fmt.Println("✓ Patrol created")
	fmt.Printf("  ID:      %s\n", patrolID)
	fmt.Printf("  Name:    %s\n", *name)
	fmt.Printf("  Subcamp: %s\n", *subcampID)
	return nil
}

func assignSubcamp() error {
	fs := flag.NewFlagSet("assign-subcamp", flag.ExitOnError)

	userID := fs.String("user", "", "User ID (required)")
	subcampID := fs.String("subcamp", "", "Subcamp ID (required)")
	sortOrder := fs.Int("order", 0, "Sort order (default: auto-increment)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Assign a subcamp to a user

Usage:
  admin assign-subcamp -user usr-morrison -subcamp sub-mor [-order 1]

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *userID == "" || *subcampID == "" {
		fs.Usage()
		return fmt.Errorf("required flags: -user, -subcamp")
	}

	db, err := connectDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Auto-increment sort order if not specified
	if *sortOrder == 0 {
		var maxOrder int
		err := db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) FROM user_subcamps WHERE user_id = $1", *userID).Scan(&maxOrder)
		if err != nil {
			return fmt.Errorf("querying max sort order: %w", err)
		}
		*sortOrder = maxOrder + 1
	}

	_, err = db.Exec(
		"INSERT INTO user_subcamps (user_id, subcamp_id, sort_order) VALUES ($1, $2, $3)",
		*userID, *subcampID, *sortOrder,
	)
	if err != nil {
		return fmt.Errorf("assigning subcamp: %w", err)
	}

	fmt.Println("✓ Subcamp assigned")
	fmt.Printf("  User:    %s\n", *userID)
	fmt.Printf("  Subcamp: %s\n", *subcampID)
	fmt.Printf("  Order:   %d\n", *sortOrder)
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
	seed := time.Now().UnixNano()
	if err := binary.Read(crand.Reader, binary.LittleEndian, &seed); err != nil {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	sessionID := fs.String("session", "", "Session ID (required)")
	userID := fs.String("user", "", "User ID to attribute submissions to (required)")
	minScore := fs.Int("min", 3, "Minimum random score")
	maxScore := fs.Int("max", 10, "Maximum random score")

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

	// Get the session's template
	var templateID string
	if err := db.QueryRow("SELECT template_id FROM sessions WHERE id = $1", *sessionID).Scan(&templateID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("session %q not found", *sessionID)
		}
		return fmt.Errorf("looking up session: %w", err)
	}

	// Get criteria for the template
	rows, err := db.Query("SELECT id FROM criteria WHERE template_id = $1 ORDER BY sort_order", templateID)
	if err != nil {
		return fmt.Errorf("querying criteria: %w", err)
	}
	var criterionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scanning criterion: %w", err)
		}
		criterionIDs = append(criterionIDs, id)
	}
	rows.Close()
	if len(criterionIDs) == 0 {
		return fmt.Errorf("no criteria found for template %q", templateID)
	}

	// Get patrols assigned to the user (via subcamps)
	patrolRows, err := db.Query(
		`SELECT p.id FROM user_subcamps us
		 JOIN patrols p ON p.subcamp_id = us.subcamp_id
		 WHERE us.user_id = $1 ORDER BY us.sort_order, p.name`, *userID)
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
	if len(patrolIDs) == 0 {
		return fmt.Errorf("user %q has no assigned patrols", *userID)
	}

	// Create submissions with random scores for each patrol
	scoreRange := *maxScore - *minScore + 1
	for _, patrolID := range patrolIDs {
		submissionID := uuid.New().String()

		// Upsert submission
		_, err := db.Exec(
			`INSERT INTO submissions (id, submitted_by, session_id, patrol_id, locked)
			 VALUES ($1, $2, $3, $4, TRUE)
			 ON CONFLICT (session_id, patrol_id) DO UPDATE SET locked = TRUE, submitted_at = NOW(), submitted_by = $2`,
			submissionID, *userID, *sessionID, patrolID,
		)
		if err != nil {
			return fmt.Errorf("inserting submission for patrol %s: %w", patrolID, err)
		}

		// Get actual submission ID (in case of conflict)
		if err := db.QueryRow(
			"SELECT id FROM submissions WHERE session_id = $1 AND patrol_id = $2",
			*sessionID, patrolID,
		).Scan(&submissionID); err != nil {
			return fmt.Errorf("getting submission ID: %w", err)
		}

		// Clear old scores if re-seeding
		_, err = db.Exec("DELETE FROM submission_scores WHERE submission_id = $1", submissionID)
		if err != nil {
			return fmt.Errorf("clearing old scores: %w", err)
		}

		// Insert random scores
		for _, criterionID := range criterionIDs {
			value := *minScore + rng.Intn(scoreRange)
			scoreID := uuid.New().String()
			_, err := db.Exec(
				`INSERT INTO submission_scores (id, submission_id, criterion_id, value, comment, scored_by)
				 VALUES ($1, $2, $3, $4, '', $5)`,
				scoreID, submissionID, criterionID, value, *userID,
			)
			if err != nil {
				return fmt.Errorf("inserting score for criterion %s: %w", criterionID, err)
			}
		}

		fmt.Printf("  ✓ Seeded scores for patrol %s\n", patrolID)
	}

	fmt.Printf("\nSeeded %d patrols × %d criteria with random scores [%d-%d]\n",
		len(patrolIDs), len(criterionIDs), *minScore, *maxScore)
	return nil
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
