package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
	"golang.org/x/crypto/bcrypt"
)

var ErrMailUnavailable = errors.New("SMTP mail is not configured")

func (s *Service) ListOutboundMail(page, pageSize int) (store.OutboundMailPage, error) {
	return s.repo.ListOutboundMail(page, pageSize)
}

func (s *Service) ListOutboundMailContext(ctx context.Context, page, pageSize int) (store.OutboundMailPage, error) {
	return s.repo.ListOutboundMailContext(ctx, page, pageSize)
}

// MailAvailable reports whether the service has a configured mailer.
func (s *Service) MailAvailable() bool {
	return s.mailer != nil
}

func (s *Service) notifyOwner(site *store.Site, subject, body string) {
	if site == nil {
		return
	}
	recipient := strings.TrimSpace(site.AdminMail)
	if recipient == "" {
		return
	}
	if err := s.repo.SaveOutboundMail(recipient, subject, body); err != nil {
		s.logger.Warn("save owner notification failed", "subject", subject, "error", err)
	}
	if s.mailer != nil {
		if err := s.mailer.Send(context.Background(), recipient, subject, body); err != nil {
			s.logger.Warn("deliver owner notification failed", "subject", subject, "error", err)
		}
	}
}

func (s *Service) RequestPasswordReset(ctx context.Context, username string) error {
	if s.mailer == nil {
		return ErrMailUnavailable
	}
	user, err := s.repo.UserContext(ctx, strings.TrimSpace(username))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !user.Active {
		return nil
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	expires := time.Now().UTC().Add(30 * time.Minute)
	if err := s.repo.SavePasswordResetContext(ctx, user.Username, store.HashResetToken(token), expires); err != nil {
		return err
	}
	site, err := s.repo.SiteContext(ctx)
	if err != nil {
		_ = s.repo.DeletePasswordReset(store.HashResetToken(token))
		return err
	}
	resetURL := "/admin"
	if site != nil && strings.TrimSpace(site.URL) != "" {
		resetURL = strings.TrimRight(strings.TrimSpace(site.URL), "/") + "/admin"
	}
	body := fmt.Sprintf("你好 %s，\n\n请在 30 分钟内使用下面的重置口令设置新密码：\n重置口令：%s\n\n管理页面：%s\n如果这不是你发起的请求，请忽略本邮件。\n", user.Username, token, resetURL)
	if err := s.mailer.Send(ctx, user.Email, "重置 Typecho Next 密码", body); err != nil {
		if cleanupErr := s.repo.DeletePasswordReset(store.HashResetToken(token)); cleanupErr != nil {
			return errors.Join(fmt.Errorf("send reset email: %w", err), cleanupErr)
		}
		return fmt.Errorf("send reset email: %w", err)
	}
	return nil
}

func (s *Service) ResetPassword(token, newPassword string) error {
	return s.ResetPasswordContext(context.Background(), token, newPassword)
}

func (s *Service) ResetPasswordContext(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("%w: 密码至少 8 个字符", ErrValidation)
	}
	if err := validateBcryptInput(newPassword, "密码"); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tokenHash := store.HashResetToken(strings.TrimSpace(token))
	if err := s.repo.CheckPasswordResetContext(ctx, tokenHash, time.Now().UTC()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.repo.CompletePasswordReset(ctx, tokenHash, time.Now().UTC(), string(hash))
}
