package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func (s *Service) uniqueSlug(raw, title, currentID string) (string, error) {
	base := limitSlug(slugify(raw, title))
	existing, err := s.repo.PostBySlug(base)
	if errors.Is(err, store.ErrNotFound) || (err == nil && existing.ID == currentID) {
		return base, nil
	}
	if err != nil {
		return "", err
	}
	for suffix := 2; suffix < 10000; suffix++ {
		candidate := slugWithSuffix(base, suffix)
		existing, err := s.repo.PostBySlug(candidate)
		if errors.Is(err, store.ErrNotFound) || (err == nil && existing.ID == currentID) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("%w: unable to allocate a unique slug", ErrValidation)
}

func (s *Service) uniquePageSlug(raw, title, currentID string) (string, error) {
	base := limitSlug(slugify(raw, title))
	existing, err := s.repo.PageBySlug(base)
	if errors.Is(err, store.ErrNotFound) || (err == nil && existing.ID == currentID) {
		return base, nil
	}
	if err != nil {
		return "", err
	}
	for suffix := 2; suffix < 10000; suffix++ {
		candidate := slugWithSuffix(base, suffix)
		existing, err := s.repo.PageBySlug(candidate)
		if errors.Is(err, store.ErrNotFound) || (err == nil && existing.ID == currentID) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("%w: unable to allocate a unique page slug", ErrValidation)
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
	if !validEmail(input.AdminEmail) {
		return fmt.Errorf("%w: adminEmail must be a valid email address", ErrValidation)
	}
	return nil
}

func validatePost(post store.Post) error {
	if err := validateContentFields(post.Title, post.Slug, post.Excerpt, post.Content); err != nil {
		return err
	}
	if post.Status != "" && post.Status != "draft" && post.Status != "published" {
		return fmt.Errorf("%w: status must be draft or published", ErrValidation)
	}
	if err := validateTermNames(post.Categories, "category"); err != nil {
		return err
	}
	if err := validateTermNames(post.Tags, "tag"); err != nil {
		return err
	}
	return nil
}

func validatePage(page store.Page) error {
	if err := validateContentFields(page.Title, page.Slug, page.Excerpt, page.Content); err != nil {
		return err
	}
	if page.Status != "" && page.Status != "draft" && page.Status != "published" {
		return fmt.Errorf("%w: status must be draft or published", ErrValidation)
	}
	return nil
}

func validateContentFields(title, slug, excerpt, content string) error {
	titleLength := runeLength(strings.TrimSpace(title))
	if titleLength < 1 || titleLength > 200 {
		return fmt.Errorf("%w: title must be between 1 and 200 characters", ErrValidation)
	}
	if runeLength(strings.TrimSpace(slug)) > 120 {
		return fmt.Errorf("%w: slug must be at most 120 characters", ErrValidation)
	}
	if runeLength(excerpt) > 500 {
		return fmt.Errorf("%w: excerpt must be at most 500 characters", ErrValidation)
	}
	if runeLength(content) > MaxContentRunes {
		return fmt.Errorf("%w: content must be at most %d characters", ErrValidation, MaxContentRunes)
	}
	return nil
}

func normalizePostTerms(post *store.Post) error {
	post.Categories = normalizeTerms(post.Categories, "category")
	post.Tags = normalizeTerms(post.Tags, "tag")
	return nil
}

func normalizeTerms(input []store.Term, kind string) []store.Term {
	seen := make(map[string]struct{}, len(input))
	terms := make([]store.Term, 0, len(input))
	for _, term := range input {
		name := strings.TrimSpace(term.Name)
		key := strings.ToLower(name)
		if name == "" || key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		terms = append(terms, store.Term{Name: name, Kind: kind})
	}
	return terms
}

func validateTermNames(terms []store.Term, kind string) error {
	if len(terms) > 20 {
		return fmt.Errorf("%w: at most 20 %s terms are allowed", ErrValidation, kind)
	}
	for _, term := range terms {
		if runeLength(strings.TrimSpace(term.Name)) < 1 || runeLength(strings.TrimSpace(term.Name)) > 60 {
			return fmt.Errorf("%w: %s names must be between 1 and 60 characters", ErrValidation, kind)
		}
	}
	return nil
}

func validateTermInput(input TermInput) error {
	kind := strings.TrimSpace(input.Kind)
	name := strings.TrimSpace(input.Name)
	if kind != "category" && kind != "tag" {
		return fmt.Errorf("%w: kind must be category or tag", ErrValidation)
	}
	if runeLength(name) < 1 || runeLength(name) > 60 {
		return fmt.Errorf("%w: term name must be between 1 and 60 characters", ErrValidation)
	}
	return nil
}

func validCommentStatus(status string) bool {
	return status == "pending" || status == "approved" || status == "spam"
}

func validateUserInput(input UserInput, creating bool) error {
	if creating && !usernamePattern.MatchString(input.Username) {
		return fmt.Errorf("%w: username must be 3-32 characters and use letters, numbers, _ or -", ErrValidation)
	}
	if !validEmail(input.Email) {
		return fmt.Errorf("%w: email must be a valid email address", ErrValidation)
	}
	if creating && len(input.Password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrValidation)
	}
	if !validStaffRole(input.Role) {
		return fmt.Errorf("%w: role must be editor or moderator", ErrValidation)
	}
	return nil
}

func validStaffRole(role string) bool {
	return role == "editor" || role == "moderator"
}

func runeLength(value string) int {
	return len([]rune(value))
}

func validURL(raw string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func validEmail(raw string) bool {
	parsed, err := mail.ParseAddress(strings.TrimSpace(raw))
	return err == nil && parsed.Address == strings.TrimSpace(raw) && strings.Contains(parsed.Address, "@")
}

func slugify(raw, title string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = strings.TrimSpace(title)
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '-' {
			return unicode.ToLower(r)
		}
		return '-'
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		return fmt.Sprintf("post-%d", time.Now().UnixNano())
	}
	return value
}

func limitSlug(value string) string {
	runes := []rune(strings.Trim(strings.TrimSpace(value), "-"))
	if len(runes) > 120 {
		runes = runes[:120]
	}
	return strings.Trim(string(runes), "-")
}

func slugWithSuffix(base string, suffix int) string {
	suffixText := fmt.Sprintf("-%d", suffix)
	baseRunes := []rune(strings.Trim(base, "-"))
	maxBaseLength := 120 - len([]rune(suffixText))
	if len(baseRunes) > maxBaseLength {
		baseRunes = baseRunes[:maxBaseLength]
	}
	baseValue := strings.Trim(string(baseRunes), "-")
	if baseValue == "" {
		baseValue = "post"
	}
	return baseValue + suffixText
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
