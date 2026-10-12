package service

import (
	"errors"
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

func (s *Service) RecentApprovedComments(page, pageSize int) (store.CommentPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	} else if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.ListComments(store.CommentQuery{
		Status:             "approved",
		PublishedPostsOnly: true,
		Page:               page,
		PageSize:           pageSize,
	})
}

func (s *Service) ListAllComments(status string, page, pageSize int) (store.CommentPage, error) {
	if status != "" && !validCommentStatus(status) {
		return store.CommentPage{}, fmt.Errorf("%w: 评论状态只能是待审核、已通过或垃圾", ErrValidation)
	}
	return s.repo.ListComments(store.CommentQuery{Status: status, IncludePending: true, Page: page, PageSize: pageSize})
}

func (s *Service) CreateComment(postKey string, input CommentInput) (store.Comment, error) {
	post, err := s.GetPost(postKey, false)
	if err != nil {
		return store.Comment{}, err
	}
	site, err := s.repo.Site()
	if err != nil {
		return store.Comment{}, err
	}
	var parent *store.Comment
	parentDepth := 0
	if parentID := strings.TrimSpace(input.ParentID); parentID != "" {
		found, err := s.repo.Comment(parentID)
		if errors.Is(err, store.ErrNotFound) {
			return store.Comment{}, fmt.Errorf("%w: 未找到要回复的评论", ErrValidation)
		}
		if err != nil {
			return store.Comment{}, err
		}
		parent = &found
		parentDepth, err = s.commentDepth(found)
		if err != nil {
			return store.Comment{}, err
		}
	}
	comment, err := buildComment(*site, post, input, parent, parentDepth)
	if err != nil {
		return store.Comment{}, err
	}
	saved, err := s.repo.SaveComment(comment)
	if err != nil {
		return store.Comment{}, err
	}
	if saved.Status == "pending" {
		body := fmt.Sprintf("文章：%s\n作者：%s\n内容：%s\n", post.Title, saved.AuthorName, saved.Content)
		if err := s.notifyOwner(site, "新评论待审核", body); err != nil {
			return store.Comment{}, err
		}
	}
	return saved, nil
}

func (s *Service) ModerateComment(id, status string) (store.Comment, error) {
	if !validCommentStatus(status) {
		return store.Comment{}, fmt.Errorf("%w: 评论状态只能是待审核、已通过或垃圾", ErrValidation)
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
		return store.Media{}, fmt.Errorf("%w: 媒体信息不完整", ErrValidation)
	}
	return s.repo.SaveMedia(media)
}

func (s *Service) DeleteMedia(id string) error {
	return s.repo.DeleteMedia(id)
}
