package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var ErrNotFound = errors.New("record not found")

type Site struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	AdminMail string    `json:"adminMail"`
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Admin struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"passwordHash"`
	Email        string    `json:"email"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Post struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Excerpt     string    `json:"excerpt"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	PublishedAt time.Time `json:"publishedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type PostStats struct {
	Posts          int `json:"posts"`
	PublishedPosts int `json:"publishedPosts"`
	Drafts         int `json:"drafts"`
}

type PostQuery struct {
	Search        string
	IncludeDrafts bool
	Page          int
	PageSize      int
}

type PostPage struct {
	Items    []Post `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type Repository interface {
	Initialized() bool
	Site() (*Site, error)
	Admin() (*Admin, error)
	Setup(Site, Admin) error
	UpdateSite(Site) error
	Posts() ([]Post, error)
	SearchPosts(PostQuery) (PostPage, error)
	Post(string) (Post, error)
	PostBySlug(string) (Post, error)
	SavePost(Post) (Post, error)
	DeletePost(string) error
	Stats() (PostStats, error)
	CreateSession(string, string, time.Time) error
	ResolveSession(string, time.Time) (string, error)
	DeleteSession(string) error
	PurgeSessions(time.Time) error
	Close() error
}

type SQLiteRepository struct {
	db *sql.DB
}

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
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions(expires_at)`,
	}
	for _, statement := range statements {
		if _, err := r.db.Exec(statement); err != nil {
			return fmt.Errorf("run schema migration: %w", err)
		}
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record schema migration: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) Initialized() bool {
	var initialized bool
	err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sites WHERE id = 1) AND EXISTS(SELECT 1 FROM admins WHERE id = 1)`).Scan(&initialized)
	return err == nil && initialized
}

func (r *SQLiteRepository) Site() (*Site, error) {
	var site Site
	var createdAt, updatedAt string
	err := r.db.QueryRow(`SELECT name, url, admin_mail, timezone, created_at, updated_at FROM sites WHERE id = 1`).Scan(&site.Name, &site.URL, &site.AdminMail, &site.Timezone, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read site: %w", err)
	}
	site.CreatedAt = parseTime(createdAt)
	site.UpdatedAt = parseTime(updatedAt)
	return &site, nil
}

func (r *SQLiteRepository) Admin() (*Admin, error) {
	var admin Admin
	var createdAt string
	err := r.db.QueryRow(`SELECT username, password_hash, email, created_at FROM admins WHERE id = 1`).Scan(&admin.Username, &admin.PasswordHash, &admin.Email, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read admin: %w", err)
	}
	admin.CreatedAt = parseTime(createdAt)
	return &admin, nil
}

func (r *SQLiteRepository) Setup(site Site, admin Admin) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("start setup transaction: %w", err)
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sites`).Scan(&count); err != nil {
		return fmt.Errorf("check setup state: %w", err)
	}
	if count > 0 {
		return errors.New("application is already initialized")
	}
	now := time.Now().UTC()
	site.CreatedAt = now
	site.UpdatedAt = now
	admin.CreatedAt = now
	if _, err := tx.Exec(`INSERT INTO sites(id, name, url, admin_mail, timezone, created_at, updated_at) VALUES (1, ?, ?, ?, ?, ?, ?)`, site.Name, site.URL, site.AdminMail, site.Timezone, formatTime(site.CreatedAt), formatTime(site.UpdatedAt)); err != nil {
		return fmt.Errorf("save site: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO admins(id, username, password_hash, email, created_at) VALUES (1, ?, ?, ?, ?)`, admin.Username, admin.PasswordHash, admin.Email, formatTime(admin.CreatedAt)); err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit setup transaction: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) UpdateSite(site Site) error {
	current, err := r.Site()
	if err != nil {
		return err
	}
	site.CreatedAt = current.CreatedAt
	site.UpdatedAt = time.Now().UTC()
	result, err := r.db.Exec(`UPDATE sites SET name = ?, url = ?, admin_mail = ?, timezone = ?, updated_at = ? WHERE id = 1`, site.Name, site.URL, site.AdminMail, site.Timezone, formatTime(site.UpdatedAt))
	if err != nil {
		return fmt.Errorf("update site: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) Posts() ([]Post, error) {
	rows, err := r.db.Query(`SELECT id, title, slug, excerpt, content, status, published_at, updated_at FROM posts ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()
	posts := make([]Post, 0)
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posts: %w", err)
	}
	return posts, nil
}

func (r *SQLiteRepository) SearchPosts(query PostQuery) (PostPage, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}
	search := strings.TrimSpace(query.Search)
	where := "1 = 1"
	args := make([]any, 0, 5)
	if !query.IncludeDrafts {
		where += " AND status = 'published'"
	}
	if search != "" {
		where += " AND (title LIKE ? OR excerpt LIKE ? OR content LIKE ? OR slug LIKE ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern, pattern)
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM posts WHERE `+where, args...).Scan(&total); err != nil {
		return PostPage{}, fmt.Errorf("count posts: %w", err)
	}
	offset := (query.Page - 1) * query.PageSize
	rows, err := r.db.Query(`SELECT id, title, slug, excerpt, content, status, published_at, updated_at FROM posts WHERE `+where+` ORDER BY updated_at DESC LIMIT ? OFFSET ?`, append(args, query.PageSize, offset)...)
	if err != nil {
		return PostPage{}, fmt.Errorf("search posts: %w", err)
	}
	defer rows.Close()
	items := make([]Post, 0, query.PageSize)
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			return PostPage{}, fmt.Errorf("scan searched post: %w", err)
		}
		items = append(items, post)
	}
	if err := rows.Err(); err != nil {
		return PostPage{}, fmt.Errorf("iterate searched posts: %w", err)
	}
	return PostPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func (r *SQLiteRepository) Post(id string) (Post, error) {
	return r.postBy(`id = ?`, id)
}

func (r *SQLiteRepository) PostBySlug(slug string) (Post, error) {
	return r.postBy(`slug = ?`, slug)
}

func (r *SQLiteRepository) postBy(where, value string) (Post, error) {
	row := r.db.QueryRow(`SELECT id, title, slug, excerpt, content, status, published_at, updated_at FROM posts WHERE `+where, value)
	post, err := scanPost(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	if err != nil {
		return Post{}, fmt.Errorf("read post: %w", err)
	}
	return post, nil
}

func (r *SQLiteRepository) SavePost(post Post) (Post, error) {
	if post.ID == "" {
		post.ID = newID()
	}
	if post.Status == "" {
		post.Status = "draft"
	}
	post.UpdatedAt = time.Now().UTC()
	if post.Status == "published" && post.PublishedAt.IsZero() {
		post.PublishedAt = post.UpdatedAt
	}
	var publishedAt any
	if !post.PublishedAt.IsZero() {
		publishedAt = formatTime(post.PublishedAt)
	}
	_, err := r.db.Exec(`INSERT INTO posts(id, title, slug, excerpt, content, status, published_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title = excluded.title, slug = excluded.slug, excerpt = excluded.excerpt, content = excluded.content, status = excluded.status, published_at = excluded.published_at, updated_at = excluded.updated_at`,
		post.ID, post.Title, post.Slug, post.Excerpt, post.Content, post.Status, publishedAt, formatTime(post.UpdatedAt))
	if err != nil {
		return Post{}, fmt.Errorf("save post: %w", err)
	}
	return r.Post(post.ID)
}

func (r *SQLiteRepository) DeletePost(id string) error {
	result, err := r.db.Exec(`DELETE FROM posts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) Stats() (PostStats, error) {
	var stats PostStats
	err := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'published' THEN 1 ELSE 0 END), 0), COALESCE(SUM(CASE WHEN status = 'draft' THEN 1 ELSE 0 END), 0) FROM posts`).Scan(&stats.Posts, &stats.PublishedPosts, &stats.Drafts)
	if err != nil {
		return PostStats{}, fmt.Errorf("read post stats: %w", err)
	}
	return stats, nil
}

func (r *SQLiteRepository) CreateSession(username, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(`INSERT INTO sessions(token_hash, username, expires_at, created_at) VALUES (?, ?, ?, ?)`, tokenHash, username, formatTime(expiresAt), formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ResolveSession(tokenHash string, now time.Time) (string, error) {
	var username, expiresAt string
	err := r.db.QueryRow(`SELECT username, expires_at FROM sessions WHERE token_hash = ?`, tokenHash).Scan(&username, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read session: %w", err)
	}
	if !parseTime(expiresAt).After(now) {
		_ = r.DeleteSession(tokenHash)
		return "", ErrNotFound
	}
	return username, nil
}

func (r *SQLiteRepository) DeleteSession(tokenHash string) error {
	if _, err := r.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) PurgeSessions(now time.Time) error {
	if _, err := r.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, formatTime(now)); err != nil {
		return fmt.Errorf("purge sessions: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
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

type scanner interface {
	Scan(...any) error
}

func scanPost(row scanner) (Post, error) {
	var post Post
	var publishedAt, updatedAt sql.NullString
	err := row.Scan(&post.ID, &post.Title, &post.Slug, &post.Excerpt, &post.Content, &post.Status, &publishedAt, &updatedAt)
	if err != nil {
		return Post{}, err
	}
	if publishedAt.Valid {
		post.PublishedAt = parseTime(publishedAt.String)
	}
	if updatedAt.Valid {
		post.UpdatedAt = parseTime(updatedAt.String)
	}
	return post, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func newID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("post-%d", time.Now().UnixNano())
	}
	return "post-" + hex.EncodeToString(buffer)
}
