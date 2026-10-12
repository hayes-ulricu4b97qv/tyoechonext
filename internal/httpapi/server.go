package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/config"
	"github.com/untesheazulchunghu-beep/typecho/internal/service"
	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

const (
	// maxJSONBody is large enough for one post or page document whose body
	// is a single rune past the documented maximum. UTF-8 uses at most 4
	// bytes per rune, and encoding/json may escape a rune as \uXXXX.
	// The extra rune lets validation, rather than the transport limit,
	// reject an over-long body. The remaining allowance covers the other
	// documented fields and JSON framing.
	maxFieldRunes = 200 + 120 + 500 + 20*60 + 20*60
	maxJSONBody   = (service.MaxContentRunes+1)*6 + maxFieldRunes*4 + 16*1024
	maxUploadSize = 10 << 20
	cookieName    = "app_session"
	csrfCookie    = "app_csrf"
	csrfHeader    = "X-CSRF-Token"
)

type Server struct {
	repo          store.Repository
	app           *service.Service
	settings      config.Settings
	logger        *slog.Logger
	webDir        string
	mediaRoot     string
	loginFailures *service.HitCounter
	commentHits   *service.HitCounter
}

func NewServer(repo store.Repository, settings config.Settings, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	webDir := os.Getenv("APP_WEB_DIR")
	if webDir == "" {
		webDir = "./web/dist"
	}
	mediaRoot := os.Getenv("APP_MEDIA_DIR")
	if mediaRoot == "" {
		dataDir := os.Getenv("APP_DATA_DIR")
		if dataDir == "" {
			dataDir = "./data"
		}
		mediaRoot = filepath.Join(dataDir, "uploads")
	}
	return &Server{
		repo:          repo,
		app:           service.New(repo),
		settings:      settings,
		logger:        logger,
		webDir:        webDir,
		mediaRoot:     mediaRoot,
		loginFailures: service.NewHitCounter(),
		commentHits:   service.NewHitCounter(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /feed.xml", s.feed)
	mux.HandleFunc("GET /sitemap.xml", s.sitemap)
	mux.HandleFunc("GET /robots.txt", s.robots)
	mux.HandleFunc("GET /api/v1/system/status", s.systemStatus)
	mux.HandleFunc("POST /api/v1/setup", s.setup)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/forgot", s.forgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset", s.resetPassword)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/me", s.me)
	mux.HandleFunc("GET /api/v1/content/posts", s.listPosts)
	mux.HandleFunc("GET /api/v1/content/archive", s.listArchive)
	mux.HandleFunc("GET /api/v1/content/comments/recent", s.listRecentComments)
	mux.HandleFunc("GET /api/v1/content/pages", s.listPages)
	mux.HandleFunc("POST /api/v1/content/pages", s.createPage)
	mux.HandleFunc("GET /api/v1/content/pages/{slug}", s.getPage)
	mux.HandleFunc("PUT /api/v1/content/pages/{id}", s.updatePage)
	mux.HandleFunc("DELETE /api/v1/content/pages/{id}", s.deletePage)
	mux.HandleFunc("GET /api/v1/content/terms", s.listTerms)
	mux.HandleFunc("POST /api/v1/content/terms", s.createTerm)
	mux.HandleFunc("PUT /api/v1/content/terms/{id}", s.updateTerm)
	mux.HandleFunc("DELETE /api/v1/content/terms/{id}", s.deleteTerm)
	mux.HandleFunc("GET /api/v1/content/posts/{id}/revisions", s.listPostRevisions)
	mux.HandleFunc("POST /api/v1/content/posts/{id}/revisions/{revisionID}/restore", s.restorePostRevision)
	mux.HandleFunc("GET /api/v1/content/posts/{slug}", s.getPost)
	mux.HandleFunc("GET /api/v1/content/posts/{slug}/comments", s.listComments)
	mux.HandleFunc("POST /api/v1/content/posts/{slug}/comments", s.createComment)
	mux.HandleFunc("GET /api/v1/content/comments", s.listAllComments)
	mux.HandleFunc("PUT /api/v1/content/comments/{id}", s.updateComment)
	mux.HandleFunc("DELETE /api/v1/content/comments/{id}", s.deleteComment)
	mux.HandleFunc("GET /api/v1/media", s.listMedia)
	mux.HandleFunc("POST /api/v1/media", s.uploadMedia)
	mux.HandleFunc("DELETE /api/v1/media/{id}", s.deleteMedia)
	mux.HandleFunc("GET /media/{id}", s.serveMedia)
	mux.HandleFunc("POST /api/v1/content/posts", s.createPost)
	mux.HandleFunc("PUT /api/v1/content/posts/{id}", s.updatePost)
	mux.HandleFunc("DELETE /api/v1/content/posts/{id}", s.deletePost)
	mux.HandleFunc("GET /api/v1/dashboard/stats", s.dashboardStats)
	mux.HandleFunc("GET /api/v1/settings/site", s.getSite)
	mux.HandleFunc("PUT /api/v1/settings/site", s.updateSite)
	mux.HandleFunc("GET /api/v1/settings/reading", s.getReading)
	mux.HandleFunc("PUT /api/v1/settings/reading", s.updateReading)
	mux.HandleFunc("PUT /api/v1/settings/account", s.updateAccount)
	mux.HandleFunc("GET /api/v1/settings/users", s.listUsers)
	mux.HandleFunc("POST /api/v1/settings/users", s.createUser)
	mux.HandleFunc("PUT /api/v1/settings/users/{username}", s.updateUser)
	mux.HandleFunc("DELETE /api/v1/settings/users/{username}", s.deleteUser)
	mux.HandleFunc("GET /api/v1/audit", s.listAudit)
	mux.HandleFunc("GET /api/v1/backup", s.backup)
	mux.HandleFunc("/", s.frontend)

	return requestLog(s.routeGuard(mux), s.logger)
}

func (s *Server) routeGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		s.ensureCSRFCookie(w, r)
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/feed.xml" || r.URL.Path == "/sitemap.xml" || r.URL.Path == "/robots.txt" || r.URL.Path == "/api/v1/system/status" || r.URL.Path == "/api/v1/setup" || r.URL.Path == "/api/v1/auth/login" || strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/favicon.svg" {
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
		if isUnsafeMethod(r.Method) && !s.validCSRF(r) {
			writeError(w, http.StatusForbidden, "csrf_failed", "A valid CSRF token is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(csrfCookie); err == nil && cookie.Value != "" {
		return
	}
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: hex.EncodeToString(buffer), Path: "/", HttpOnly: false, SameSite: http.SameSiteLaxMode, Secure: s.settings.CookieSecure})
}

