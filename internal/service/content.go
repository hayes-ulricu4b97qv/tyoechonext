package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) ListPosts(search string, includeDrafts bool, page, pageSize int) (store.PostPage, error) {
	search, err := normalizeSearchQuery(search)
	if err != nil {
		return store.PostPage{}, err
	}
	return s.repo.SearchPosts(store.PostQuery{Search: search, IncludeDrafts: includeDrafts, Page: page, PageSize: pageSize})
}

func (s *Service) ListPostsWithContent(search string, includeDrafts bool, page, pageSize int) (store.PostPage, error) {
	search, err := normalizeSearchQuery(search)
	if err != nil {
		return store.PostPage{}, err
	}
	return s.repo.SearchPosts(store.PostQuery{Search: search, IncludeDrafts: includeDrafts, IncludeContent: true, Page: page, PageSize: pageSize})
}

func (s *Service) ListPostsByTerm(search, kind, slug string, includeDrafts bool, page, pageSize int) (store.PostPage, error) {
	search, err := normalizeSearchQuery(search)
	if err != nil {
		return store.PostPage{}, err
	}
	kind = strings.TrimSpace(kind)
	slug = strings.TrimSpace(slug)
	if (kind == "") != (slug == "") || (kind != "" && kind != "category" && kind != "tag") {
		return store.PostPage{}, fmt.Errorf("%w: 需要有效的分类或标签筛选", ErrValidation)
	}
	return s.repo.SearchPosts(store.PostQuery{Search: search, IncludeDrafts: includeDrafts, TermKind: kind, TermSlug: slug, Page: page, PageSize: pageSize})
}

func (s *Service) ListTerms(kind, search string, includeDrafts bool, page, pageSize int) (store.TermPage, error) {
	kind = strings.TrimSpace(kind)
	if kind != "" && kind != "category" && kind != "tag" {
		return store.TermPage{}, fmt.Errorf("%w: 类型只能是分类或标签", ErrValidation)
	}
	search, err := normalizeSearchQuery(search)
	if err != nil {
		return store.TermPage{}, err
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
	if !includeDraft && !visibleToPublic(post, time.Now().UTC()) {
		return store.Post{}, store.ErrNotFound
	}
	return post, nil
}

func visibleToPublic(post store.Post, now time.Time) bool {
	return visiblePublishedContentToPublic(post.Status, post.PublishedAt, now)
}

func visiblePageToPublic(page store.Page, now time.Time) bool {
	return visiblePublishedContentToPublic(page.Status, page.PublishedAt, now)
}

func visiblePublishedContentToPublic(status string, publishedAt, now time.Time) bool {
	if status != "published" || publishedAt.IsZero() {
		return false
	}
	return !publishedAt.After(now)
}

func (s *Service) RevealPost(post store.Post, editor bool, passphrase string) bool {
	if editor || post.PassphraseHash == "" {
		return true
	}
	return bcrypt.CompareHashAndPassword([]byte(post.PassphraseHash), []byte(passphrase)) == nil
}

func (s *Service) CreatePost(post store.Post) (store.Post, error) {
	if err := validatePost(post); err != nil {
		return store.Post{}, err
	}
	customFields, err := normalizeCustomFields(post.CustomFields)
	if err != nil {
		return store.Post{}, err
	}
	post.CustomFields = customFields
	post.SetCustomFields = true
	post.Title = strings.TrimSpace(post.Title)
	if err := normalizePostTerms(&post); err != nil {
		return store.Post{}, err
	}
	requestedPublish := post.RequestedPublishAt
	post.PublishedAt = time.Time{}
	if post.Status == "published" && !requestedPublish.IsZero() {
		post.PublishedAt = requestedPublish.UTC()
	}
	if err := hashPassphrase(&post); err != nil {
		return store.Post{}, err
	}
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
	if !input.SetCustomFields {
		input.CustomFields = current.CustomFields
	}
	input.CustomFields, err = normalizeCustomFields(input.CustomFields)
	if err != nil {
		return store.Post{}, err
	}
	input.SetCustomFields = true
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
	if !current.PublishedAt.IsZero() {
		input.PublishedAt = current.PublishedAt
	} else if input.Status == "published" && !input.RequestedPublishAt.IsZero() {
		input.PublishedAt = input.RequestedPublishAt.UTC()
	} else {
		input.PublishedAt = time.Time{}
	}
	if !input.SetPassphrase {
		input.PassphraseHash = current.PassphraseHash
	}
	if err := hashPassphrase(&input); err != nil {
		return store.Post{}, err
	}
	return s.repo.SavePost(input)
}

func hashPassphrase(post *store.Post) error {
	if !post.SetPassphrase {
		return nil
	}
	plain := post.PassphrasePlain
	post.PassphrasePlain = ""
	if plain == "" {
		post.PassphraseHash = ""
		return nil
	}
	if err := validateBcryptInput(plain, "阅读口令"); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	post.PassphraseHash = string(hash)
	return nil
}

func (s *Service) ListPostRevisions(postID string) ([]store.PostRevision, error) {
	if _, err := s.repo.Post(postID); err != nil {
		return nil, err
	}
	return s.repo.ListPostRevisions(postID)
}

func (s *Service) RestorePostRevision(postID, revisionID string) (store.Post, error) {
	current, err := s.repo.Post(postID)
	if err != nil {
		return store.Post{}, err
	}
	revision, err := s.repo.PostRevision(revisionID)
	if err != nil {
		return store.Post{}, err
	}
	if revision.PostID != current.ID {
		return store.Post{}, store.ErrNotFound
	}
	current.Title = revision.Title
	current.Excerpt = revision.Excerpt
	current.Content = revision.Content
	current.CustomFields = revision.CustomFields
	current.SetCustomFields = true
	current.SetPassphrase = false
	if err := validatePost(current); err != nil {
		return store.Post{}, err
	}
	return s.repo.SavePost(current)
}

func (s *Service) DeletePost(id string) error {
	return s.repo.DeletePost(id)
}

func (s *Service) ListPages(search string, includeDrafts bool, page, pageSize int) (store.PagePage, error) {
	search, err := normalizeSearchQuery(search)
	if err != nil {
		return store.PagePage{}, err
	}
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
	if !includeDraft && !visiblePageToPublic(page, time.Now().UTC()) {
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
