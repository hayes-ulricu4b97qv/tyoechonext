package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (r *SQLiteRepository) snapshotPost(tx *sql.Tx, post Post) error {
	var title, excerpt, content, rawCustomFields string
	err := tx.QueryRow(`SELECT title, excerpt, content, custom_fields FROM posts WHERE id = ?`, post.ID).Scan(&title, &excerpt, &content, &rawCustomFields)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read post revision source: %w", err)
	}
	currentCustomFields, err := decodeCustomFields(rawCustomFields)
	if err != nil {
		return err
	}
	if title == post.Title && excerpt == post.Excerpt && content == post.Content && equalCustomFields(currentCustomFields, post.CustomFields) {
		return nil
	}
	customFields, err := encodeCustomFields(currentCustomFields)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO post_revisions(id, post_id, title, excerpt, content, custom_fields, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, newID(), post.ID, title, excerpt, content, customFields, formatTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("save post revision: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ListPostRevisions(postID string) ([]PostRevision, error) {
	rows, err := r.db.Query(`SELECT id, post_id, title, excerpt, content, custom_fields, created_at FROM post_revisions WHERE post_id = ? ORDER BY created_at DESC, id DESC`, postID)
	if err != nil {
		return nil, fmt.Errorf("list post revisions: %w", err)
	}
	defer rows.Close()
	items := make([]PostRevision, 0)
	for rows.Next() {
		var item PostRevision
		var createdAt, rawCustomFields string
		if err := rows.Scan(&item.ID, &item.PostID, &item.Title, &item.Excerpt, &item.Content, &rawCustomFields, &createdAt); err != nil {
			return nil, fmt.Errorf("scan post revision: %w", err)
		}
		item.CustomFields, err = decodeCustomFields(rawCustomFields)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(createdAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *SQLiteRepository) PostRevision(id string) (PostRevision, error) {
	var item PostRevision
	var createdAt, rawCustomFields string
	err := r.db.QueryRow(`SELECT id, post_id, title, excerpt, content, custom_fields, created_at FROM post_revisions WHERE id = ?`, id).Scan(&item.ID, &item.PostID, &item.Title, &item.Excerpt, &item.Content, &rawCustomFields, &createdAt)
	if err == sql.ErrNoRows {
		return PostRevision{}, ErrNotFound
	}
	if err != nil {
		return PostRevision{}, fmt.Errorf("read post revision: %w", err)
	}
	item.CustomFields, err = decodeCustomFields(rawCustomFields)
	if err != nil {
		return PostRevision{}, err
	}
	item.CreatedAt = parseTime(createdAt)
	return item, nil
}

func (r *SQLiteRepository) SaveOutboundMail(recipient, subject, body string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin outbound mail save: %w", err)
	}
	defer tx.Rollback()
	createdAt := time.Now().UTC()
	if _, err := tx.Exec(`INSERT INTO outbound_mail(id, recipient, subject, body, created_at) VALUES (?, ?, ?, ?, ?)`, newID(), recipient, subject, body, formatTime(createdAt)); err != nil {
		return fmt.Errorf("save outbound mail: %w", err)
	}
	cutoff := formatTime(createdAt.Add(-90 * 24 * time.Hour))
	if _, err := tx.Exec(`DELETE FROM outbound_mail WHERE created_at < ?`, cutoff); err != nil {
		return fmt.Errorf("expire outbound mail: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM outbound_mail WHERE id IN (
		SELECT id FROM outbound_mail ORDER BY created_at DESC, id DESC LIMIT -1 OFFSET 1000
	)`); err != nil {
		return fmt.Errorf("bound outbound mail history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit outbound mail save: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ListOutboundMail(page, pageSize int) (OutboundMailPage, error) {
	page, pageSize = normalizePage(page, pageSize)
	if _, err := r.db.Exec(`DELETE FROM outbound_mail WHERE created_at < ?`, formatTime(time.Now().UTC().Add(-90*24*time.Hour))); err != nil {
		return OutboundMailPage{}, fmt.Errorf("expire outbound mail: %w", err)
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM outbound_mail`).Scan(&total); err != nil {
		return OutboundMailPage{}, fmt.Errorf("count outbound mail: %w", err)
	}
	offset := (page - 1) * pageSize
	rows, err := r.db.Query(`SELECT id, recipient, subject, body, created_at FROM outbound_mail ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return OutboundMailPage{}, fmt.Errorf("list outbound mail: %w", err)
	}
	defer rows.Close()
	items := make([]OutboundMail, 0)
	for rows.Next() {
		var item OutboundMail
		var createdAt string
		if err := rows.Scan(&item.ID, &item.Recipient, &item.Subject, &item.Body, &createdAt); err != nil {
			return OutboundMailPage{}, fmt.Errorf("scan outbound mail: %w", err)
		}
		item.CreatedAt = parseTime(createdAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OutboundMailPage{}, err
	}
	return OutboundMailPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func HashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (r *SQLiteRepository) SavePasswordReset(username, tokenHash string, expires time.Time) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin password reset save: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.Exec(`DELETE FROM password_resets WHERE username = ? OR expires_at <= ?`, username, formatTime(now)); err != nil {
		return fmt.Errorf("replace old password resets: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO password_resets(token_hash, username, expires_at, created_at) VALUES (?, ?, ?, ?)`, tokenHash, username, formatTime(expires), formatTime(now)); err != nil {
		return fmt.Errorf("save password reset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit password reset save: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) DeletePasswordReset(tokenHash string) error {
	if _, err := r.db.Exec(`DELETE FROM password_resets WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete password reset: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ConsumePasswordReset(tokenHash string, now time.Time) (string, error) {
	var username, expiresAt string
	err := r.db.QueryRow(`SELECT username, expires_at FROM password_resets WHERE token_hash = ?`, tokenHash).Scan(&username, &expiresAt)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read password reset: %w", err)
	}
	if parseTime(expiresAt).Before(now) {
		_, _ = r.db.Exec(`DELETE FROM password_resets WHERE token_hash = ?`, tokenHash)
		return "", ErrNotFound
	}
	if _, err := r.db.Exec(`DELETE FROM password_resets WHERE token_hash = ?`, tokenHash); err != nil {
		return "", fmt.Errorf("consume password reset: %w", err)
	}
	return username, nil
}

func (r *SQLiteRepository) BackupDatabase(dest string) error {
	dest = filepath.ToSlash(dest)
	escaped := strings.ReplaceAll(dest, "'", "''")
	if _, err := r.db.Exec(`VACUUM INTO '` + escaped + `'`); err != nil {
		return fmt.Errorf("backup database: %w", err)
	}
	return nil
}