func (s *Server) validCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	header := r.Header.Get(csrfHeader)
	if header == "" || len(header) != len(cookie.Value) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) == 1
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": s.settings.Version})
}

func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	if !s.repo.Initialized() {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "Complete the initial setup before serving traffic")
		return
	}
	if _, err := s.repo.Site(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "The content store is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "version": s.settings.Version})
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	Description string  `xml:"description"`
	PubDate     string  `xml:"pubDate"`
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink bool   `xml:"isPermaLink,attr"`
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	site, err := s.app.Site()
	if errors.Is(err, store.ErrNotFound) || site == nil {
		writeError(w, http.StatusNotFound, "not_found", "Site settings not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read site settings")
		return
	}
	baseURL := strings.TrimRight(site.URL, "/")
	editor := s.canEditContent(r)
	passphrase := r.Header.Get("X-Post-Passphrase")
	items := make([]rssItem, 0)
	for page := 1; ; page++ {
		posts, err := s.app.ListPosts("", false, page, 100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read published posts")
			return
		}
		for _, post := range posts.Items {
			publishedAt := post.PublishedAt
			if publishedAt.IsZero() {
				publishedAt = post.UpdatedAt
			}
			permalink := baseURL + "/user/" + url.PathEscape(post.Slug)
			content := ""
			if s.app.RevealPost(post, editor, passphrase) {
				content = post.Content
			}
			items = append(items, rssItem{
				Title:       post.Title,
				Link:        permalink,
				GUID:        rssGUID{Value: permalink, IsPermaLink: true},
				Description: service.FeedBlurb(post.Excerpt, content),
				PubDate:     publishedAt.UTC().Format(time.RFC1123Z),
			})
		}
		if len(posts.Items) == 0 || posts.PageSize <= 0 || len(posts.Items) < posts.PageSize {
			break
		}
	}
	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title:         site.Name,
			Link:          baseURL,
			Description:   feedDescription(site),
			LastBuildDate: time.Now().UTC().Format(time.RFC1123Z),
			Items:         items,
		},
	}
	writeXML(w, http.StatusOK, "application/rss+xml; charset=utf-8", feed)
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Location string `xml:"loc"`
	LastMod  string `xml:"lastmod,omitempty"`
}

