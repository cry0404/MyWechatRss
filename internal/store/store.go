package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/cry0404/MyWechatRss/internal/crypto"
)

type Store struct {
	db    *sql.DB
	codec *crypto.Codec

	deadHook func(ctx context.Context, userID int64, lastErr string)
}

func Open(dbPath string, codec *crypto.Codec) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := applyMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return &Store{db: db, codec: codec}, nil
}

func applyMigrations(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE articles ADD COLUMN url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscriptions ADD COLUMN fetch_window_start_min INTEGER NOT NULL DEFAULT -1`,
		`ALTER TABLE subscriptions ADD COLUMN fetch_window_end_min INTEGER NOT NULL DEFAULT -1`,
		`CREATE TABLE IF NOT EXISTS site_config (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "duplicate column") {
				continue
			}
			return fmt.Errorf("migrate %q: %w", s, err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

var ErrNotFound = errors.New("not found")

func wrapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
