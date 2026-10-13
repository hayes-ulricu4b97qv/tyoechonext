package service

import (
	"context"
	"errors"
	"log/slog"
	"regexp"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
	_ "time/tzdata"
)

// MaxContentRunes is the documented maximum length of a post or page body.
const MaxContentRunes = 500000

// MaxSearchRunes bounds work for user-controlled content search queries.
const MaxSearchRunes = 200

// maxBcryptInputBytes is bcrypt's maximum supported password input size.
const maxBcryptInputBytes = 72

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

type AccountInput struct {
	Email           string
	CurrentPassword string
	NewPassword     string
	SessionToken    string
}

type CommentInput struct {
	AuthorName  string
	AuthorEmail string
	AuthorURL   string
	Content     string
	ParentID    string
}

type UserInput struct {
	Username string
	Email    string
	Password string
	Role     string
	Active   *bool
}

type TermInput struct {
	Name string
	Kind string
}

type Service struct {
	repo   store.Repository
	mailer Mailer
	logger *slog.Logger
}

type Mailer interface {
	Send(context.Context, string, string, string) error
}

func New(repo store.Repository, logger *slog.Logger, mailer Mailer) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, logger: logger, mailer: mailer}
}
