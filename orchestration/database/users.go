package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// ErrUserExists is returned by CreateUser when the username is already taken.
var ErrUserExists = errors.New("user already exists")

// UserHash returns the stored bcrypt hash for username, for the login compare.
// sql.ErrNoRows when the user does not exist.
func (s *Store) UserHash(username string) (string, error) {
	var hash string
	err := s.DB.QueryRow(
		"SELECT pass_hash FROM csfleet_web_users WHERE username = ?", username).Scan(&hash)
	if err != nil {
		return "", err
	}
	return hash, nil
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.DB.Query(
		"SELECT username, created_at FROM csfleet_web_users ORDER BY username")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.Username, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CreateUser(username, passHash string) error {
	_, err := s.DB.Exec(
		"INSERT INTO csfleet_web_users (username, pass_hash) VALUES (?, ?)",
		username, passHash)
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return ErrUserExists
		}
		return fmt.Errorf("create user %q: %w", username, err)
	}
	return nil
}

func (s *Store) DeleteUser(username string) error {
	res, err := s.DB.Exec("DELETE FROM csfleet_web_users WHERE username = ?", username)
	if err != nil {
		return fmt.Errorf("delete user %q: %w", username, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetUserPassword(username, passHash string) error {
	res, err := s.DB.Exec(
		"UPDATE csfleet_web_users SET pass_hash = ? WHERE username = ?", passHash, username)
	if err != nil {
		return fmt.Errorf("set password %q: %w", username, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
