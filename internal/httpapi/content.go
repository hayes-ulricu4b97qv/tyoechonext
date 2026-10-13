package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/untesheazulchunghu-beep/typecho/internal/service"
	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := s.readingPage(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	includeDrafts := r.URL.Query().Get("includeDraft") == "true"
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	if (includeDrafts || statusFilter == "draft") && !s.requireContentEditor(w, r) {
		return
	}
	if statusFilter == "draft" {
		includeDrafts = true
	}
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	posts, err := s.app.ListPostsByTerms(r.URL.Query().Get("q"), category, tag, r.URL.Query().Get("month"), r.URL.Query().Get("order"), includeDrafts, statusFilter, page, pageSize)
	if err != nil {
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicPostPage(posts))
}

func (s *Server) listArchive(w http.ResponseWriter, _ *http.Request) {
	items, err := s.app.MonthArchive()
	if err != nil {
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	if items == nil {
		items = []store.MonthCount{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listRecentComments(w http.ResponseWriter, r *http.Request) {
	page, pageSize, ok := s.pages(w, r)
	if !ok {
		return
	}
	comments, err := s.app.RecentApprovedComments(page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read recent comments")
		return
	}
	writeJSON(w, http.StatusOK, publicCommentPage(comments, false))
}

func (s *Server) listPages(w http.ResponseWriter, r *http.Request) {
	includeDrafts := r.URL.Query().Get("includeDraft") == "true"
	if includeDrafts && !s.requireContentEditor(w, r) {
		return
	}
	page, pageSize, ok := s.pages(w, r)
	if !ok {
		return
	}
	pages, err := s.app.ListPages(r.URL.Query().Get("q"), includeDrafts, page, pageSize)
	if err != nil {
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicPagePage(pages))
}

func (s *Server) listTerms(w http.ResponseWriter, r *http.Request) {
	includeDrafts := r.URL.Query().Get("includeDraft") == "true"
	if includeDrafts && !s.requireContentEditor(w, r) {
		return
	}
	page, pageSize, ok := s.pages(w, r)
	if !ok {
		return
	}
	terms, err := s.app.ListTerms(r.URL.Query().Get("kind"), r.URL.Query().Get("q"), includeDrafts, page, pageSize)
	if err != nil {
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicTermPage(terms))
}

type termRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func (s *Server) createTerm(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	var input termRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	term, err := s.app.CreateTerm(service.TermInput{Name: input.Name, Kind: input.Kind})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "term_taken", "已有同名分类或标签")
			return
		}
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "term.created", "term", term.ID, map[string]string{"name": term.Name, "kind": term.Kind})
	writeJSON(w, http.StatusCreated, publicTerm(*term))
}

func (s *Server) updateTerm(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	var input termRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	term, err := s.app.UpdateTerm(r.PathValue("id"), service.TermInput{Name: input.Name, Kind: input.Kind})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "term_taken", "已有同名分类或标签")
			return
		}
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "term.updated", "term", term.ID, map[string]string{"name": term.Name, "kind": term.Kind})
	writeJSON(w, http.StatusOK, publicTerm(*term))
}

