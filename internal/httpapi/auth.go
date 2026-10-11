package httpapi

import (
	"errors"
	"net/http"

	"github.com/untesheazulchunghu-beep/typecho/internal/service"
)

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
	// The setup request has no session cookie yet, so keep the actor identity in details.
	s.recordAudit(r, "site.initialized", "site", "1", map[string]string{"owner": request.AdminUsername})
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
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(admin)})
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
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(admin)})
}