func (s *Server) sitemap(w http.ResponseWriter, _ *http.Request) {
	site, err := s.app.Site()
	if errors.Is(err, store.ErrNotFound) || site == nil {
		writeError(w, http.StatusNotFound, "not_found", "Site settings not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read site settings")
		return
	}
	baseURL := strings.TrimRight(site.URL, "/")
	urls := []sitemapURL{{Location: baseURL + "/user"}}
	for page := 1; ; page++ {
		posts, err := s.app.ListPosts("", false, page, 100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read published posts")
			return
		}
		for _, post := range posts.Items {
			lastMod := post.UpdatedAt
			if lastMod.IsZero() {
				lastMod = post.PublishedAt
			}
			entry := sitemapURL{Location: baseURL + "/user/" + url.PathEscape(post.Slug)}
			if !lastMod.IsZero() {
				entry.LastMod = lastMod.UTC().Format("2006-01-02")
			}
			urls = append(urls, entry)
		}
		if len(posts.Items) == 0 || posts.PageSize <= 0 || len(posts.Items) < posts.PageSize {
			break
		}
	}
	for page := 1; ; page++ {
		terms, err := s.app.ListTerms("", "", false, page, 100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read published terms")
			return
		}
		for _, term := range terms.Items {
			entry := sitemapURL{Location: baseURL + "/user?" + term.Kind + "=" + url.QueryEscape(term.Slug)}
			if !term.UpdatedAt.IsZero() {
				entry.LastMod = term.UpdatedAt.UTC().Format("2006-01-02")
			}
			urls = append(urls, entry)
		}
		if len(terms.Items) == 0 || terms.PageSize <= 0 || len(terms.Items) < terms.PageSize {
			break
		}
	}
	for page := 1; ; page++ {
		pages, err := s.app.ListPages("", false, page, 100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read published pages")
			return
		}
		for _, item := range pages.Items {
			entry := sitemapURL{Location: baseURL + "/page/" + url.PathEscape(item.Slug)}
			if !item.UpdatedAt.IsZero() {
				entry.LastMod = item.UpdatedAt.UTC().Format("2006-01-02")
			}
			urls = append(urls, entry)
		}
		if len(pages.Items) == 0 || pages.PageSize <= 0 || len(pages.Items) < pages.PageSize {
			break
		}
	}
	writeXML(w, http.StatusOK, "application/xml; charset=utf-8", sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	})
}

func (s *Server) robots(w http.ResponseWriter, _ *http.Request) {
	site, err := s.app.Site()
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read site settings")
		return
	}
	lines := []string{"User-agent: *", "Allow: /", "Disallow: /admin", "Disallow: /setting"}
	if site != nil && strings.TrimSpace(site.URL) != "" {
		lines = append(lines, "Sitemap: "+strings.TrimRight(site.URL, "/")+"/sitemap.xml")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, strings.Join(lines, "\n")+"\n")
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
		"features":    []string{"content", "pages", "taxonomy", "comments", "discussion", "media", "users", "sessions", "settings", "feeds", "archive", "audit"},
	}
	if site != nil {
		response["site"] = publicSite(site)
		response["reading"] = publicReading(site)
	}
	writeJSON(w, http.StatusOK, response)
}

type postRequest struct {
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	Excerpt      string   `json:"excerpt"`
	Content      string   `json:"content"`
	Status       string   `json:"status"`
	AllowComment *bool    `json:"allowComment"`
	Categories   []string `json:"categories"`
	Tags         []string `json:"tags"`
	PublishAt    *string  `json:"publishAt"`
	Passphrase   *string  `json:"passphrase"`
}

func (input postRequest) post() (store.Post, error) {
	post := store.Post{
		Title:      input.Title,
		Slug:       input.Slug,
		Excerpt:    input.Excerpt,
		Content:    input.Content,
		Status:     input.Status,
		Categories: termInputs(input.Categories, "category"),
		Tags:       termInputs(input.Tags, "tag"),
	}
	if input.PublishAt != nil && strings.TrimSpace(*input.PublishAt) != "" {
		raw := strings.TrimSpace(*input.PublishAt)
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, raw)
		}
		if err != nil {
			return store.Post{}, fmt.Errorf("%w: 发布时间格式无效", service.ErrValidation)
		}
		post.RequestedPublishAt = parsed.UTC()
	}
	if input.Passphrase != nil {
		post.SetPassphrase = true
		post.PassphrasePlain = *input.Passphrase
	}
	return post, nil
}

