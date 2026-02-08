package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// UserRow represents a user record from the database.
type UserRow struct {
	ID           string
	Username     string
	PasswordHash string
	DisplayName  string
	IsAdmin      bool
	CreatedAt    time.Time
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
		"SELECT id, username, password_hash, display_name, is_admin, created_at FROM users WHERE username = $1",
		username,
	)

	u := &UserRow{}
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.IsAdmin, &u.CreatedAt)
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
		"SELECT id, username, password_hash, display_name, is_admin, created_at FROM users WHERE id = $1",
		id,
	)

	u := &UserRow{}
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.IsAdmin, &u.CreatedAt)
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
		"SELECT COUNT(*) FROM user_patrols WHERE user_id = $1 AND patrol_id = $2",
		userID, patrolID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("checking patrol ownership: %w", err)
	}
	return count > 0, nil
}
