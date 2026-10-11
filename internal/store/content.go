package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (r *SQLiteRepository) Posts() ([]Post, error) {
	rows, err := r.db.Query(`SELECT id, title, slug, excerpt, content, status, published_at, updated_at FROM posts ORDER BY updated_at DESC, id DESC`)
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
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close posts: %w", err)
	}
	for index := range posts {
		if err := r.populatePostTerms(&posts[index]); err != nil {
			return nil, err
		}
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
	if query.TermKind != "" && query.TermSlug != "" {
		where += ` AND EXISTS (
			SELECT 1 FROM post_terms filter_pt JOIN terms filter_t ON filter_t.id = filter_pt.term_id
			WHERE filter_pt.post_id = posts.id AND filter_t.kind = ? AND filter_t.slug = ?
		)`
		args = append(args, query.TermKind, query.TermSlug)
	}
	if query.CategorySlug != "" {
		where += ` AND EXISTS (
			SELECT 1 FROM post_terms category_pt JOIN terms category_t ON category_t.id = category_pt.term_id
			WHERE category_pt.post_id = posts.id AND category_t.kind = 'category' AND category_t.slug = ?
		)`
		args = append(args, query.CategorySlug)
	}
	if query.TagSlug != "" {
		where += ` AND EXISTS (
			SELECT 1 FROM post_terms tag_pt JOIN terms tag_t ON tag_t.id = tag_pt.term_id
			WHERE tag_pt.post_id = posts.id AND tag_t.kind = 'tag' AND tag_t.slug = ?
		)`
		args = append(args, query.TagSlug)
	}
	if search != "" {
		where += " AND (title LIKE ? ESCAPE '\\' OR excerpt LIKE ? ESCAPE '\\' OR content LIKE ? ESCAPE '\\' OR slug LIKE ? ESCAPE '\\')"
		pattern := containsPattern(search)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM posts WHERE `+where, args...).Scan(&total); err != nil {
		return PostPage{}, fmt.Errorf("count posts: %w", err)
	}
	offset := (query.Page - 1) * query.PageSize
	rows, err := r.db.Query(`SELECT id, title, slug, excerpt, content, status, published_at, updated_at FROM posts WHERE `+where+` ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, query.PageSize, offset)...)
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
	if err := rows.Close(); err != nil {
		return PostPage{}, fmt.Errorf("close searched posts: %w", err)
	}
	for index := range items {
		if err := r.populatePostTerms(&items[index]); err != nil {
			return PostPage{}, err
		}
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
	if err := r.populatePostTerms(&post); err != nil {
		return Post{}, err
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
	if post.Status == "published" {
		if post.PublishedAt.IsZero() {
			post.PublishedAt = post.UpdatedAt
		}
	} else {
		post.PublishedAt = time.Time{}
	}
	var publishedAt any
	if !post.PublishedAt.IsZero() {
		publishedAt = formatTime(post.PublishedAt)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return Post{}, fmt.Errorf("start post save transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO posts(id, title, slug, excerpt, content, status, published_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title = excluded.title, slug = excluded.slug, excerpt = excluded.excerpt, content = excluded.content, status = excluded.status, published_at = excluded.published_at, updated_at = excluded.updated_at`,
		post.ID, post.Title, post.Slug, post.Excerpt, post.Content, post.Status, publishedAt, formatTime(post.UpdatedAt)); err != nil {
		return Post{}, fmt.Errorf("save post: %w", err)
	}
	if err := r.savePostTerms(tx, post.ID, post.Categories, post.Tags); err != nil {
		return Post{}, err
	}
	if err := tx.Commit(); err != nil {
		return Post{}, fmt.Errorf("commit post save: %w", err)
	}
	return r.Post(post.ID)
}

func (r *SQLiteRepository) populatePostTerms(post *Post) error {
	rows, err := r.db.Query(`SELECT t.id, t.name, t.slug, t.kind,
		(SELECT COUNT(DISTINCT p2.id) FROM post_terms pt2 JOIN posts p2 ON p2.id = pt2.post_id
		 WHERE pt2.term_id = t.id AND p2.status = 'published'), t.created_at, t.updated_at
		FROM post_terms pt JOIN terms t ON t.id = pt.term_id
		WHERE pt.post_id = ? ORDER BY CASE t.kind WHEN 'category' THEN 0 ELSE 1 END, t.name COLLATE NOCASE`, post.ID)
	if err != nil {
		return fmt.Errorf("list post terms: %w", err)
	}
	defer rows.Close()
	post.Categories = nil
	post.Tags = nil
	for rows.Next() {
		term, err := scanTerm(rows)
		if err != nil {
			return fmt.Errorf("scan post term: %w", err)
		}
		if term.Kind == "category" {
			post.Categories = append(post.Categories, term)
		} else {
			post.Tags = append(post.Tags, term)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate post terms: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) savePostTerms(tx *sql.Tx, postID string, categories, tags []Term) error {
	if _, err := tx.Exec(`DELETE FROM post_terms WHERE post_id = ?`, postID); err != nil {
		return fmt.Errorf("clear post terms: %w", err)
	}
	for _, input := range append(append([]Term{}, categories...), tags...) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			continue
		}
		kind := input.Kind
		if kind != "category" && kind != "tag" {
			return fmt.Errorf("invalid post term kind %q", kind)
		}
		termID, err := upsertTermTx(tx, kind, name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO post_terms(post_id, term_id) VALUES (?, ?)`, postID, termID); err != nil {
			return fmt.Errorf("link post term: %w", err)
		}
	}
	return nil
}

func upsertTermTx(tx *sql.Tx, kind, name string) (string, error) {
	var existingID string
	err := tx.QueryRow(`SELECT id FROM terms WHERE kind = ? AND name = ?`, kind, name).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read post term: %w", err)
	}
	id := newTermID()
	baseSlug := termSlug(name)
	now := time.Now().UTC()
	for suffix := 1; suffix < 10000; suffix++ {
		slug := baseSlug
		if suffix > 1 {
			slug = fmt.Sprintf("%s-%d", baseSlug, suffix)
		}
		_, err := tx.Exec(`INSERT INTO terms(id, name, slug, kind, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, id, name, slug, kind, formatTime(now), formatTime(now))
		if err == nil {
			return id, nil
		}
		if !strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			return "", fmt.Errorf("create post term: %w", err)
		}
	}
	return "", fmt.Errorf("unable to allocate a unique term slug")
}

func (r *SQLiteRepository) SaveTerm(term Term) (Term, error) {
	term.Name = strings.TrimSpace(term.Name)
	term.Kind = strings.TrimSpace(term.Kind)
	if term.ID == "" {
		term.ID = newTermID()
	}
	if term.Slug == "" {
		term.Slug = termSlug(term.Name)
	}
	if term.Kind != "category" && term.Kind != "tag" {
		return Term{}, fmt.Errorf("invalid term kind %q", term.Kind)
	}
	now := time.Now().UTC()
	if term.CreatedAt.IsZero() {
		term.CreatedAt = now
	}
	term.UpdatedAt = now
	_, err := r.db.Exec(`INSERT INTO terms(id, name, slug, kind, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, slug = excluded.slug, kind = excluded.kind, updated_at = excluded.updated_at`,
		term.ID, term.Name, term.Slug, term.Kind, formatTime(term.CreatedAt), formatTime(term.UpdatedAt))
	if err != nil {
		return Term{}, fmt.Errorf("save term: %w", err)
	}
	return r.termByID(term.ID)
}

func (r *SQLiteRepository) termByID(id string) (Term, error) {
	row := r.db.QueryRow(`SELECT t.id, t.name, t.slug, t.kind,
		COUNT(DISTINCT CASE WHEN p.status = 'published' THEN p.id END), t.created_at, t.updated_at
		FROM terms t LEFT JOIN post_terms pt ON pt.term_id = t.id LEFT JOIN posts p ON p.id = pt.post_id
		WHERE t.id = ? GROUP BY t.id, t.name, t.slug, t.kind, t.created_at, t.updated_at`, id)
	term, err := scanTerm(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Term{}, ErrNotFound
	}
	if err != nil {
		return Term{}, fmt.Errorf("read term: %w", err)
	}
	return term, nil
}

func (r *SQLiteRepository) DeleteTerm(id string) error {
	result, err := r.db.Exec(`DELETE FROM terms WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete term: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) ListTerms(query TermQuery) (TermPage, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}
	where := "1 = 1"
	if !query.IncludeDrafts {
		where = `EXISTS (
			SELECT 1 FROM post_terms visible_pt JOIN posts visible_p ON visible_p.id = visible_pt.post_id
			WHERE visible_pt.term_id = t.id AND visible_p.status = 'published'
		)`
	}
	args := make([]any, 0, 2)
	if strings.TrimSpace(query.Kind) != "" {
		where += " AND t.kind = ?"
		args = append(args, strings.TrimSpace(query.Kind))
	}
	if strings.TrimSpace(query.Search) != "" {
		where += " AND t.name LIKE ? ESCAPE '\\'"
		args = append(args, containsPattern(query.Search))
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM terms t WHERE `+where, args...).Scan(&total); err != nil {
		return TermPage{}, fmt.Errorf("count terms: %w", err)
	}
	offset := (query.Page - 1) * query.PageSize
	rows, err := r.db.Query(`SELECT t.id, t.name, t.slug, t.kind,
		COUNT(DISTINCT CASE WHEN p.status = 'published' THEN p.id END), t.created_at, t.updated_at
		FROM terms t LEFT JOIN post_terms pt ON pt.term_id = t.id LEFT JOIN posts p ON p.id = pt.post_id
		WHERE `+where+` GROUP BY t.id, t.name, t.slug, t.kind, t.created_at, t.updated_at
		ORDER BY t.kind, t.name COLLATE NOCASE LIMIT ? OFFSET ?`, append(args, query.PageSize, offset)...)
	if err != nil {
		return TermPage{}, fmt.Errorf("list terms: %w", err)
	}
	defer rows.Close()
	items := make([]Term, 0, query.PageSize)
	for rows.Next() {
		term, err := scanTerm(rows)
		if err != nil {
			return TermPage{}, fmt.Errorf("scan term: %w", err)
		}
		items = append(items, term)
	}
	if err := rows.Err(); err != nil {
		return TermPage{}, fmt.Errorf("iterate terms: %w", err)
	}
	return TermPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
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

func (r *SQLiteRepository) SearchPages(query PageQuery) (PagePage, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}
	where := "1 = 1"
	args := make([]any, 0, 4)
	if !query.IncludeDrafts {
		where += " AND status = 'published'"
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		where += " AND (title LIKE ? ESCAPE '\\' OR excerpt LIKE ? ESCAPE '\\' OR content LIKE ? ESCAPE '\\' OR slug LIKE ? ESCAPE '\\')"
		pattern := containsPattern(search)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM pages WHERE `+where, args...).Scan(&total); err != nil {
		return PagePage{}, fmt.Errorf("count pages: %w", err)
	}
	offset := (query.Page - 1) * query.PageSize
	rows, err := r.db.Query(`SELECT id, title, slug, excerpt, content, status, published_at, updated_at
		FROM pages WHERE `+where+` ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, query.PageSize, offset)...)
	if err != nil {
		return PagePage{}, fmt.Errorf("search pages: %w", err)
	}
	defer rows.Close()
	items := make([]Page, 0, query.PageSize)
	for rows.Next() {
		page, err := scanPage(rows)
		if err != nil {
			return PagePage{}, fmt.Errorf("scan page: %w", err)
		}
		items = append(items, page)
	}
	if err := rows.Err(); err != nil {
		return PagePage{}, fmt.Errorf("iterate pages: %w", err)
	}
	return PagePage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func (r *SQLiteRepository) Page(id string) (Page, error) {
	return r.pageBy(`id = ?`, id)
}

func (r *SQLiteRepository) PageBySlug(slug string) (Page, error) {
	return r.pageBy(`slug = ?`, slug)
}

func (r *SQLiteRepository) pageBy(where, value string) (Page, error) {
	row := r.db.QueryRow(`SELECT id, title, slug, excerpt, content, status, published_at, updated_at FROM pages WHERE `+where, value)
	page, err := scanPage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("read page: %w", err)
	}
	return page, nil
}

func (r *SQLiteRepository) SavePage(page Page) (Page, error) {
	if page.ID == "" {
		page.ID = newPageID()
	}
	if page.Status == "" {
		page.Status = "draft"
	}
	page.UpdatedAt = time.Now().UTC()
	if page.Status == "published" {
		if page.PublishedAt.IsZero() {
			page.PublishedAt = page.UpdatedAt
		}
	} else {
		page.PublishedAt = time.Time{}
	}
	var publishedAt any
	if !page.PublishedAt.IsZero() {
		publishedAt = formatTime(page.PublishedAt)
	}
	if _, err := r.db.Exec(`INSERT INTO pages(id, title, slug, excerpt, content, status, published_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title = excluded.title, slug = excluded.slug, excerpt = excluded.excerpt,
		content = excluded.content, status = excluded.status, published_at = excluded.published_at, updated_at = excluded.updated_at`,
		page.ID, page.Title, page.Slug, page.Excerpt, page.Content, page.Status, publishedAt, formatTime(page.UpdatedAt)); err != nil {
		return Page{}, fmt.Errorf("save page: %w", err)
	}
	return r.Page(page.ID)
}

func (r *SQLiteRepository) DeletePage(id string) error {
	result, err := r.db.Exec(`DELETE FROM pages WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete page: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}
