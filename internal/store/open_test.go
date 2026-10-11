package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestSingletonAdminIsNotPromoted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	if err := os.WriteFile(filepath.Join(dir, "app.json"), []byte(`{"initialized":true,"site":{"name":"From JSON","url":"https://json.example","adminMail":"json@example.com","timezone":"UTC"},"admin":{"username":"from-json","passwordHash":"hash","email":"json@example.com"},"posts":[{"id":"json-post","title":"JSON","slug":"json-post","content":"body","status":"published"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
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
		`INSERT INTO sites(id, name, url, admin_mail, timezone, created_at, updated_at) VALUES (1, 'Only Admin', 'https://example.com', 'legacy@example.com', 'UTC', '` + now + `', '` + now + `')`,
		`CREATE TABLE admins (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO admins(id, username, password_hash, email, created_at) VALUES (1, 'legacy-admin', 'hash', 'legacy@example.com', '` + now + `')`,
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
	if repo.Initialized() {
		t.Fatal("a singleton admin row was promoted to an owner")
	}
	if _, err := repo.User("legacy-admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("singleton admin was copied: %v", err)
	}
	if _, err := repo.User("from-json"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy file user was imported: %v", err)
	}
	users, err := repo.Users()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 0 {
		t.Fatalf("users = %#v", users)
	}
	site, err := repo.Site()
	if err != nil {
		t.Fatal(err)
	}
	if site.Name != "Only Admin" {
		t.Fatalf("site = %+v", site)
	}
	posts, err := repo.Posts()
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Fatalf("posts = %#v", posts)
	}
}
