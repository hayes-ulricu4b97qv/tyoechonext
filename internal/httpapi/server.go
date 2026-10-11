package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/config"
	"github.com/untesheazulchunghu-beep/typecho/internal/service"
	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

const (
	maxJSONBody = 1 << 20
	cookieName  = "app_session"
)

type Server struct {
	repo     store.Repository
	app      *service.Service
	settings config.Settings
	logger   *slog.Logger
	webDir   string
}

func NewServer(repo store.Repository, settings config.Settings, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	webDir := os.Getenv("APP_WEB_DIR")
	if webDir == "" {
		webDir = "./web/dist"
	}
	return &Server{
		repo:     repo,
		app:      service.New(repo),
		settings: settings,
		logger:   logger,
		webDir:   webDir,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/system/status", s.systemStatus)
	mux.HandleFunc("POST /api/v1/setup", s.setup)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/me", s.me)
	mux.HandleFunc("GET /api/v1/content/posts", s.listPosts)
	mux.HandleFunc("GET /api/v1/content/posts/{slug}", s.getPost)
	mux.HandleFunc("POST /api/v1/content/posts", s.createPost)
	mux.HandleFunc("PUT /api/v1/content/posts/{id}", s.updatePost)
	mux.HandleFunc("DELETE /api/v1/content/posts/{id}", s.deletePost)
	mux.HandleFunc("GET /api/v1/dashboard/stats", s.dashboardStats)
	mux.HandleFunc("GET /api/v1/settings/site", s.getSite)
	mux.HandleFunc("PUT /api/v1/settings/site", s.updateSite)
	mux.HandleFunc("/", s.frontend)

	return requestLog(s.routeGuard(mux), s.logger)
}

func (s *Server) routeGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/v1/system/status" || r.URL.Path == "/api/v1/setup" || r.URL.Path == "/api/v1/auth/login" || strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/favicon.svg" {
			next.ServeHTTP(w, r)
			return
		}
		if !s.repo.Initialized() {
			if r.URL.Path == "/setting" || strings.HasPrefix(r.URL.Path, "/setting/") {
				next.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, http.StatusPreconditionRequired, "setup_required", "Complete the initial setup at /setting")
				return
			}
			http.Redirect(w, r, "/setting", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/user", http.StatusTemporaryRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": s.settings.Version})
}

func (s *Server) systemStatus(w http.ResponseWriter, _ *http.Request) {
	site, err := s.app.Site()
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read site status")
		return
	}
	response := map[string]any{
		"initialized": s.repo.Initialized(),
		"version":     s.settings.Version,
		"features":    []string{"content", "sessions", "settings"},
	}
	if site != nil {
		response["site"] = publicSite(site)
	}
	writeJSON(w, http.StatusOK, response)
}

