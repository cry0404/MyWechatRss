package store

import (
	"context"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
)

func (s *Store) CreateUser(ctx context.Context, u *model.User) error {
	now := time.Now().Unix()
	if u.CreatedAt == 0 {
		u.CreatedAt = now
	}
	res, err := s.db.ExecContext(ctx, `
        INSERT INTO users (username, email, password_hash, created_at, is_admin)
        VALUES (?, ?, ?, ?, ?)
    `, u.Username, u.Email, u.PasswordHash, u.CreatedAt, boolToInt(u.IsAdmin))
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, username, email, password_hash, created_at, is_admin
        FROM users WHERE username = ?
    `, username)
	return scanUser(row)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, username, email, password_hash, created_at, is_admin
        FROM users WHERE id = ?
    `, id)
	return scanUser(row)
}

func (s *Store) UpdateUserEmail(ctx context.Context, id int64, email string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email = ? WHERE id = ?`,
		email, id,
	)
	return err
}

func (s *Store) UpdateUserUsername(ctx context.Context, id int64, username string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET username = ? WHERE id = ?`,
		username, id,
	)
	return err
}

func (s *Store) UpdateUserPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ?`,
		passwordHash, id,
	)
	return err
}

func (s *Store) HasAnyUser(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func scanUser(row rowScanner) (*model.User, error) {
	u := &model.User{}
	var isAdmin int
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &isAdmin); err != nil {
		return nil, wrapNotFound(err)
	}
	u.IsAdmin = isAdmin != 0
	return u, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
