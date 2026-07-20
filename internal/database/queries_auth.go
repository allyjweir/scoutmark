package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrUserNotFound = errors.New("user not found")

// UserRow represents a user record from the database.
type UserRow struct {
	ID                     string
	Username               string
	PasswordHash           string
	DisplayName            string
	IsAdmin                bool
	IsCampChief            bool
	SubcampID              *string
	PasswordChangeRequired bool
	CreatedAt              time.Time
}

// AdminUserRow is the user data safe to return from admin user management APIs.
type AdminUserRow struct {
	ID          string
	Username    string
	DisplayName string
	IsAdmin     bool
	SubcampID   *string
	SubcampName *string
}

func (d *DB) ListUsers(ctx context.Context) ([]AdminUserRow, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT u.id, u.username, u.display_name, u.is_admin, u.subcamp_id, sc.name
		 FROM users u LEFT JOIN subcamps sc ON sc.id = u.subcamp_id
		 ORDER BY u.display_name ASC, u.username ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()
	var users []AdminUserRow
	for rows.Next() {
		var user AdminUserRow
		if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.IsAdmin, &user.SubcampID, &user.SubcampName); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (d *DB) CreateUser(ctx context.Context, username, passwordHash, displayName, subcampID string, isAdmin bool) (*AdminUserRow, error) {
	id := uuid.NewString()
	row := d.QueryRowContext(ctx,
		`INSERT INTO users (id, username, password_hash, display_name, is_admin, subcamp_id, password_change_required)
		 VALUES ($1, $2, $3, $4, $5, $6, TRUE)
		 RETURNING id, username, display_name, is_admin, subcamp_id`,
		id, username, passwordHash, displayName, isAdmin, subcampID,
	)
	user := &AdminUserRow{}
	if err := row.Scan(&user.ID, &user.Username, &user.DisplayName, &user.IsAdmin, &user.SubcampID); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	if user.SubcampID != nil {
		if err := d.QueryRowContext(ctx, `SELECT name FROM subcamps WHERE id = $1`, *user.SubcampID).Scan(&user.SubcampName); err != nil {
			return nil, fmt.Errorf("getting user subcamp: %w", err)
		}
	}
	return user, nil
}

// AdminSetUserPassword always requires the account to choose a new password at next sign-in.
func (d *DB) AdminSetUserPassword(ctx context.Context, userID, passwordHash string) error {
	result, err := d.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, password_change_required = TRUE WHERE id = $2`,
		passwordHash, userID,
	)
	if err != nil {
		return fmt.Errorf("resetting user password: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking password reset: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SessionRow represents a user session (auth token) record.
type SessionRow struct {
	Token     string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// GetUserByUsername fetches a user by their username.
func (d *DB) GetUserByUsername(ctx context.Context, username string) (*UserRow, error) {
	row := d.QueryRowContext(ctx,
		"SELECT id, username, password_hash, display_name, is_admin, is_camp_chief, subcamp_id, password_change_required, created_at FROM users WHERE username = $1",
		username,
	)

	u := &UserRow{}
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.IsAdmin, &u.IsCampChief, &u.SubcampID, &u.PasswordChangeRequired, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	return u, nil
}

// GetUserByID fetches a user by their ID.
func (d *DB) GetUserByID(ctx context.Context, id string) (*UserRow, error) {
	row := d.QueryRowContext(ctx,
		"SELECT id, username, password_hash, display_name, is_admin, is_camp_chief, subcamp_id, password_change_required, created_at FROM users WHERE id = $1",
		id,
	)

	u := &UserRow{}
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.IsAdmin, &u.IsCampChief, &u.SubcampID, &u.PasswordChangeRequired, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	return u, nil
}

// CreateUserSession creates a new auth session token for a user.
func (d *DB) CreateUserSession(ctx context.Context, userID string, duration time.Duration) (*SessionRow, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	expiresAt := time.Now().Add(duration)

	_, err := d.ExecContext(ctx,
		"INSERT INTO user_sessions (token, user_id, expires_at) VALUES ($1, $2, $3)",
		token, userID, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting session: %w", err)
	}

	return &SessionRow{
		Token:     token,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}, nil
}

// GetUserSession fetches a valid (non-expired) session by token.
func (d *DB) GetUserSession(ctx context.Context, token string) (*SessionRow, error) {
	row := d.QueryRowContext(ctx,
		"SELECT token, user_id, expires_at, created_at FROM user_sessions WHERE token = $1 AND expires_at > NOW()",
		token,
	)

	s := &SessionRow{}
	err := row.Scan(&s.Token, &s.UserID, &s.ExpiresAt, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}
	return s, nil
}

// DeleteUserSession removes a session token (logout).
func (d *DB) DeleteUserSession(ctx context.Context, token string) error {
	_, err := d.ExecContext(ctx, "DELETE FROM user_sessions WHERE token = $1", token)
	return err
}

// DeleteExpiredSessions removes all expired session tokens.
func (d *DB) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	result, err := d.ExecContext(ctx, "DELETE FROM user_sessions WHERE expires_at < NOW()")
	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}
	return result.RowsAffected()
}

// UserOwnsPatrol checks whether a user is assigned to the given patrol.
func (d *DB) UserOwnsPatrol(ctx context.Context, userID, patrolID string) (bool, error) {
	row := d.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM users u
		 JOIN patrols p ON p.id = $2
		 WHERE u.id = $1
		   AND (u.is_admin = TRUE OR (u.subcamp_id IS NOT NULL AND u.subcamp_id = p.subcamp_id))`,
		userID, patrolID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("checking patrol ownership: %w", err)
	}
	return count > 0, nil
}

