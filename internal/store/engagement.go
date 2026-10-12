package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (r *SQLiteRepository) ListComments(query CommentQuery) (CommentPage, error) {
	query.Page, query.PageSize = normalizePage(query.Page, query.PageSize)
	where := "1 = 1"
	args := make([]any, 0, 3)
	if query.PostID != "" {
		where += " AND c.post_id = ?"
		args = append(args, query.PostID)
	}
	if query.Status != "" {
		where += " AND c.status = ?"
		args = append(args, query.Status)
	} else if !query.IncludePending {
		where += " AND c.status = 'approved'"
	}
	if query.PublishedPostsOnly {
		where += " AND p.status = 'published'"
	}
	from := "comments c JOIN posts p ON p.id = c.post_id"
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM `+from+` WHERE `+where, args...).Scan(&total); err != nil {
		return CommentPage{}, fmt.Errorf("count comments: %w", err)
	}
	offset := (query.Page - 1) * query.PageSize
	rows, err := r.db.Query(`SELECT c.id, c.post_id, p.title, p.slug, c.author_name, c.author_email, c.author_url, c.content, c.status, c.parent_id, c.created_at, c.updated_at
		FROM `+from+` WHERE `+where+` ORDER BY c.created_at DESC, c.id DESC LIMIT ? OFFSET ?`, append(args, query.PageSize, offset)...)
	if err != nil {
		return CommentPage{}, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()
	items := make([]Comment, 0, query.PageSize)
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return CommentPage{}, fmt.Errorf("scan comment: %w", err)
		}
		items = append(items, comment)
	}
	if err := rows.Err(); err != nil {
		return CommentPage{}, fmt.Errorf("iterate comments: %w", err)
	}
	return CommentPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func (r *SQLiteRepository) Comment(id string) (Comment, error) {
	row := r.db.QueryRow(`SELECT c.id, c.post_id, p.title, p.slug, c.author_name, c.author_email, c.author_url, c.content, c.status, c.parent_id, c.created_at, c.updated_at
		FROM comments c JOIN posts p ON p.id = c.post_id WHERE c.id = ?`, id)
	comment, err := scanComment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Comment{}, ErrNotFound
	}
	if err != nil {
		return Comment{}, fmt.Errorf("read comment: %w", err)
	}
	return comment, nil
}

func (r *SQLiteRepository) SaveComment(comment Comment) (Comment, error) {
	if comment.ID == "" {
		comment.ID = newCommentID()
	}
	if comment.Status == "" {
		comment.Status = "pending"
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now().UTC()
	}
	comment.UpdatedAt = time.Now().UTC()
	_, err := r.db.Exec(`INSERT INTO comments(id, post_id, parent_id, author_name, author_email, author_url, content, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET post_id = excluded.post_id, parent_id = excluded.parent_id, author_name = excluded.author_name, author_email = excluded.author_email,
		author_url = excluded.author_url, content = excluded.content, status = excluded.status, updated_at = excluded.updated_at`,
		comment.ID, comment.PostID, comment.ParentID, comment.AuthorName, comment.AuthorEmail, comment.AuthorURL, comment.Content, comment.Status, formatTime(comment.CreatedAt), formatTime(comment.UpdatedAt))
	if err != nil {
		return Comment{}, fmt.Errorf("save comment: %w", err)
	}
	return r.Comment(comment.ID)
}

func (r *SQLiteRepository) UpdateCommentStatus(id, status string) (Comment, error) {
	result, err := r.db.Exec(`UPDATE comments SET status = ?, updated_at = ? WHERE id = ?`, status, formatTime(time.Now().UTC()), id)
	if err != nil {
		return Comment{}, fmt.Errorf("update comment status: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Comment{}, ErrNotFound
	}
	return r.Comment(id)
}

func (r *SQLiteRepository) DeleteComment(id string) error {
	result, err := r.db.Exec(`DELETE FROM comments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) ListMedia(query MediaQuery) (MediaPage, error) {
	query.Page, query.PageSize = normalizePage(query.Page, query.PageSize)
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM media`).Scan(&total); err != nil {
		return MediaPage{}, fmt.Errorf("count media: %w", err)
	}
	offset := (query.Page - 1) * query.PageSize
	rows, err := r.db.Query(`SELECT id, filename, content_type, size, storage_key, created_at FROM media ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, query.PageSize, offset)
	if err != nil {
		return MediaPage{}, fmt.Errorf("list media: %w", err)
	}
	defer rows.Close()
	items := make([]Media, 0, query.PageSize)
	for rows.Next() {
		media, err := scanMedia(rows)
		if err != nil {
			return MediaPage{}, fmt.Errorf("scan media: %w", err)
		}
		items = append(items, media)
	}
	if err := rows.Err(); err != nil {
		return MediaPage{}, fmt.Errorf("iterate media: %w", err)
	}
	return MediaPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func (r *SQLiteRepository) Media(id string) (Media, error) {
	row := r.db.QueryRow(`SELECT id, filename, content_type, size, storage_key, created_at FROM media WHERE id = ?`, id)
	media, err := scanMedia(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Media{}, ErrNotFound
	}
	if err != nil {
		return Media{}, fmt.Errorf("read media: %w", err)
	}
	return media, nil
}

func (r *SQLiteRepository) SaveMedia(media Media) (Media, error) {
	if media.ID == "" {
		media.ID = newMediaID()
	}
	if media.CreatedAt.IsZero() {
		media.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.Exec(`INSERT INTO media(id, filename, content_type, size, storage_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET filename = excluded.filename, content_type = excluded.content_type, size = excluded.size, storage_key = excluded.storage_key`,
		media.ID, media.Filename, media.ContentType, media.Size, media.StorageKey, formatTime(media.CreatedAt))
	if err != nil {
		return Media{}, fmt.Errorf("save media: %w", err)
	}
	return r.Media(media.ID)
}

func (r *SQLiteRepository) DeleteMedia(id string) error {
	result, err := r.db.Exec(`DELETE FROM media WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete media: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}