func (s *Server) deleteTerm(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	if err := s.app.DeleteTerm(r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Term not found")
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
	} else {
		s.recordAudit(r, "term.deleted", "term", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	post, err := s.app.GetPost(slug, s.canEditContent(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read post")
		return
	}
	previous, next, err := s.app.PublishedNeighbors(post)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read neighboring posts")
		return
	}
	withBody := s.app.RevealPost(post, s.canEditContent(r), r.Header.Get("X-Post-Passphrase"))
	writeJSON(w, http.StatusOK, publicPostWithNeighbors(post, previous, next, withBody))
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	page, err := s.app.GetPage(r.PathValue("slug"), s.canEditContent(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Page not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read page")
		return
	}
	writeJSON(w, http.StatusOK, publicPage(page))
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	page, pageSize, ok := s.pages(w, r)
	if !ok {
		return
	}
	includePending := r.URL.Query().Get("includePending") == "true" && s.canModerate(r)
	comments, err := s.app.ListComments(r.PathValue("slug"), includePending, page, pageSize)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read comments")
		return
	}
	writeJSON(w, http.StatusOK, publicCommentPage(comments, false))
}

type commentRequest struct {
	AuthorName  string `json:"authorName"`
	AuthorEmail string `json:"authorEmail"`
	AuthorURL   string `json:"authorUrl"`
	Content     string `json:"content"`
	ParentID    string `json:"parentId"`
}

func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	var input commentRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	if !s.allowComment(r) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "尝试过于频繁，请稍后再试")
		return
	}
	comment, err := s.app.CreateComment(r.PathValue("slug"), service.CommentInput{
		AuthorName:  input.AuthorName,
		AuthorEmail: input.AuthorEmail,
		AuthorURL:   input.AuthorURL,
		Content:     input.Content,
		ParentID:    input.ParentID,
	})
	if err != nil {
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "comment.created", "comment", comment.ID, map[string]string{"postId": comment.PostID, "status": comment.Status, "parentId": comment.ParentID})
	statusCode := http.StatusAccepted
	if comment.Status == "approved" {
		statusCode = http.StatusCreated
	}
	writeJSON(w, statusCode, publicComment(comment, false))
}

func (s *Server) listAllComments(w http.ResponseWriter, r *http.Request) {
	if !s.requireModerator(w, r) {
		return
	}
	page, pageSize, ok := s.pages(w, r)
	if !ok {
		return
	}
	comments, err := s.app.ListAllComments(r.URL.Query().Get("status"), page, pageSize)
	if err != nil {
		status, code := serviceError(err)
		if status == http.StatusNotFound {
			status = http.StatusInternalServerError
			code = "read_failed"
		}
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicCommentPage(comments, true))
}

type commentStatusRequest struct {
	Status string `json:"status"`
}

