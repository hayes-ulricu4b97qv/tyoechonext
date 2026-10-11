package service

import (
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func (s *Service) RecordAudit(event store.AuditEvent) error {
	return s.repo.RecordAudit(event)
}

func (s *Service) ListAudit(action, resourceType string, page, pageSize int) (store.AuditPage, error) {
	return s.repo.ListAudit(store.AuditQuery{Action: action, ResourceType: resourceType, Page: page, PageSize: pageSize})
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

func (s *Service) RevokeUserSessions(username string) error {
	return s.repo.DeleteUserSessions(username)
}

func (s *Service) RevokeOtherSessions(username, token string) error {
	if token == "" {
		return s.repo.DeleteUserSessions(username)
	}
	return s.repo.DeleteOtherUserSessions(username, hashToken(token))
}

func (s *Service) PurgeSessions(now time.Time) error {
	return s.repo.PurgeSessions(now)
}
