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
		return nil, fmt.Errorf("%w: 每页文章数必须是 1 到 50", ErrValidation)
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
			return 0, fmt.Errorf("%w: 评论线程无效", ErrValidation)
		}
		seen[current.ParentID] = struct{}{}
		parent, err := lookup(current.ParentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return 0, fmt.Errorf("%w: 评论线程无效", ErrValidation)
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
		return store.Comment{}, fmt.Errorf("%w: 称呼需要 1 到 80 个字符", ErrValidation)
	}
	if email != "" && !validEmail(email) {
		return store.Comment{}, fmt.Errorf("%w: 邮箱格式不正确", ErrValidation)
	}
	if authorURL != "" && (runeLength(authorURL) > 200 || !validURL(authorURL)) {
		return store.Comment{}, fmt.Errorf("%w: 网址必须是不超过 200 个字符的 http 或 https 地址", ErrValidation)
	}
	if runeLength(content) < 1 || runeLength(content) > 4000 {
		return store.Comment{}, fmt.Errorf("%w: 评论需要 1 到 4000 个字符", ErrValidation)
	}
	if !site.CommentsEnabled || !post.AllowComment {
		return store.Comment{}, fmt.Errorf("%w: 评论已关闭", ErrValidation)
	}
	parentID := ""
	if parent != nil {
		if !site.CommentsThreaded {
			return store.Comment{}, fmt.Errorf("%w: 已关闭回复", ErrValidation)
		}
		if parent.PostID != post.ID || parent.Status != "approved" {
			return store.Comment{}, fmt.Errorf("%w: 回复必须指向同一篇文章里已通过的评论", ErrValidation)
		}
		if parentDepth >= maxCommentDepth-1 {
			return store.Comment{}, fmt.Errorf("%w: 回复层级过多", ErrValidation)
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
