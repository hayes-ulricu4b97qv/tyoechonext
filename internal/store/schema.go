package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func NewSQLiteRepository(path, legacyJSONPath string) (*SQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.importLegacy(legacyJSONPath); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.ensureOwnerUser(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.backfillSessionUserIDs(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sites (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			admin_mail TEXT NOT NULL,
			timezone TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admins (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'editor' CHECK (role IN ('owner', 'editor', 'moderator')),
			active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS users_role_active_idx ON users(role, active)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			excerpt TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
			published_at TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS posts_status_updated_idx ON posts(status, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS pages (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			excerpt TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
			published_at TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS pages_status_updated_idx ON pages(status, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS terms (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL COLLATE NOCASE,
			slug TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('category', 'tag')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(kind, name),
			UNIQUE(kind, slug)
		)`,
		`CREATE INDEX IF NOT EXISTS terms_kind_name_idx ON terms(kind, name)`,
		`CREATE TABLE IF NOT EXISTS post_terms (
			post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			term_id TEXT NOT NULL REFERENCES terms(id) ON DELETE CASCADE,
			PRIMARY KEY(post_id, term_id)
		)`,
		`CREATE INDEX IF NOT EXISTS post_terms_term_idx ON post_terms(term_id, post_id)`,
		`CREATE TABLE IF NOT EXISTS comments (
			id TEXT PRIMARY KEY,
			post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			author_name TEXT NOT NULL,
			author_email TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'spam')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS comments_post_status_created_idx ON comments(post_id, status, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS media (
			id TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			storage_key TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS media_created_idx ON media(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id TEXT PRIMARY KEY,
			actor_user_id TEXT NOT NULL DEFAULT '',
			actor_username TEXT NOT NULL,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS audit_events_created_idx ON audit_events(created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS audit_events_resource_idx ON audit_events(resource_type, action, created_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := r.db.Exec(statement); err != nil {
			return fmt.Errorf("run schema migration: %w", err)
		}
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record schema migration: %w", err)
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record comment and media migration: %w", err)
	}
	if err := r.ensureSessionUserColumn(); err != nil {
		return err
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (4, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record audit migration: %w", err)
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (5, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record taxonomy migration: %w", err)
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (6, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record pages migration: %w", err)
	}
	if err := r.ensureOwnerUser(); err != nil {
		return err
	}
	return nil
}

func (r *SQLiteRepository) ensureSessionUserColumn() error {
	rows, err := r.db.Query(`PRAGMA table_info(sessions)`)
	if err != nil {
		return fmt.Errorf("inspect sessions schema: %w", err)
	}
	hasUserID := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("read sessions schema: %w", err)
		}
		if name == "user_id" {
			hasUserID = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate sessions schema: %w", err)
	}
	rows.Close()
	if !hasUserID {
		if _, err := r.db.Exec(`ALTER TABLE sessions ADD COLUMN user_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add session user reference: %w", err)
		}
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id)`); err != nil {
		return fmt.Errorf("index session user reference: %w", err)
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (3, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record session user migration: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) backfillSessionUserIDs() error {
	if _, err := r.db.Exec(`UPDATE sessions SET user_id = (
		SELECT id FROM users WHERE users.username = sessions.username
	) WHERE user_id = '' AND EXISTS (
		SELECT 1 FROM users WHERE users.username = sessions.username
	)`); err != nil {
		return fmt.Errorf("backfill session user references: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ensureOwnerUser() error {
	var admin Admin
	var createdAt string
	err := r.db.QueryRow(`SELECT username, password_hash, email, created_at FROM admins WHERE id = 1`).Scan(&admin.Username, &admin.PasswordHash, &admin.Email, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy administrator for migration: %w", err)
	}
	created := parseTime(createdAt)
	if created.IsZero() {
		created = time.Now().UTC()
	}
	_, err = r.db.Exec(`INSERT OR IGNORE INTO users(id, username, password_hash, email, role, active, created_at, updated_at)
		VALUES ('user-owner', ?, ?, ?, 'owner', 1, ?, ?)`, admin.Username, admin.PasswordHash, admin.Email, formatTime(created), formatTime(created))
	if err != nil {
		return fmt.Errorf("migrate legacy administrator: %w", err)
	}
	return nil
}
func (r *SQLiteRepository) importLegacy(path string) error {
	if path == "" || r.Initialized() {
		return nil
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy repository: %w", err)
	}
	var legacy struct {
		Initialized bool   `json:"initialized"`
		Site        *Site  `json:"site"`
		Admin       *Admin `json:"admin"`
		Posts       []Post `json:"posts"`
	}
	if err := json.Unmarshal(content, &legacy); err != nil {
		return fmt.Errorf("decode legacy repository: %w", err)
	}
	if !legacy.Initialized || legacy.Site == nil || legacy.Admin == nil {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("start legacy migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO sites(id, name, url, admin_mail, timezone, created_at, updated_at) VALUES (1, ?, ?, ?, ?, ?, ?)`, legacy.Site.Name, legacy.Site.URL, legacy.Site.AdminMail, legacy.Site.Timezone, formatTime(legacy.Site.CreatedAt), formatTime(legacy.Site.UpdatedAt)); err != nil {
		return fmt.Errorf("migrate legacy site: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO admins(id, username, password_hash, email, created_at) VALUES (1, ?, ?, ?, ?)`, legacy.Admin.Username, legacy.Admin.PasswordHash, legacy.Admin.Email, formatTime(legacy.Admin.CreatedAt)); err != nil {
		return fmt.Errorf("migrate legacy admin: %w", err)
	}
	for _, post := range legacy.Posts {
		var publishedAt any
		if !post.PublishedAt.IsZero() {
			publishedAt = formatTime(post.PublishedAt)
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO posts(id, title, slug, excerpt, content, status, published_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, post.ID, post.Title, post.Slug, post.Excerpt, post.Content, post.Status, publishedAt, formatTime(post.UpdatedAt)); err != nil {
			return fmt.Errorf("migrate legacy post: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy migration: %w", err)
	}
	return nil
}
