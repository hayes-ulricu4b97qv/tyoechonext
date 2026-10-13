package httpapi

import (
	"net/http"
	"strings"
)

func apiErrorEnvelope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1" && !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&apiErrorWriter{ResponseWriter: w}, r)
	})
}

type apiErrorWriter struct {
	http.ResponseWriter
	wrote    bool
	replaced bool
}

func (w *apiErrorWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *apiErrorWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	if (status == http.StatusNotFound || status == http.StatusMethodNotAllowed) &&
		!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		code, message := "not_found", "Not found"
		if status == http.StatusMethodNotAllowed {
			code, message = "method_not_allowed", "Method not allowed"
		}
		w.Header().Del("Content-Length")
		w.replaced = true
		writeError(w.ResponseWriter, status, code, message)
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *apiErrorWriter) Write(payload []byte) (int, error) {
	if w.replaced {
		return len(payload), nil
	}
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(payload)
}