type setupRequest struct {
	SiteName      string `json:"siteName"`
	SiteURL       string `json:"siteUrl"`
	AdminUsername string `json:"adminUsername"`
	AdminPassword string `json:"adminPassword"`
	AdminEmail    string `json:"adminEmail"`
	Timezone      string `json:"timezone"`
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if s.repo.Initialized() {
		writeError(w, http.StatusConflict, "already_initialized", "The application has already been initialized")
		return
	}
	var request setupRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	setup, err := s.app.Initialize(service.SetupInput{
		SiteName:      request.SiteName,
		SiteURL:       request.SiteURL,
		AdminUsername: request.AdminUsername,
		AdminPassword: request.AdminPassword,
		AdminEmail:    request.AdminEmail,
		Timezone:      request.Timezone,
	})
	if err != nil {
		status := http.StatusConflict
		code := "setup_failed"
		if errors.Is(err, service.ErrValidation) {
			status = http.StatusUnprocessableEntity
			code = "validation_error"
		}
		writeError(w, status, code, err.Error())
		return
	}
	if err := s.issueSession(w, request.AdminUsername); err != nil {
		writeError(w, http.StatusInternalServerError, "session_failed", "Unable to create a sign-in session")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"initialized": true, "site": publicSite(setup)})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.repo.Initialized() {
		writeError(w, http.StatusPreconditionRequired, "setup_required", "Complete the initial setup at /setting")
		return
	}
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	admin, err := s.app.Authenticate(request.Username, request.Password)
	if err != nil && !errors.Is(err, service.ErrInvalidCredentials) {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read administrator account")
		return
	}
	if errors.Is(err, service.ErrInvalidCredentials) || admin == nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Username or password is incorrect")
		return
	}
	if err := s.issueSession(w, admin.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "session_failed", "Unable to create a sign-in session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": map[string]string{"username": admin.Username, "email": admin.Email}})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		_ = s.app.RevokeSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.settings.CookieSecure})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.currentAdmin(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sign in to continue")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": map[string]string{"username": admin.Username, "email": admin.Email}})
}

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	page, pageSize := pagination(r)
	posts, err := s.app.ListPosts(r.URL.Query().Get("q"), s.isAdmin(r), page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read posts")
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	post, err := s.app.GetPost(slug, s.isAdmin(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read post")
		return
	}
	writeJSON(w, http.StatusOK, post)
}

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var post store.Post
	if err := decodeJSON(w, r, &post); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	created, err := s.app.CreatePost(post)
	if err != nil {
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updatePost(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	post, err := s.app.GetPost(r.PathValue("id"), true)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	var input store.Post
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, err := s.app.UpdatePost(post.ID, input)
	if err != nil {
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if err := s.app.DeletePost(r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) dashboardStats(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	stats, err := s.app.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read post stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	site, err := s.app.Site()
	if errors.Is(err, store.ErrNotFound) || site == nil {
		writeError(w, http.StatusNotFound, "not_found", "Site settings not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read site settings")
		return
	}
	writeJSON(w, http.StatusOK, publicSite(site))
}

func (s *Server) updateSite(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var input struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		AdminMail string `json:"adminMail"`
		Timezone  string `json:"timezone"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	updated, err := s.app.UpdateSite(store.Site{Name: input.Name, URL: input.URL, AdminMail: input.AdminMail, Timezone: input.Timezone})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Site settings not found")
			return
		}
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicSite(updated))
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sign in to continue")
		return false
	}
	return true
}

func (s *Server) isAdmin(r *http.Request) bool {
	_, ok := s.currentAdmin(r)
	return ok
}

func (s *Server) currentAdmin(r *http.Request) (*store.Admin, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	username, err := s.app.ResolveSession(cookie.Value, time.Now().UTC())
	if err != nil {
		return nil, false
	}
	admin, err := s.app.Admin()
	if err != nil || admin == nil || admin.Username != username {
		return nil, false
	}
	return admin, true
}

func (s *Server) issueSession(w http.ResponseWriter, username string) error {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	expires := time.Now().Add(time.Duration(s.settings.SessionTTLMin) * time.Minute)
	if err := s.app.CreateSession(username, token, expires); err != nil {
		return err
	}
	_ = s.app.PurgeSessions(time.Now().UTC())
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", Expires: expires, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.settings.CookieSecure})
	return nil
}

func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	cleanPath := path.Clean(r.URL.Path)
	if cleanPath == "/" {
		if s.repo.Initialized() {
			http.Redirect(w, r, "/user", http.StatusTemporaryRedirect)
		} else {
			http.Redirect(w, r, "/setting", http.StatusTemporaryRedirect)
		}
		return
	}
	if strings.HasPrefix(cleanPath, "/assets/") || cleanPath == "/favicon.svg" {
		file := path.Join(s.webDir, cleanPath)
		if _, err := os.Stat(file); err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, file)
		return
	}
	if cleanPath == "/admin" || strings.HasPrefix(cleanPath, "/admin/") || cleanPath == "/user" || strings.HasPrefix(cleanPath, "/user/") || cleanPath == "/setting" || strings.HasPrefix(cleanPath, "/setting/") {
		index := path.Join(s.webDir, "index.html")
		if _, err := os.Stat(index); err == nil {
			http.ServeFile(w, r, index)
			return
		}
	}
	http.NotFound(w, r)
}

func serviceError(err error) (int, string) {
	if errors.Is(err, service.ErrValidation) {
		return http.StatusUnprocessableEntity, "validation_error"
	}
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound, "not_found"
	}
	return http.StatusInternalServerError, "save_failed"
}

func pagination(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func publicSite(site *store.Site) map[string]string {
	return map[string]string{"name": site.Name, "url": site.URL, "adminMail": site.AdminMail, "timezone": site.Timezone}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func requestLog(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
	})
}
