package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bitop-dev/agent/pkg/session"
)

type Store struct {
	Path string
}

func (s Store) Create(ctx context.Context, meta session.Metadata) (session.Session, error) {
	db, err := s.open(ctx)
	if err != nil {
		return session.Session{}, err
	}
	defer db.Close()
	if meta.ID == "" {
		meta.ID = session.NewID(time.Now())
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = meta.CreatedAt
	}
	_, err = db.ExecContext(ctx, `
		INSERT OR REPLACE INTO sessions (id, profile, cwd, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, meta.ID, meta.Profile, meta.CWD, meta.CreatedAt.UTC(), meta.UpdatedAt.UTC())
	if err != nil {
		return session.Session{}, err
	}
	return session.Session{Metadata: meta}, nil
}

func (s Store) Load(ctx context.Context, id string) (session.Session, error) {
	db, err := s.open(ctx)
	if err != nil {
		return session.Session{}, err
	}
	defer db.Close()
	meta, err := loadMetadata(ctx, db, `SELECT id, profile, cwd, created_at, updated_at FROM sessions WHERE id = ?`, id)
	if err != nil {
		return session.Session{}, err
	}
	entries, err := loadEntries(ctx, db, id)
	if err != nil {
		return session.Session{}, err
	}
	return session.Session{Metadata: meta, Entries: entries}, nil
}

func (s Store) Append(ctx context.Context, id string, entry session.Entry) error {
	db, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO entries (session_id, kind, role, content, event_type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, entry.Kind, entry.Role, entry.Content, entry.EventType, entry.Metadata, entry.CreatedAt.UTC())
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE sessions SET updated_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}

func (s Store) List(ctx context.Context, cwd string, limit int) ([]session.Metadata, error) {
	db, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id, profile, cwd, created_at, updated_at FROM sessions`
	args := []any{}
	if cwd != "" {
		query += ` WHERE cwd = ?`
		args = append(args, cwd)
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metas []session.Metadata
	for rows.Next() {
		var meta session.Metadata
		if err := rows.Scan(&meta.ID, &meta.Profile, &meta.CWD, &meta.CreatedAt, &meta.UpdatedAt); err != nil {
			return nil, err
		}
		metas = append(metas, meta)
	}
	return metas, rows.Err()
}

func (s Store) Count(ctx context.Context, cwd string) (int, error) {
	db, err := s.open(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	query := `SELECT COUNT(*) FROM sessions`
	args := []any{}
	if cwd != "" {
		query += ` WHERE cwd = ?`
		args = append(args, cwd)
	}
	var count int
	err = db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s Store) MostRecent(ctx context.Context, cwd string) (session.Session, error) {
	db, err := s.open(ctx)
	if err != nil {
		return session.Session{}, err
	}
	defer db.Close()
	query := `SELECT id, profile, cwd, created_at, updated_at FROM sessions`
	args := []any{}
	if cwd != "" {
		query += ` WHERE cwd = ?`
		args = append(args, cwd)
	}
	query += ` ORDER BY updated_at DESC LIMIT 1`
	meta, err := loadMetadata(ctx, db, query, args...)
	if err != nil {
		return session.Session{}, err
	}
	entries, err := loadEntries(ctx, db, meta.ID)
	if err != nil {
		return session.Session{}, err
	}
	return session.Session{Metadata: meta, Entries: entries}, nil
}

func (s Store) open(ctx context.Context) (*sql.DB, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", s.Path)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			profile TEXT NOT NULL,
			cwd TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE IF NOT EXISTS entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			event_type TEXT NOT NULL,
			metadata TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id)
		);
		CREATE INDEX IF NOT EXISTS idx_entries_session_id_created_at
		ON entries(session_id, created_at);
	`)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE entries ADD COLUMN metadata TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	return nil
}

func loadMetadata(ctx context.Context, db *sql.DB, query string, args ...any) (session.Metadata, error) {
	var meta session.Metadata
	err := db.QueryRowContext(ctx, query, args...).Scan(&meta.ID, &meta.Profile, &meta.CWD, &meta.CreatedAt, &meta.UpdatedAt)
	if err != nil {
		return session.Metadata{}, err
	}
	return meta, nil
}

func loadEntries(ctx context.Context, db *sql.DB, sessionID string) ([]session.Entry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT kind, role, content, event_type, metadata, created_at
		FROM entries
		WHERE session_id = ?
		ORDER BY created_at ASC, id ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []session.Entry
	for rows.Next() {
		var entry session.Entry
		if err := rows.Scan(&entry.Kind, &entry.Role, &entry.Content, &entry.EventType, &entry.Metadata, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