func applyAllowComment(post store.Post, input postRequest, fallback bool) store.Post {
	post.AllowComment = fallback
	if input.AllowComment != nil {
		post.AllowComment = *input.AllowComment
	}
	return post
}

func termInputs(names []string, kind string) []store.Term {
	terms := make([]store.Term, 0, len(names))
	for _, name := range names {
		terms = append(terms, store.Term{Name: name, Kind: kind})
	}
	return terms
}

type pageRequest struct {
	Title   string `json:"title"`
	Slug    string `json:"slug"`
	Excerpt string `json:"excerpt"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

func (input pageRequest) page() store.Page {
	return store.Page{Title: input.Title, Slug: input.Slug, Excerpt: input.Excerpt, Content: input.Content, Status: input.Status}
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sign in to continue")
		return false
	}
	return true
}

func (s *Server) requireOwner(w http.ResponseWriter, r *http.Request) bool {
	user, ok := s.currentAdmin(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sign in to continue")
		return false
	}
	if user.Role != "owner" {
		writeError(w, http.StatusForbidden, "forbidden", "Owner permission is required")
		return false
	}
	return true
}

func (s *Server) requireContentEditor(w http.ResponseWriter, r *http.Request) bool {
	return s.requireRole(w, r, "owner", "editor")
}

func (s *Server) requireModerator(w http.ResponseWriter, r *http.Request) bool {
	return s.requireRole(w, r, "owner", "moderator")
}

func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, roles ...string) bool {
	user, ok := s.currentAdmin(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sign in to continue")
		return false
	}
	for _, role := range roles {
		if user.Role == role {
			return true
		}
	}
	writeError(w, http.StatusForbidden, "forbidden", "You do not have permission for this action")
	return false
}

func (s *Server) isAdmin(r *http.Request) bool {
	_, ok := s.currentAdmin(r)
	return ok
}

func (s *Server) canEditContent(r *http.Request) bool {
	user, ok := s.currentAdmin(r)
	return ok && (user.Role == "owner" || user.Role == "editor")
}

func (s *Server) canModerate(r *http.Request) bool {
	user, ok := s.currentAdmin(r)
	return ok && (user.Role == "owner" || user.Role == "moderator")
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
	admin, err := s.app.User(username)
	if err != nil || admin == nil || admin.Username != username || !admin.Active {
		return nil, false
	}
	return admin, true
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *Server) recordAudit(r *http.Request, action, resourceType, resourceID string, details map[string]string) {
	event := store.AuditEvent{
		ActorUsername: "anonymous",
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Details:       details,
	}
	if user, ok := s.currentAdmin(r); ok {
		event.ActorUserID = user.ID
		event.ActorUsername = user.Username
	}
	if err := s.app.RecordAudit(event); err != nil {
		s.logger.Warn("record audit event failed", "action", action, "resource_type", resourceType, "resource_id", resourceID, "error", err)
	}
}

func (s *Server) issueSession(w http.ResponseWriter, username string) error {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	expires := time.Now().Add(s.sessionTTL())
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
	if cleanPath == "/admin" || strings.HasPrefix(cleanPath, "/admin/") || cleanPath == "/user" || strings.HasPrefix(cleanPath, "/user/") || cleanPath == "/page" || strings.HasPrefix(cleanPath, "/page/") || cleanPath == "/setting" || strings.HasPrefix(cleanPath, "/setting/") {
		s.serveAppShell(w, r, cleanPath)
		return
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

func (s *Server) readingPage(r *http.Request) (int, int, error) {
	page, pageSize, err := pagination(r)
	if err != nil {
		return 0, 0, err
	}
	if strings.TrimSpace(r.URL.Query().Get("pageSize")) != "" {
		return page, pageSize, nil
	}
	site, err := s.app.Site()
	if err != nil || site == nil || site.PostsPerPage < 1 || site.PostsPerPage > 50 {
		return page, pageSize, nil
	}
	return page, site.PostsPerPage, nil
}

func (s *Server) pages(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page, pageSize, err := pagination(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return 0, 0, false
	}
	return page, pageSize, true
}

func pagination(r *http.Request) (int, int, error) {
	page := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	pageSize := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("pageSize")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return 0, 0, fmt.Errorf("%w: pageSize 必须是 1 到 100 的整数", service.ErrValidation)
		}
		pageSize = parsed
	}
	return page, pageSize, nil
}

func feedDescription(site *store.Site) string {
	if site != nil {
		if description := strings.TrimSpace(site.Description); description != "" {
			return description
		}
		return site.Name + " latest posts"
	}
	return "latest posts"
}

func publicSite(site *store.Site) map[string]string {
	return map[string]string{
		"name":        site.Name,
		"url":         site.URL,
		"adminMail":   site.AdminMail,
		"timezone":    site.Timezone,
		"description": site.Description,
		"keywords":    site.Keywords,
	}
}

func publicReading(site *store.Site) map[string]any {
	postsPerPage := site.PostsPerPage
	if postsPerPage < 1 || postsPerPage > 50 {
		postsPerPage = 10
	}
	return map[string]any{
		"postsPerPage":              postsPerPage,
		"commentsEnabled":           site.CommentsEnabled,
		"commentsRequireModeration": site.CommentsRequireModeration,
		"commentsThreaded":          site.CommentsThreaded,
	}
}

func publicUser(user *store.User) map[string]string {
	return map[string]string{"username": user.Username, "email": user.Email, "role": user.Role}
}

type userResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
}

func publicUserRecord(user store.User) userResponse {
	return userResponse{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role, Active: user.Active, CreatedAt: user.CreatedAt}
}

type postResponse struct {
	ID           string                `json:"id"`
	Title        string                `json:"title"`
	Slug         string                `json:"slug"`
	Excerpt      string                `json:"excerpt"`
	Content      *string               `json:"content,omitempty"`
	Locked       bool                  `json:"locked,omitempty"`
	Status       string                `json:"status"`
	AllowComment bool                  `json:"allowComment"`
	Categories   []termResponse        `json:"categories,omitempty"`
	Tags         []termResponse        `json:"tags,omitempty"`
	PublishedAt  *time.Time            `json:"publishedAt,omitempty"`
	UpdatedAt    time.Time             `json:"updatedAt"`
	Previous     *postNeighborResponse `json:"previous,omitempty"`
	Next         *postNeighborResponse `json:"next,omitempty"`
}

type postNeighborResponse struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Slug        string     `json:"slug"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

type termResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Kind      string `json:"kind"`
	PostCount int    `json:"postCount"`
}

