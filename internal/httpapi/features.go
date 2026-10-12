package httpapi

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func (s *Server) loginFailureLimit() int {
	if s.settings.LoginFailureLimit > 0 {
		return s.settings.LoginFailureLimit
	}
	return 5
}

func (s *Server) commentRateLimit() int {
	if s.settings.CommentLimit > 0 {
		return s.settings.CommentLimit
	}
	return 30
}

func (s *Server) sessionTTL() time.Duration {
	minutes := s.settings.SessionTTLMin
	if minutes < 1 {
		minutes = 60 * 24
	}
	return time.Duration(minutes) * time.Minute
}

func (s *Server) allowComment(r *http.Request) bool {
	return s.commentHits.Allow(clientHost(r), s.commentRateLimit(), time.Minute)
}

func clientHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		if r.RemoteAddr != "" {
			return r.RemoteAddr
		}
		return "unknown"
	}
	return host
}

type forgotRequest struct {
	Username string `json:"username"`
}

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if !s.repo.Initialized() {
		writeError(w, http.StatusPreconditionRequired, "setup_required", "Complete the initial setup at /setting")
		return
	}
	var request forgotRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := s.app.RequestPasswordReset(request.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "mail_failed", "Unable to send mail")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "message": "如果账户存在，重置说明已发送到站点邮箱"})
}

type resetRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	if !s.repo.Initialized() {
		writeError(w, http.StatusPreconditionRequired, "setup_required", "Complete the initial setup at /setting")
		return
	}
	var request resetRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := s.app.ResetPassword(request.Token, request.Password); err != nil {
		if serviceErr, code := serviceError(err); code == "validation_error" {
			writeError(w, serviceErr, code, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_token", "The reset token is invalid or expired")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reset": true})
}

func (s *Server) listPostRevisions(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	revisions, err := s.app.ListPostRevisions(r.PathValue("id"))
	if err != nil {
		status, code := serviceError(err)
		if code == "not_found" {
			writeError(w, http.StatusNotFound, "not_found", "Post not found")
			return
		}
		writeError(w, status, "read_failed", "Unable to read revisions")
		return
	}
	if revisions == nil {
		revisions = []store.PostRevision{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": revisions})
}

func (s *Server) restorePostRevision(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	restored, err := s.app.RestorePostRevision(r.PathValue("id"), r.PathValue("revisionID"))
	if err != nil {
		status, code := serviceError(err)
		if code == "not_found" {
			writeError(w, http.StatusNotFound, "not_found", "Revision not found")
			return
		}
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "post.restored", "post", restored.ID, map[string]string{"revisionId": r.PathValue("revisionID")})
	writeJSON(w, http.StatusOK, publicPost(restored, true))
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwner(w, r) {
		return
	}
	tempDir, err := os.MkdirTemp("", "typecho-backup-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup_failed", "Unable to prepare the backup")
		return
	}
	defer os.RemoveAll(tempDir)
	dbPath := filepath.Join(tempDir, "app.db")
	if err := s.repo.BackupDatabase(dbPath); err != nil {
		writeError(w, http.StatusInternalServerError, "backup_failed", "Unable to prepare the backup")
		return
	}
	archivePath := filepath.Join(tempDir, "backup.zip")
	if err := buildBackup(archivePath, dbPath, s.mediaRoot); err != nil {
		s.logger.Error("prepare backup", "error", err)
		writeError(w, http.StatusInternalServerError, "backup_failed", "Unable to prepare the backup")
		return
	}
	file, err := os.Open(archivePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup_failed", "Unable to prepare the backup")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup_failed", "Unable to prepare the backup")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="typecho-backup.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "typecho-backup.zip", info.ModTime(), file)
}

// Build the complete archive before sending a successful HTTP response. Media
// references come from the database snapshot, rather than the changing live DB.
func buildBackup(archivePath, dbPath, mediaRoot string) (err error) {
	absolutePath, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	databasePath := filepath.ToSlash(absolutePath)
	if !strings.HasPrefix(databasePath, "/") {
		databasePath = "/" + databasePath
	}
	databaseURL := url.URL{Scheme: "file", Path: databasePath, RawQuery: "mode=ro"}
	snapshot, err := sql.Open("sqlite3", databaseURL.String())
	if err != nil {
		return err
	}
	defer snapshot.Close()
	output, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
	}()
	archive := zip.NewWriter(output)
	defer func() {
		if closeErr := archive.Close(); err == nil {
			err = closeErr
		}
	}()
	if err := addZipFile(archive, "app.db", dbPath); err != nil {
		return err
	}
	rows, err := snapshot.Query("SELECT id, storage_key FROM media ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			return err
		}
		if key == "" || key == "." || key == ".." || filepath.Base(key) != key || strings.ContainsAny(key, `/\`) {
			return fmt.Errorf("invalid media storage key: %q", key)
		}
		if err := addZipFile(archive, "media/"+key, filepath.Join(mediaRoot, key)); err != nil {
			return fmt.Errorf("backup media %s: %w", id, err)
		}
	}
	return rows.Err()
}

func addZipFile(archive *zip.Writer, name, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup file is not a regular file: %s", name)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = name
	header.Method = zip.Deflate
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, file)
	return err
}

func (s *Server) serveAppShell(w http.ResponseWriter, r *http.Request, cleanPath string) {
	raw, err := os.ReadFile(filepath.Join(s.webDir, "index.html"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	siteName := "Typecho Next"
	description := ""
	var site *store.Site
	if s.repo.Initialized() {
		if loaded, err := s.app.Site(); err == nil && loaded != nil && strings.TrimSpace(loaded.Name) != "" {
			site = loaded
			siteName = loaded.Name
			description = strings.TrimSpace(loaded.Description)
		}
	}
	title := siteName
	body := "<h1>" + html.EscapeString(siteName) + "</h1>"
	if description != "" {
		body += "<p>" + html.EscapeString(description) + "</p>"
	}
	if slug, ok := routeSlug(cleanPath, "/user/"); ok {
		if post, err := s.app.GetPost(slug, false); err == nil {
			title = post.Title + " - " + siteName
			body = "<article><h1>" + html.EscapeString(post.Title) + "</h1>"
			if post.PassphraseHash == "" {
				body += "<div>" + html.EscapeString(post.Content) + "</div>"
			}
			body += "</article>"
		}
	}
	if slug, ok := routeSlug(cleanPath, "/page/"); ok {
		if page, err := s.app.GetPage(slug, false); err == nil {
			title = page.Title + " - " + siteName
			body = "<article><h1>" + html.EscapeString(page.Title) + "</h1><div>" + html.EscapeString(page.Content) + "</div></article>"
		}
	}
	if site == nil && (cleanPath == "/admin" || strings.HasPrefix(cleanPath, "/setting")) {
		body = "<h1>" + html.EscapeString(siteName) + "</h1>"
	}
	htmlText := replaceHTMLTitle(string(raw), title)
	const marker = `<div id="app"></div>`
	if strings.Contains(htmlText, marker) {
		htmlText = strings.Replace(htmlText, marker, `<div id="app">`+body+`</div>`, 1)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, htmlText)
}

func replaceHTMLTitle(htmlText, title string) string {
	start := strings.Index(htmlText, "<title>")
	end := strings.Index(htmlText, "</title>")
	if start < 0 || end < start {
		return htmlText
	}
	return htmlText[:start] + "<title>" + html.EscapeString(title) + htmlText[end:]
}

func routeSlug(cleanPath, prefix string) (string, bool) {
	if !strings.HasPrefix(cleanPath, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(cleanPath, prefix)
	if rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}
