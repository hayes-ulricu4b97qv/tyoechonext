package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

const maxCommentDepth = 3

type ReadingInput struct {
	PostsPerPage              int
	CommentsEnabled           bool
	CommentsRequireModeration bool
	CommentsThreaded          bool
}

func (s *Service) UpdateReading(input ReadingInput) (*store.Site, error) {
	if input.PostsPerPage < 1 || input.PostsPerPage > 50 {
		return nil, fmt.Errorf("%w: postsPerPage must be between 1 and 50", ErrValidation)
	}
	current, err := s.repo.Site()
	if err != nil {
		return nil, err
	}
	current.PostsPerPage = input.PostsPerPage
	current.CommentsEnabled = input.CommentsEnabled
	current.CommentsRequireModeration = input.CommentsRequireModeration
	current.CommentsThreaded = input.CommentsThreaded
	if err := s.repo.UpdateSite(*current); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *Service) commentDepth(comment store.Comment) (int, error) {
	return commentDepthFrom(comment, s.repo.Comment)
}

func commentDepthFrom(comment store.Comment, lookup func(string) (store.Comment, error)) (int, error) {
	depth := 0
	seen := map[string]struct{}{comment.ID: {}}
	current := comment
	for current.ParentID != "" && depth <= maxCommentDepth {
		if _, ok := seen[current.ParentID]; ok {
			return 0, fmt.Errorf("%w: comment thread is invalid", ErrValidation)
		}
		seen[current.ParentID] = struct{}{}
		parent, err := lookup(current.ParentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return 0, fmt.Errorf("%w: comment thread is invalid", ErrValidation)
			}
			return 0, err
		}
		depth++
		current = parent
	}
	return depth, nil
}

func buildComment(site store.Site, post store.Post, input CommentInput, parent *store.Comment, parentDepth int) (store.Comment, error) {
	name := strings.TrimSpace(input.AuthorName)
	email := strings.TrimSpace(input.AuthorEmail)
	authorURL := strings.TrimSpace(input.AuthorURL)
	content := strings.TrimSpace(input.Content)
	if runeLength(name) < 1 || runeLength(name) > 80 {
		return store.Comment{}, fmt.Errorf("%w: authorName must be between 1 and 80 characters", ErrValidation)
	}
	if email != "" && !validEmail(email) {
		return store.Comment{}, fmt.Errorf("%w: authorEmail must be a valid email address", ErrValidation)
	}
	if authorURL != "" && (runeLength(authorURL) > 200 || !validURL(authorURL)) {
		return store.Comment{}, fmt.Errorf("%w: authorUrl must be an http or https URL up to 200 characters", ErrValidation)
	}
	if runeLength(content) < 1 || runeLength(content) > 4000 {
		return store.Comment{}, fmt.Errorf("%w: content must be between 1 and 4000 characters", ErrValidation)
	}
	if !site.CommentsEnabled || !post.AllowComment {
		return store.Comment{}, fmt.Errorf("%w: comments are closed", ErrValidation)
	}
	parentID := ""
	if parent != nil {
		if !site.CommentsThreaded {
			return store.Comment{}, fmt.Errorf("%w: comment replies are disabled", ErrValidation)
		}
		if parent.PostID != post.ID || parent.Status != "approved" {
			return store.Comment{}, fmt.Errorf("%w: replies must target an approved comment on the same post", ErrValidation)
		}
		if parentDepth >= maxCommentDepth-1 {
			return store.Comment{}, fmt.Errorf("%w: comment thread is too deep", ErrValidation)
		}
		parentID = parent.ID
	}
	status := "pending"
	if !site.CommentsRequireModeration {
		status = "approved"
	}
	return store.Comment{
		PostID:      post.ID,
		ParentID:    parentID,
		AuthorName:  name,
		AuthorEmail: email,
		AuthorURL:   authorURL,
		Content:     content,
		Status:      status,
	}, nil
}