type termPageResponse struct {
	Items    []termResponse `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

type postPageResponse struct {
	Items    []postResponse `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

type pageResponse struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Slug        string     `json:"slug"`
	Excerpt     string     `json:"excerpt"`
	Content     string     `json:"content"`
	Status      string     `json:"status"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type pagePageResponse struct {
	Items    []pageResponse `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

func publicPostWithNeighbors(post store.Post, previous, next *store.Post, withBody bool) postResponse {
	response := publicPost(post, withBody)
	response.Previous = publicNeighbor(previous)
	response.Next = publicNeighbor(next)
	return response
}

func publicNeighbor(post *store.Post) *postNeighborResponse {
	if post == nil {
		return nil
	}
	response := &postNeighborResponse{ID: post.ID, Title: post.Title, Slug: post.Slug}
	if !post.PublishedAt.IsZero() {
		publishedAt := post.PublishedAt
		response.PublishedAt = &publishedAt
	}
	return response
}

func publicPost(post store.Post, withBody bool) postResponse {
	response := postResponse{
		ID:           post.ID,
		Title:        post.Title,
		Slug:         post.Slug,
		Excerpt:      post.Excerpt,
		Status:       post.Status,
		AllowComment: post.AllowComment,
		Locked:       post.PassphraseHash != "",
		UpdatedAt:    post.UpdatedAt,
	}
	if withBody {
		content := post.Content
		response.Content = &content
	}
	response.Categories = publicTerms(post.Categories)
	response.Tags = publicTerms(post.Tags)
	if !post.PublishedAt.IsZero() {
		publishedAt := post.PublishedAt
		response.PublishedAt = &publishedAt
	}
	return response
}

func publicTerms(terms []store.Term) []termResponse {
	items := make([]termResponse, 0, len(terms))
	for _, term := range terms {
		items = append(items, termResponse{ID: term.ID, Name: term.Name, Slug: term.Slug, Kind: term.Kind, PostCount: term.PostCount})
	}
	return items
}

func publicTerm(term store.Term) termResponse {
	return termResponse{ID: term.ID, Name: term.Name, Slug: term.Slug, Kind: term.Kind, PostCount: term.PostCount}
}

func publicTermPage(page store.TermPage) termPageResponse {
	return termPageResponse{Items: publicTerms(page.Items), Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

func publicPostPage(page store.PostPage) postPageResponse {
	items := make([]postResponse, 0, len(page.Items))
	for _, post := range page.Items {
		items = append(items, publicPost(post, false))
	}
	return postPageResponse{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

func publicPage(page store.Page) pageResponse {
	response := pageResponse{ID: page.ID, Title: page.Title, Slug: page.Slug, Excerpt: page.Excerpt, Content: page.Content, Status: page.Status, UpdatedAt: page.UpdatedAt}
	if !page.PublishedAt.IsZero() {
		publishedAt := page.PublishedAt
		response.PublishedAt = &publishedAt
	}
	return response
}

func publicPagePage(page store.PagePage) pagePageResponse {
	items := make([]pageResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, publicPage(item))
	}
	return pagePageResponse{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

type commentResponse struct {
	ID          string    `json:"id"`
	PostID      string    `json:"postId,omitempty"`
	ParentID    string    `json:"parentId,omitempty"`
	PostTitle   string    `json:"postTitle,omitempty"`
	PostSlug    string    `json:"postSlug,omitempty"`
	AuthorName  string    `json:"authorName"`
	AuthorEmail string    `json:"authorEmail,omitempty"`
	AuthorURL   string    `json:"authorUrl,omitempty"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type commentPageResponse struct {
	Items    []commentResponse `json:"items"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
}

func publicComment(comment store.Comment, includePrivateFields bool) commentResponse {
	response := commentResponse{
		ID:         comment.ID,
		AuthorName: comment.AuthorName,
		ParentID:   comment.ParentID,
		PostTitle:  comment.PostTitle,
		PostSlug:   comment.PostSlug,
		AuthorURL:  comment.AuthorURL,
		Content:    comment.Content,
		Status:     comment.Status,
		CreatedAt:  comment.CreatedAt,
		UpdatedAt:  comment.UpdatedAt,
	}
	if includePrivateFields {
		response.PostID = comment.PostID
		response.AuthorEmail = comment.AuthorEmail
	}
	return response
}

func publicCommentPage(page store.CommentPage, includePrivateFields bool) commentPageResponse {
	items := make([]commentResponse, 0, len(page.Items))
	for _, comment := range page.Items {
		items = append(items, publicComment(comment, includePrivateFields))
	}
	return commentPageResponse{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

type mediaResponse struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"createdAt"`
}

type mediaPageResponse struct {
	Items    []mediaResponse `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

func publicMedia(media store.Media) mediaResponse {
	return mediaResponse{
		ID:          media.ID,
		Filename:    media.Filename,
		ContentType: media.ContentType,
		Size:        media.Size,
		URL:         "/media/" + media.ID,
		CreatedAt:   media.CreatedAt,
	}
}

func publicMediaPage(page store.MediaPage) mediaPageResponse {
	items := make([]mediaResponse, 0, len(page.Items))
	for _, media := range page.Items {
		items = append(items, publicMedia(media))
	}
	return mediaPageResponse{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

func cleanMediaFilename(raw string) string {
	filename := filepath.Base(strings.TrimSpace(raw))
	if filename == "." || filename == string(filepath.Separator) || filename == "" || strings.ContainsRune(filename, 0) {
		return ""
	}
	runes := []rune(filename)
	if len(runes) > 180 {
		filename = string(runes[:180])
	}
	return filename
}

func allowedMediaType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func mediaExtension(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func newMediaID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("media-%d", time.Now().UnixNano())
	}
	return "media-" + hex.EncodeToString(buffer)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("invalid JSON: request body must contain a single value")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeXML(w http.ResponseWriter, status int, contentType string, value any) {
	payload, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode_failed", "Unable to encode response")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = io.WriteString(w, xml.Header)
	_, _ = w.Write(payload)
	_, _ = io.WriteString(w, "\n")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": localize(message)}})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(payload []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(payload)
}

func requestLog(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "status", recorder.status, "duration_ms", time.Since(started).Milliseconds())
	})
}
