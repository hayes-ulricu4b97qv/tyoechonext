package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (r *SQLiteRepository) Initialized() bool {
	var initialized bool
	err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sites WHERE id = 1) AND EXISTS(SELECT 1 FROM users WHERE role = 'owner' AND active = 1)`).Scan(&initialized)
	return err == nil && initialized
}

func (r *SQLiteRepository) Site() (*Site, error) {
	var site Site
	var createdAt, updatedAt string
	var postsPerPage, commentsEnabled, commentsRequireModeration, commentsThreaded int
	err := r.db.QueryRow(`SELECT name, url, admin_mail, timezone, description, keywords, posts_per_page, comments_enabled, comments_require_moderation, comments_threaded, created_at, updated_at FROM sites WHERE id = 1`).Scan(&site.Name, &site.URL, &site.AdminMail, &site.Timezone, &site.Description, &site.Keywords, &postsPerPage, &commentsEnabled, &commentsRequireModeration, &commentsThreaded, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read site: %w", err)
	}
	site.PostsPerPage = postsPerPage
	site.CommentsEnabled = commentsEnabled != 0
	site.CommentsRequireModeration = commentsRequireModeration != 0
	site.CommentsThreaded = commentsThreaded != 0
	if site.PostsPerPage < 1 || site.PostsPerPage > 50 {
		site.PostsPerPage = 10
	}
	site.CreatedAt = parseTime(createdAt)
	site.UpdatedAt = parseTime(updatedAt)
	return &site, nil
}

func (r *SQLiteRepository) Admin() (*Admin, error) {
	row := r.db.QueryRow(`SELECT id, username, password_hash, email, role, active, created_at, updated_at FROM users WHERE role = 'owner' ORDER BY created_at ASC, id ASC LIMIT 1`)
	admin, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read owner: %w", err)
	}
	return &admin, nil
}

func (r *SQLiteRepository) UpdateAdmin(admin Admin) error {
	result, err := r.db.Exec(`UPDATE users SET password_hash = ?, email = ?, updated_at = ? WHERE role = 'owner' AND username = ?`, admin.PasswordHash, admin.Email, formatTime(time.Now().UTC()), admin.Username)
	if err != nil {
		return fmt.Errorf("update owner: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) Users() ([]User, error) {
	rows, err := r.db.Query(`SELECT id, username, password_hash, email, role, active, created_at, updated_at FROM users ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (r *SQLiteRepository) User(username string) (User, error) {
	row := r.db.QueryRow(`SELECT id, username, password_hash, email, role, active, created_at, updated_at FROM users WHERE username = ?`, username)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("read user: %w", err)
	}
	return user, nil
}

func (r *SQLiteRepository) CreateUser(user User) (User, error) {
	if user.ID == "" {
		user.ID = newUserID()
	}
	if user.Role == "" {
		user.Role = "editor"
	}
	user.Active = true
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now
	_, err := r.db.Exec(`INSERT INTO users(id, username, password_hash, email, role, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, user.ID, user.Username, user.PasswordHash, user.Email, user.Role, boolToInt(user.Active), formatTime(user.CreatedAt), formatTime(user.UpdatedAt))
	if err != nil {
		return User{}, wrapSQLiteWriteError("create user", err)
	}
	return r.User(user.Username)
}

func (r *SQLiteRepository) UpdateUser(user User) (User, error) {
	user.UpdatedAt = time.Now().UTC()
	var result sql.Result
	var err error
	if user.PasswordHash == "" {
		result, err = r.db.Exec(`UPDATE users SET email = ?, role = ?, active = ?, updated_at = ? WHERE id = ?`, user.Email, user.Role, boolToInt(user.Active), formatTime(user.UpdatedAt), user.ID)
	} else {
		result, err = r.db.Exec(`UPDATE users SET password_hash = ?, email = ?, role = ?, active = ?, updated_at = ? WHERE id = ?`, user.PasswordHash, user.Email, user.Role, boolToInt(user.Active), formatTime(user.UpdatedAt), user.ID)
	}
	if err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return User{}, ErrNotFound
	}
	return r.User(user.Username)
}

func (r *SQLiteRepository) DeleteUser(username string) error {
	result, err := r.db.Exec(`DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
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
	admin.UpdatedAt = now
	admin.ID = "user-owner"
	admin.Role = "owner"
	admin.Active = true
	if _, err := tx.Exec(`INSERT INTO sites(id, name, url, admin_mail, timezone, created_at, updated_at) VALUES (1, ?, ?, ?, ?, ?, ?)`, site.Name, site.URL, site.AdminMail, site.Timezone, formatTime(site.CreatedAt), formatTime(site.UpdatedAt)); err != nil {
		return fmt.Errorf("save site: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO users(id, username, password_hash, email, role, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, admin.ID, admin.Username, admin.PasswordHash, admin.Email, admin.Role, boolToInt(admin.Active), formatTime(admin.CreatedAt), formatTime(admin.UpdatedAt)); err != nil {
		return fmt.Errorf("save owner user: %w", err)
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
	result, err := r.db.Exec(`UPDATE sites SET name = ?, url = ?, admin_mail = ?, timezone = ?, description = ?, keywords = ?, posts_per_page = ?, comments_enabled = ?, comments_require_moderation = ?, comments_threaded = ?, updated_at = ? WHERE id = 1`, site.Name, site.URL, site.AdminMail, site.Timezone, site.Description, site.Keywords, site.PostsPerPage, boolToInt(site.CommentsEnabled), boolToInt(site.CommentsRequireModeration), boolToInt(site.CommentsThreaded), formatTime(site.UpdatedAt))
	if err != nil {
		return fmt.Errorf("update site: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}
