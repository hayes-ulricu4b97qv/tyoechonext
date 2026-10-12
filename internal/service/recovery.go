package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) notifyOwner(site *store.Site, subject, body string) error {
	if site == nil {
		return nil
	}
	recipient := strings.TrimSpace(site.AdminMail)
	if recipient == "" {
		return nil
	}
	return s.repo.SaveOutboundMail(recipient, subject, body)
}

func (s *Service) RequestPasswordReset(username string) error {
	user, err := s.repo.User(strings.TrimSpace(username))
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
	if err := s.repo.SavePasswordReset(user.Username, store.HashResetToken(token), expires); err != nil {
		return err
	}
	site, err := s.repo.Site()
	if err != nil {
		return err
	}
	body := fmt.Sprintf("用户 %s 请求重置密码。\n重置口令：%s\n", user.Username, token)
	return s.notifyOwner(site, "重置密码", body)
}

func (s *Service) ResetPassword(token, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("%w: 密码至少 8 个字符", ErrValidation)
	}
	username, err := s.repo.ConsumePasswordReset(store.HashResetToken(strings.TrimSpace(token)), time.Now().UTC())
	if err != nil {
		return err
	}
	user, err := s.repo.User(username)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hash)
	if _, err := s.repo.UpdateUser(user); err != nil {
		return err
	}
	return s.RevokeUserSessions(username)
}
