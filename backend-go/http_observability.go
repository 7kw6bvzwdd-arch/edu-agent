package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type requestIDKey struct{}
type responseStatus struct {
	http.ResponseWriter
	status int
}

func (w *responseStatus) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *responseStatus) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (w *responseStatus) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *responseStatus) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (s *Server) serveObserved(w http.ResponseWriter, r *http.Request) {
	id := newID()
	start := time.Now()
	out := &responseStatus{ResponseWriter: w}
	out.Header().Set("X-Request-ID", id)
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("request panic", "request_id", id, "panic_type", fmt.Sprintf("%T", recovered))
			if out.status == 0 {
				respond(out, 500, map[string]string{"message": "服务暂时异常，已有内容保留，请稍后重试", "request_id": id})
			}
		}
		slog.Info("http request", "request_id", id, "method", r.Method, "route", r.Pattern, "status", out.status, "duration_ms", time.Since(start).Milliseconds())
	}()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Cache-Control", "no-store")
	s.mux.ServeHTTP(out, r)
}
