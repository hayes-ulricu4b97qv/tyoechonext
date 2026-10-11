package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func (s *Service) ListPosts(search string, includeDrafts bool, page, pageSize int) (store.PostPage, error) {
	return s.repo.SearchPosts(store.PostQuery{Search: search, IncludeDrafts: includeDrafts, Page: page, PageSize: pageSize})
}

func (s *Service) ListPostsByTerm(search, kind, slug string, includeDrafts bool, page, pageSize int) (store.PostPage, error) {
	kind = strings.TrimSpace(kind)
	slug = strings.TrimSpace(slug)
	if (kind == "") != (slug == "") || (kind != "" && kind != "category" && kind != "tag") {
		return store.PostPage{}, fmt.Errorf("%w: a valid category or tag filter is required", ErrValidation)
	}
	return s.repo.SearchPosts(store.PostQuery{Search: search, IncludeDrafts: includeDrafts, TermKind: kind, TermSlug: slug, Page: page, PageSize: pageSize})
}

func (s *Service) ListTerms(kind, search string, includeDrafts bool, page, pageSize int) (store.TermPage, error) {
	kind = strings.TrimSpace(kind)
	if kind != "" && kind != "category" && kind != "tag" {
		return store.TermPage{}, fmt.Errorf("%w: kind must be category or tag", ErrValidation)
	}
	return s.repo.ListTerms(store.TermQuery{Kind: kind, Search: search, IncludeDrafts: includeDrafts, Page: page, PageSize: pageSize})
}

func (s *Service) CreateTerm(input TermInput) (*store.Term, error) {
	if err := validateTermInput(input); err != nil {
		return nil, err
	}
	term, err := s.repo.SaveTerm(store.Term{Name: strings.TrimSpace(input.Name), Kind: strings.TrimSpace(input.Kind)})
	if err != nil {
		return nil, err
	}
	return &term, nil
}

func (s *Service) UpdateTerm(id string, input TermInput) (*store.Term, error) {
	if err := validateTermInput(input); err != nil {
		return nil, err
	}
	term, err := s.repo.SaveTerm(store.Term{ID: id, Name: strings.TrimSpace(input.Name), Kind: strings.TrimSpace(input.Kind)})
	if err != nil {
		return nil, err
	}
	return &term, nil
}

func (s *Service) DeleteTerm(id string) error {
	return s.repo.DeleteTerm(id)
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
	if err := normalizePostTerms(&post); err != nil {
		return store.Post{}, err
	}
	post.PublishedAt = time.Time{}
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
	if err := normalizePostTerms(&input); err != nil {
		return store.Post{}, err
	}
	requestedSlug := strings.TrimSpace(input.Slug)
	if requestedSlug == "" {
		requestedSlug = current.Slug
	}
	input.Slug, err = s.uniqueSlug(requestedSlug, input.Title, input.ID)
	if err != nil {
		return store.Post{}, err
	}
	input.PublishedAt = current.PublishedAt
	return s.repo.SavePost(input)
}

func (s *Service) DeletePost(id string) error {
	return s.repo.DeletePost(id)
}

func (s *Service) ListPages(search string, includeDrafts bool, page, pageSize int) (store.PagePage, error) {
	return s.repo.SearchPages(store.PageQuery{Search: search, IncludeDrafts: includeDrafts, Page: page, PageSize: pageSize})
}

func (s *Service) GetPage(key string, includeDraft bool) (store.Page, error) {
	page, err := s.repo.PageBySlug(key)
	if errors.Is(err, store.ErrNotFound) {
		page, err = s.repo.Page(key)
	}
	if err != nil {
		return store.Page{}, err
	}
	if page.Status != "published" && !includeDraft {
		return store.Page{}, store.ErrNotFound
	}
	return page, nil
}

func (s *Service) CreatePage(page store.Page) (store.Page, error) {
	if err := validatePage(page); err != nil {
		return store.Page{}, err
	}
	page.Title = strings.TrimSpace(page.Title)
	var err error
	page.Slug, err = s.uniquePageSlug(page.Slug, page.Title, "")
	if err != nil {
		return store.Page{}, err
	}
	return s.repo.SavePage(page)
}

func (s *Service) UpdatePage(id string, input store.Page) (store.Page, error) {
	current, err := s.repo.Page(id)
	if err != nil {
		return store.Page{}, err
	}
	if err := validatePage(input); err != nil {
		return store.Page{}, err
	}
	input.ID = current.ID
	input.Title = strings.TrimSpace(input.Title)
	requestedSlug := strings.TrimSpace(input.Slug)
	if requestedSlug == "" {
		requestedSlug = current.Slug
	}
	input.Slug, err = s.uniquePageSlug(requestedSlug, input.Title, input.ID)
	if err != nil {
		return store.Page{}, err
	}
	input.PublishedAt = current.PublishedAt
	return s.repo.SavePage(input)
}

func (s *Service) DeletePage(id string) error {
	return s.repo.DeletePage(id)
}
