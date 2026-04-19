// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package server

import (
	"log/slog"
	"net/http"
	"time"
)

// accessLog wraps h so each request emits one structured info-level
// "request" record: method, path, status, bytes written, duration,
// remote addr. Users who want this suppressed can raise the threshold
// to warn.
func accessLog(logger *slog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"dur_ms", time.Since(start).Milliseconds(),
			"remote", remoteIP(r),
		)
	})
}

// statusRecorder wraps http.ResponseWriter so the middleware can see the
// response status code and the number of bytes written.
type statusRecorder struct {
	http.ResponseWriter
	status       int
	bytes        int
	wroteHeader  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		// http.ResponseWriter.Write triggers an implicit 200 if headers
		// haven't been written yet; record that here too.
		r.wroteHeader = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// remoteIP strips the port from r.RemoteAddr so the log field is just the
// client IP. Reverse-proxy-forwarded addresses are left to the operator
// to handle out-of-band (e.g. a separate middleware that honors
// X-Forwarded-For); we don't want to trust that header by default.
func remoteIP(r *http.Request) string {
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
