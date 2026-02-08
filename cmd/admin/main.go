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
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

const usage = `Scoutmark admin CLI

Usage:
  admin <command>

Commands:
  create-user       Create a new user interactively
  change-password   Change a user's password
  list-users        List all users
  create-session    Create a scoring session
  list-sessions     List all sessions with status

Environment:
  DATABASE_URL      MySQL connection string (default: root:scoutmark@tcp(localhost:3306)/scoutmark?parseTime=true)
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
	case "create-session":
		err = createSession()
	case "list-sessions":
		err = listSessions()
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
		dsn = "root:scoutmark@tcp(localhost:3306)/scoutmark?parseTime=true"
	}

	db, err := sql.Open("mysql", dsn)
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
	reader := bufio.NewReader(os.Stdin)

	username, err := prompt(reader, "Username")
	if err != nil {
		return err
	}

	password, err := promptPassword("Password")
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

	displayName, err := prompt(reader, "Display name")
	if err != nil {
		return err
	}

	adminInput, err := promptDefault(reader, "Admin user?", "N")
	if err != nil {
		return err
	}
	isAdmin := strings.HasPrefix(strings.ToLower(adminInput), "y")

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
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", username).Scan(&exists); err != nil {
		return fmt.Errorf("checking existing user: %w", err)
	}
	if exists {
		return fmt.Errorf("user %q already exists", username)
	}

	userID := uuid.New().String()
	_, err = db.Exec(
		"INSERT INTO users (id, username, password_hash, display_name, is_admin) VALUES (?, ?, ?, ?, ?)",
		userID, username, string(hash), displayName, isAdmin,
	)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ User created")
	fmt.Printf("  ID:           %s\n", userID)
	fmt.Printf("  Username:     %s\n", username)
	fmt.Printf("  Display name: %s\n", displayName)
	fmt.Printf("  Admin:        %v\n", isAdmin)
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

	result, err := db.Exec("UPDATE users SET password_hash = ? WHERE username = ?", string(hash), username)
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

	rows, err := db.Query("SELECT id, username, display_name, is_admin, created_at FROM users ORDER BY created_at ASC")
	if err != nil {
		return fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tDISPLAY NAME\tADMIN\tCREATED")
	fmt.Fprintln(w, "──\t────────\t────────────\t─────\t───────")

	count := 0
	for rows.Next() {
		var id, username, displayName, createdAt string
		var isAdmin bool
		if err := rows.Scan(&id, &username, &displayName, &isAdmin, &createdAt); err != nil {
			return fmt.Errorf("scanning user: %w", err)
		}

		admin := ""
		if isAdmin {
			admin = "✓"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, username, displayName, admin, createdAt)
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
	if err := db.QueryRow("SELECT name FROM events WHERE id = ?", *eventID).Scan(&eventName); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("event %q not found\n\nAvailable events:\n%s", *eventID, listAvailable(db, "SELECT id, name FROM events ORDER BY name"))
		}
		return fmt.Errorf("checking event: %w", err)
	}

	// Verify template exists
	var templateName string
	if err := db.QueryRow("SELECT name FROM criteria_templates WHERE id = ?", *templateID).Scan(&templateName); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("template %q not found\n\nAvailable templates:\n%s", *templateID, listAvailable(db, "SELECT id, name FROM criteria_templates ORDER BY name"))
		}
		return fmt.Errorf("checking template: %w", err)
	}

	_, err = db.Exec(
		"INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, *eventID, *templateID, *name, startsAt, endsAt,
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
