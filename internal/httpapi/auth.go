package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/service"
	"github.com/untesheazulchunghu-beep/typecho/internal/store"
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
	initialized, err := s.repo.InitializedContext(r.Context())
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		s.writeRepositoryUnavailable(w, r, err)
		return
	}
	if initialized {
		writeError(w, http.StatusConflict, "already_initialized", "The application has already been initialized")
		return
	}
	var request setupRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	setup, err := s.app.InitializeContext(r.Context(), service.SetupInput{
		SiteName:      request.SiteName,
		SiteURL:       request.SiteURL,
		AdminUsername: request.AdminUsername,
		AdminPassword: request.AdminPassword,
		AdminEmail:    request.AdminEmail,
		Timezone:      request.Timezone,
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		status := http.StatusConflict
		code := "setup_failed"
		if errors.Is(err, service.ErrValidation) {
			status = http.StatusUnprocessableEntity
			code = "validation_error"
		}
		writeError(w, status, code, err.Error())
		return
	}
	if err := s.issueSessionContext(r.Context(), w, request.AdminUsername); err != nil {
		if r.Context().Err() != nil {
			return
		}
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
	initialized, err := s.repo.InitializedContext(r.Context())
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		s.writeRepositoryUnavailable(w, r, err)
		return
	}
	if !initialized {
		writeError(w, http.StatusPreconditionRequired, "setup_required", "Complete the initial setup at /setting")
		return
	}
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	key := s.clientHost(r)
	window := 10 * time.Minute
	limit := s.loginFailureLimit()
	if !s.loginFailures.Allow(key, limit, window) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "尝试过于频繁，请稍后再试")
		return
	}
	admin, err := s.app.AuthenticateContext(r.Context(), request.Username, request.Password)
	if err != nil && r.Context().Err() != nil {
		return
	}
	if err != nil && !errors.Is(err, service.ErrInvalidCredentials) {
		s.writeRepositoryUnavailable(w, r, err)
		return
	}
	if errors.Is(err, service.ErrInvalidCredentials) || admin == nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Username or password is incorrect")
		return
	}
	s.loginFailures.Reset(key)
	if err := s.issueSessionContext(r.Context(), w, admin.Username); err != nil {
		if r.Context().Err() != nil {
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "Username or password is incorrect")
			return
		}
		writeError(w, http.StatusInternalServerError, "session_failed", "Unable to create a sign-in session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(admin)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var revokeErr error
	if cookie, err := r.Cookie(cookieName); err == nil {
		revokeErr = s.app.RevokeSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.settings.CookieSecure})
	if revokeErr != nil {
		s.logger.Error("unable to revoke sign-in session during logout", "error", revokeErr)
		writeError(w, http.StatusInternalServerError, "session_revoke_failed", "Unable to revoke sign-in session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	admin, err := s.authenticatedAdmin(r.Context(), r)
	if r.Context().Err() != nil {
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sign in to continue")
		return
	}
	if err != nil {
		s.writeRepositoryUnavailable(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(admin)})
}