func (s *Server) updateComment(w http.ResponseWriter, r *http.Request) {
	if !s.requireModerator(w, r) {
		return
	}
	var input commentStatusRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	comment, err := s.app.ModerateComment(r.PathValue("id"), input.Status)
	if err != nil {
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "comment.moderated", "comment", comment.ID, map[string]string{"status": input.Status})
	writeJSON(w, http.StatusOK, publicComment(comment, true))
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	if !s.requireModerator(w, r) {
		return
	}
	if err := s.app.DeleteComment(r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Comment not found")
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
	} else {
		s.recordAudit(r, "comment.deleted", "comment", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	page, pageSize, ok := s.pages(w, r)
	if !ok {
		return
	}
	media, err := s.app.ListMedia(page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read media")
		return
	}
	writeJSON(w, http.StatusOK, publicMediaPage(media))
}

func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	if err := os.MkdirAll(s.mediaRoot, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_failed", "Unable to prepare media storage")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+(1<<20))
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Media files must be 10 MB or smaller")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", "A multipart file field is required")
		}
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "A multipart file field is required")
		return
	}
	defer file.Close()
	filename := cleanMediaFilename(header.Filename)
	if filename == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "The uploaded filename is invalid")
		return
	}

	head := make([]byte, 512)
	n, readErr := file.Read(head)
	if n == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "The uploaded file is empty")
		return
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Unable to read the uploaded file")
		return
	}
	contentType := http.DetectContentType(head[:n])
	if !allowedMediaType(contentType) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "Only JPEG, PNG, GIF and WebP images are supported")
		return
	}

	temporary, err := os.CreateTemp(s.mediaRoot, ".upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_failed", "Unable to create media file")
		return
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	written, err := temporary.Write(head[:n])
	if err != nil || written != n {
		_ = temporary.Close()
		writeError(w, http.StatusInternalServerError, "storage_failed", "Unable to write media file")
		return
	}
	remaining := io.LimitReader(file, int64(maxUploadSize-n)+1)
	copied, err := io.Copy(temporary, remaining)
	totalSize := int64(written) + copied
	if err != nil {
		_ = temporary.Close()
		writeError(w, http.StatusInternalServerError, "storage_failed", "Unable to write media file")
		return
	}
	if totalSize > maxUploadSize {
		_ = temporary.Close()
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Media files must be 10 MB or smaller")
		return
	}
	if err := temporary.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_failed", "Unable to finalize media file")
		return
	}

	id := newMediaID()
	storageKey := id + mediaExtension(contentType)
	finalPath := filepath.Join(s.mediaRoot, storageKey)
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_failed", "Unable to finalize media file")
		return
	}
	media, err := s.app.RegisterMedia(store.Media{ID: id, Filename: filename, ContentType: contentType, Size: totalSize, StorageKey: storageKey})
	if err != nil {
		_ = os.Remove(finalPath)
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "media.uploaded", "media", media.ID, map[string]string{"filename": media.Filename, "contentType": media.ContentType})
	writeJSON(w, http.StatusCreated, publicMedia(media))
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	media, err := s.app.GetMedia(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Media not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read media")
		return
	}
	storageKey := filepath.Base(media.StorageKey)
	if storageKey == media.StorageKey {
		if err := os.Remove(filepath.Join(s.mediaRoot, storageKey)); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, "delete_failed", "Unable to remove media file")
			return
		}
	} else {
		writeError(w, http.StatusInternalServerError, "delete_failed", "Media storage key is invalid")
		return
	}
	if err := s.app.DeleteMedia(media.ID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Media not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	s.recordAudit(r, "media.deleted", "media", media.ID, map[string]string{"filename": media.Filename})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	media, err := s.app.GetMedia(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to read media")
		return
	}
	storageKey := filepath.Base(media.StorageKey)
	if storageKey != media.StorageKey {
		http.NotFound(w, r)
		return
	}
	file, _, err := openRegularFile(filepath.Join(s.mediaRoot, storageKey))
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "Unable to open media")
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", media.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, media.Filename, media.CreatedAt, file)
}

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	var input postRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	post, err := input.post()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	created, err := s.app.CreatePost(applyAllowComment(post, input, true))
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "post_taken", "已有使用该固定链接的文章")
			return
		}
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "post.created", "post", created.ID, map[string]string{"status": created.Status})
	writeJSON(w, http.StatusCreated, publicPost(created, true))
}

func (s *Server) updatePost(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	post, err := s.app.GetPost(r.PathValue("id"), true)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	var input postRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	nextPost, err := input.post()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	updated, err := s.app.UpdatePost(post.ID, applyAllowComment(nextPost, input, post.AllowComment))
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "post_taken", "已有使用该固定链接的文章")
			return
		}
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "post.updated", "post", updated.ID, map[string]string{"status": updated.Status})
	writeJSON(w, http.StatusOK, publicPost(updated, true))
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	if err := s.app.DeletePost(r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
	} else {
		s.recordAudit(r, "post.deleted", "post", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) createPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	var input pageRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	created, err := s.app.CreatePage(input.page())
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "page_taken", "已有使用该固定链接的页面")
			return
		}
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "page.created", "page", created.ID, map[string]string{"status": created.Status})
	writeJSON(w, http.StatusCreated, publicPage(created))
}

func (s *Server) updatePage(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	page, err := s.app.GetPage(r.PathValue("id"), true)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Page not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	var input pageRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	updated, err := s.app.UpdatePage(page.ID, input.page())
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "page_taken", "已有使用该固定链接的页面")
			return
		}
		status, code := serviceError(err)
		writeError(w, status, code, err.Error())
		return
	}
	s.recordAudit(r, "page.updated", "page", updated.ID, map[string]string{"status": updated.Status})
	writeJSON(w, http.StatusOK, publicPage(updated))
}

func (s *Server) deletePage(w http.ResponseWriter, r *http.Request) {
	if !s.requireContentEditor(w, r) {
		return
	}
	if err := s.app.DeletePage(r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Page not found")
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
	} else {
		s.recordAudit(r, "page.deleted", "page", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}
