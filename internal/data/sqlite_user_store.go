package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SQLiteUserStore persists users in SQLite.
type SQLiteUserStore struct {
	db *sql.DB
}

// NewSQLiteUserStore returns a user store backed by db.
func NewSQLiteUserStore(db *sql.DB) *SQLiteUserStore {
	return &SQLiteUserStore{db: db}
}

func (s *SQLiteUserStore) CreateUser(ctx context.Context, user *User) (*User, error) {
	created, err := scanUser(s.db.QueryRowContext(ctx, `
		INSERT INTO users (user_id, handle, alt_handle, email, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING user_id, handle, alt_handle, email, version, created_at, updated_at`,
		user.UserID,
		user.Handle,
		user.AltHandle,
		user.Email,
		user.Version,
		UnixSec(user.CreatedAt),
		UnixSec(user.UpdatedAt),
	))
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return nil, ErrDuplicateRecord
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return created, nil
}

func (s *SQLiteUserStore) GetUserByHandle(ctx context.Context, handle string) (*User, error) {
	user, err := scanUser(s.db.QueryRowContext(ctx, `
		SELECT user_id, handle, alt_handle, email, version, created_at, updated_at
		FROM users
		WHERE handle = ?`,
		handle,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by handle: %w", err)
	}
	return user, nil
}

type userScanner interface {
	Scan(...any) error
}

func scanUser(scanner userScanner) (*User, error) {
	var user User
	var handle, altHandle, email sql.NullString
	var createdAt, updatedAt int64
	if err := scanner.Scan(
		&user.UserID,
		&handle,
		&altHandle,
		&email,
		&user.Version,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	user.Handle = stringPointer(handle)
	user.AltHandle = stringPointer(altHandle)
	user.Email = stringPointer(email)
	user.CreatedAt = TimeFromUnixSec(createdAt)
	user.UpdatedAt = TimeFromUnixSec(updatedAt)
	return &user, nil
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

var _ UserStore = (*SQLiteUserStore)(nil)
