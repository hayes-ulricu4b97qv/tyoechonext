package service

import (
	"fmt"
	"strings"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func (s *Service) ListComments(postKey string, includePending bool, page, pageSize int) (store.CommentPage, error) {
	post, err := s.GetPost(postKey, includePending)
	if err != nil {
		return store.CommentPage{}, err
	}
	return s.repo.ListComments(store.CommentQuery{
		PostID:         post.ID,
		IncludePending: includePending,
		Page:           page,
		PageSize:       pageSize,
	})
}

func (s *Service) ListAllComments(status string, page, pageSize int) (store.CommentPage, error) {
	if status != "" && !validCommentStatus(status) {
		return store.CommentPage{}, fmt.Errorf("%w: status must be pending, approved or spam", ErrValidation)
	}
	return s.repo.ListComments(store.CommentQuery{Status: status, IncludePending: true, Page: page, PageSize: pageSize})
}

func (s *Service) CreateComment(postKey string, input CommentInput) (store.Comment, error) {
	post, err := s.GetPost(postKey, false)
	if err != nil {
		return store.Comment{}, err
	}
	name := strings.TrimSpace(input.AuthorName)
	email := strings.TrimSpace(input.AuthorEmail)
	content := strings.TrimSpace(input.Content)
	if runeLength(name) < 1 || runeLength(name) > 80 {
		return store.Comment{}, fmt.Errorf("%w: authorName must be between 1 and 80 characters", ErrValidation)
	}
	if email != "" && !validEmail(email) {
		return store.Comment{}, fmt.Errorf("%w: authorEmail must be a valid email address", ErrValidation)
	}
	if runeLength(content) < 1 || runeLength(content) > 4000 {
		return store.Comment{}, fmt.Errorf("%w: content must be between 1 and 4000 characters", ErrValidation)
	}
	return s.repo.SaveComment(store.Comment{
		PostID:      post.ID,
		AuthorName:  name,
		AuthorEmail: email,
		Content:     content,
		Status:      "pending",
	})
}

func (s *Service) ModerateComment(id, status string) (store.Comment, error) {
	if !validCommentStatus(status) {
		return store.Comment{}, fmt.Errorf("%w: status must be pending, approved or spam", ErrValidation)
	}
	return s.repo.UpdateCommentStatus(id, status)
}

func (s *Service) DeleteComment(id string) error {
	return s.repo.DeleteComment(id)
}

func (s *Service) ListMedia(page, pageSize int) (store.MediaPage, error) {
	return s.repo.ListMedia(store.MediaQuery{Page: page, PageSize: pageSize})
}

func (s *Service) GetMedia(id string) (store.Media, error) {
	return s.repo.Media(id)
}

func (s *Service) RegisterMedia(media store.Media) (store.Media, error) {
	media.Filename = strings.TrimSpace(media.Filename)
	media.ContentType = strings.TrimSpace(media.ContentType)
	if media.Filename == "" || media.ContentType == "" || media.Size < 1 || media.StorageKey == "" {
		return store.Media{}, fmt.Errorf("%w: media metadata is incomplete", ErrValidation)
	}
	return s.repo.SaveMedia(media)
}

func (s *Service) DeleteMedia(id string) error {
	return s.repo.DeleteMedia(id)
}
