package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreshDatabaseUsesCompleteVersionedSchema(t *testing.T) {
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	var version int
	if err := repo.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, currentSchemaVersion)
	}
	var publicationIndex int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'posts_status_published_idx'`).Scan(&publicationIndex); err != nil {
		t.Fatal(err)
	}
	if publicationIndex != 1 {
		t.Fatal("fresh schema lacks posts_status_published_idx")
	}

	for table, required := range map[string][]string{
		"sites":          {"description", "keywords", "posts_per_page", "comments_enabled", "comments_require_moderation", "comments_threaded"},
		"posts":          {"allow_comment", "passphrase_hash", "custom_fields"},
		"comments":       {"parent_id", "author_url"},
		"sessions":       {"user_id"},
		"post_revisions": {"custom_fields"},
	} {
		columns := tableColumns(t, repo, table)
		for _, column := range required {
			if !columns[column] {
				t.Errorf("fresh %s schema lacks %s", table, column)
			}
		}
	}
}

func TestOldDatabaseIsRejectedWithoutChangingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sites (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sites(id, name) VALUES (1, 'old site')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if repo, err := NewSQLiteRepository(path); err == nil {
		_ = repo.Close()
		t.Fatal("opened an unversioned database")
	} else if !strings.Contains(err.Error(), "delete app.db") {
		t.Fatalf("old database error = %v", err)
	}

	db, err = sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.QueryRow(`SELECT name FROM sites WHERE id = 1`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "old site" {
		t.Fatalf("old database was modified: site name = %q", name)
	}
	var tableCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT GLOB 'sqlite_*'`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 1 {
		t.Fatalf("old database gained schema objects: %d tables", tableCount)
	}
}

func TestPreviousSchemaVersionIsRejectedWithoutChangingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE preserved (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO preserved(value) VALUES ('keep')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if repo, err := NewSQLiteRepository(path); err == nil {
		_ = repo.Close()
		t.Fatal("opened a database with the previous schema version")
	} else if !strings.Contains(err.Error(), "expected version 2") {
		t.Fatalf("previous schema version error = %v", err)
	}

	db, err = sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM preserved`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if version != 1 || value != "keep" {
		t.Fatalf("rejected database changed: version=%d value=%q", version, value)
	}
}

func tableColumns(t *testing.T, repo *SQLiteRepository, table string) map[string]bool {
	t.Helper()
	rows, err := repo.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}