// UserOwnsSessionPatrol checks whether a user can access a patrol within a session.
func (d *DB) UserOwnsSessionPatrol(ctx context.Context, userID, sessionID, patrolID string) (bool, error) {
	row := d.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM users u
		 JOIN sessions s ON s.id = $2
		 JOIN patrols p ON p.id = $3
		 JOIN session_subcamps ss ON ss.session_id = $2 AND ss.subcamp_id = p.subcamp_id
		 WHERE u.id = $1
		   AND (
		     NOT EXISTS (SELECT 1 FROM session_patrols spx WHERE spx.session_id = $2)
		     OR EXISTS (SELECT 1 FROM session_patrols sp WHERE sp.session_id = $2 AND sp.patrol_id = p.id)
		   )
		   AND (
		     (s.round_type = 'round2' AND u.is_admin = TRUE)
		     OR (s.round_type <> 'round2' AND (u.is_admin = TRUE OR (u.subcamp_id IS NOT NULL AND u.subcamp_id = p.subcamp_id)))
		   )`,
		userID, sessionID, patrolID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("checking session patrol ownership: %w", err)
	}
	return count > 0, nil
}

// UserCanAccessSession checks whether a user can access any patrol in the session.
func (d *DB) UserCanAccessSession(ctx context.Context, userID, sessionID string) (bool, error) {
	row := d.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM users u
		 JOIN sessions s ON s.id = $2
		 JOIN session_subcamps ss ON ss.session_id = $2
		 WHERE u.id = $1
		   AND (
		     (s.round_type = 'round2' AND u.is_admin = TRUE)
		     OR (s.round_type <> 'round2' AND (u.is_admin = TRUE OR (u.subcamp_id IS NOT NULL AND u.subcamp_id = ss.subcamp_id)))
		   )`,
		userID, sessionID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("checking session access: %w", err)
	}
	return count > 0, nil
}

// UpdateUserPassword updates a user's password hash and clears the password_change_required flag.
func (d *DB) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	_, err := d.ExecContext(ctx,
		"UPDATE users SET password_hash = $1, password_change_required = FALSE WHERE id = $2",
		passwordHash, userID,
	)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}
	return nil
}
