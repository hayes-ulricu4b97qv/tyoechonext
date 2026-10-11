package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestExistingDatabaseGainsReadingDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	statements := []string{
		`CREATE TABLE sites (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			admin_mail TEXT NOT NULL,
			timezone TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO sites(id, name, url, admin_mail, timezone, created_at, updated_at) VALUES (1, 'Old Site', 'https://example.com', 'owner@example.com', 'UTC', '` + now + `', '` + now + `')`,
		`CREATE TABLE posts (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			excerpt TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			published_at TEXT,
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO posts(id, title, slug, content, status, updated_at) VALUES ('post-old', 'Old', 'old', 'body', 'published', '` + now + `')`,
		`CREATE TABLE comments (
			id TEXT PRIMARY KEY,
			post_id TEXT NOT NULL,
			author_name TEXT NOT NULL,
			author_email TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	repo, err := NewSQLiteRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	site, err := repo.Site()
	if err != nil {
		t.Fatal(err)
	}
	if site.Name != "Old Site" || site.PostsPerPage != 10 || !site.CommentsEnabled || !site.CommentsRequireModeration || !site.CommentsThreaded {
		t.Fatalf("reading defaults were not applied: %+v", site)
	}
	if site.Description != "" || site.Keywords != "" {
		t.Fatalf("site profile defaults were not applied: %+v", site)
	}
	post, err := repo.Post("post-old")
	if err != nil {
		t.Fatal(err)
	}
	if !post.AllowComment {
		t.Fatal("existing post should allow comments after migration")
	}
}
