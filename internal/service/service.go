package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrValidation         = errors.New("validation error")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,31}$`)

type SetupInput struct {
	SiteName      string
	SiteURL       string
	AdminUsername string
	AdminPassword string
	AdminEmail    string
	Timezone      string
}

type Service struct {
	repo store.Repository
}

func New(repo store.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Initialize(input SetupInput) (*store.Site, error) {
	if err := validateSetup(input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Timezone) == "" {
		input.Timezone = "Asia/Shanghai"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash administrator password: %w", err)
	}
	site := store.Site{
		Name:      strings.TrimSpace(input.SiteName),
		URL:       strings.TrimRight(strings.TrimSpace(input.SiteURL), "/"),
		AdminMail: strings.TrimSpace(input.AdminEmail),
		Timezone:  input.Timezone,
	}
	admin := store.Admin{
		Username:     input.AdminUsername,
		PasswordHash: string(hash),
		Email:        strings.TrimSpace(input.AdminEmail),
	}
	if err := s.repo.Setup(site, admin); err != nil {
		return nil, err
	}
	return &site, nil
}

func (s *Service) Authenticate(username, password string) (*store.Admin, error) {
	admin, err := s.repo.Admin()
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if admin.Username != username || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return admin, nil
}

func (s *Service) Admin() (*store.Admin, error) {
	return s.repo.Admin()
}

func (s *Service) Site() (*store.Site, error) {
	return s.repo.Site()
}

func (s *Service) UpdateSite(site store.Site) (*store.Site, error) {
	if strings.TrimSpace(site.Name) == "" || !validURL(site.URL) {
		return nil, fmt.Errorf("%w: a site name and valid URL are required", ErrValidation)
	}
	current, err := s.repo.Site()
	if err != nil {
		return nil, err
	}
	current.Name = strings.TrimSpace(site.Name)
	current.URL = strings.TrimRight(strings.TrimSpace(site.URL), "/")
	current.AdminMail = strings.TrimSpace(site.AdminMail)
	if strings.TrimSpace(site.Timezone) != "" {
		current.Timezone = strings.TrimSpace(site.Timezone)
	}
	if err := s.repo.UpdateSite(*current); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *Service) ListPosts(search string, includeDrafts bool, page, pageSize int) (store.PostPage, error) {
	return s.repo.SearchPosts(store.PostQuery{Search: search, IncludeDrafts: includeDrafts, Page: page, PageSize: pageSize})
}

func (s *Service) GetPost(key string, includeDraft bool) (store.Post, error) {
	post, err := s.repo.PostBySlug(key)
	if errors.Is(err, store.ErrNotFound) {
		post, err = s.repo.Post(key)
	}
	if err != nil {
		return store.Post{}, err
	}
	if post.Status != "published" && !includeDraft {
		return store.Post{}, store.ErrNotFound
	}
	return post, nil
}

func (s *Service) CreatePost(post store.Post) (store.Post, error) {
	if err := validatePost(post); err != nil {
		return store.Post{}, err
	}
	post.Title = strings.TrimSpace(post.Title)
	var err error
	post.Slug, err = s.uniqueSlug(post.Slug, post.Title, "")
	if err != nil {
		return store.Post{}, err
	}
	return s.repo.SavePost(post)
}

func (s *Service) UpdatePost(id string, input store.Post) (store.Post, error) {
	current, err := s.repo.Post(id)
	if err != nil {
		return store.Post{}, err
	}
	if err := validatePost(input); err != nil {
		return store.Post{}, err
	}
	input.ID = current.ID
	input.Title = strings.TrimSpace(input.Title)
	input.Slug, err = s.uniqueSlug(input.Slug, input.Title, input.ID)
	if err != nil {
		return store.Post{}, err
	}
	input.PublishedAt = current.PublishedAt
	return s.repo.SavePost(input)
}

func (s *Service) DeletePost(id string) error {
	return s.repo.DeletePost(id)
}

func (s *Service) Stats() (store.PostStats, error) {
	return s.repo.Stats()
}

func (s *Service) CreateSession(username, token string, expiresAt time.Time) error {
	return s.repo.CreateSession(username, hashToken(token), expiresAt)
}

func (s *Service) ResolveSession(token string, now time.Time) (string, error) {
	return s.repo.ResolveSession(hashToken(token), now)
}

func (s *Service) RevokeSession(token string) error {
	return s.repo.DeleteSession(hashToken(token))
}

func (s *Service) PurgeSessions(now time.Time) error {
	return s.repo.PurgeSessions(now)
}

func (s *Service) uniqueSlug(raw, title, currentID string) (string, error) {
	base := slugify(raw, title)
	existing, err := s.repo.PostBySlug(base)
	if errors.Is(err, store.ErrNotFound) || (err == nil && existing.ID == currentID) {
		return base, nil
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano()), nil
}

func validateSetup(input SetupInput) error {
	if strings.TrimSpace(input.SiteName) == "" || len(strings.TrimSpace(input.SiteName)) > 80 {
		return fmt.Errorf("%w: siteName must be between 1 and 80 characters", ErrValidation)
	}
	if !validURL(input.SiteURL) {
		return fmt.Errorf("%w: siteUrl must be a valid http or https URL", ErrValidation)
	}
	if !usernamePattern.MatchString(input.AdminUsername) {
		return fmt.Errorf("%w: adminUsername must be 3-32 characters and use letters, numbers, _ or -", ErrValidation)
	}
	if len(input.AdminPassword) < 8 {
		return fmt.Errorf("%w: adminPassword must be at least 8 characters", ErrValidation)
	}
	if strings.TrimSpace(input.AdminEmail) == "" || !strings.Contains(input.AdminEmail, "@") {
		return fmt.Errorf("%w: adminEmail must be a valid email address", ErrValidation)
	}
	return nil
}

func validatePost(post store.Post) error {
	if strings.TrimSpace(post.Title) == "" {
		return fmt.Errorf("%w: title is required", ErrValidation)
	}
	if post.Status != "" && post.Status != "draft" && post.Status != "published" {
		return fmt.Errorf("%w: status must be draft or published", ErrValidation)
	}
	return nil
}

func validURL(raw string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func slugify(raw, title string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(title))
	}
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return '-'
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		return fmt.Sprintf("post-%d", time.Now().UnixNano())
	}
	return value
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
