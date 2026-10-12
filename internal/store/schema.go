package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
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
		`CREATE TABLE IF NOT EXISTS search_tokens (
			kind TEXT NOT NULL,
			token TEXT NOT NULL,
			record_id TEXT NOT NULL,
			PRIMARY KEY (kind, token, record_id)
		)`,
		`CREATE TABLE IF NOT EXISTS post_revisions (
			id TEXT PRIMARY KEY,
			post_id TEXT NOT NULL,
			title TEXT NOT NULL,
			excerpt TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS post_revisions_post_idx ON post_revisions(post_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS outbound_mail (
			id TEXT PRIMARY KEY,
			recipient TEXT NOT NULL,
			subject TEXT NOT NULL,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS password_resets (
			token_hash TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
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
	if err := r.ensureReadingColumns(); err != nil {
		return err
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (7, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record reading migration: %w", err)
	}
	if err := r.ensureSiteProfileColumns(); err != nil {
		return err
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (8, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record site profile migration: %w", err)
	}
	if err := r.ensurePassphraseColumn(); err != nil {
		return err
	}
	return nil
}

func (r *SQLiteRepository) ensurePassphraseColumn() error {
	names, err := r.columnNames("posts")
	if err != nil {
		return err
	}
	if names["passphrase_hash"] {
		return nil
	}
	if _, err := r.db.Exec(`ALTER TABLE posts ADD COLUMN passphrase_hash TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add posts.passphrase_hash: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ensureReadingColumns() error {
	additions := []struct {
		table  string
		column string
		ddl    string
	}{
		{"sites", "posts_per_page", `ALTER TABLE sites ADD COLUMN posts_per_page INTEGER NOT NULL DEFAULT 10`},
		{"sites", "comments_enabled", `ALTER TABLE sites ADD COLUMN comments_enabled INTEGER NOT NULL DEFAULT 1`},
		{"sites", "comments_require_moderation", `ALTER TABLE sites ADD COLUMN comments_require_moderation INTEGER NOT NULL DEFAULT 1`},
		{"sites", "comments_threaded", `ALTER TABLE sites ADD COLUMN comments_threaded INTEGER NOT NULL DEFAULT 1`},
		{"posts", "allow_comment", `ALTER TABLE posts ADD COLUMN allow_comment INTEGER NOT NULL DEFAULT 1`},
		{"comments", "parent_id", `ALTER TABLE comments ADD COLUMN parent_id TEXT NOT NULL DEFAULT ''`},
		{"comments", "author_url", `ALTER TABLE comments ADD COLUMN author_url TEXT NOT NULL DEFAULT ''`},
	}
	for _, addition := range additions {
		names, err := r.columnNames(addition.table)
		if err != nil {
			return err
		}
		if names[addition.column] {
			continue
		}
		if _, err := r.db.Exec(addition.ddl); err != nil {
			return fmt.Errorf("add %s.%s: %w", addition.table, addition.column, err)
		}
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS comments_parent_idx ON comments(parent_id, post_id)`); err != nil {
		return fmt.Errorf("index comment parents: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ensureSiteProfileColumns() error {
	additions := []struct {
		table  string
		column string
		ddl    string
	}{
		{"sites", "description", `ALTER TABLE sites ADD COLUMN description TEXT NOT NULL DEFAULT ''`},
		{"sites", "keywords", `ALTER TABLE sites ADD COLUMN keywords TEXT NOT NULL DEFAULT ''`},
	}
	for _, addition := range additions {
		names, err := r.columnNames(addition.table)
		if err != nil {
			return err
		}
		if names[addition.column] {
			continue
		}
		if _, err := r.db.Exec(addition.ddl); err != nil {
			return fmt.Errorf("add %s.%s: %w", addition.table, addition.column, err)
		}
	}
	return nil
}

func (r *SQLiteRepository) columnNames(table string) (map[string]bool, error) {
	var query string
	switch table {
	case "sites":
		query = `PRAGMA table_info(sites)`
	case "posts":
		query = `PRAGMA table_info(posts)`
	case "comments":
		query = `PRAGMA table_info(comments)`
	default:
		return nil, fmt.Errorf("unsupported table %s", table)
	}
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("read %s schema: %w", table, err)
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s schema: %w", table, err)
	}
	return names, nil
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
