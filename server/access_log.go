// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package server

import (
	"bufio"
	"io"
	"log/slog"
	"net"
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
// response status code and the number of bytes written. Implements
// Flusher / Hijacker / ReaderFrom by delegating to the underlying writer
// when it supports them — downstream handlers that stream, hijack, or
// efficiently copy from io.Reader keep working.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
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

// Flush delegates to the underlying writer when it supports it, so
// chunked responses (SSE, streamed HTML) still reach the client.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack delegates so WebSocket upgrades and other connection takeovers
// work. Byte counting stops after hijack — post-hijack traffic doesn't
// route through statusRecorder.Write.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hj.Hijack()
}

// ReadFrom lets the underlying writer use an efficient sendfile-style
// copy when available (e.g. for static files). Falls back to a buffered
// io.Copy path. Byte counts stay accurate either way.
func (r *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(src)
		r.bytes += int(n)
		return n, err
	}
	n, err := io.Copy(writerOnly{r.ResponseWriter}, src)
	r.bytes += int(n)
	return n, err
}

// writerOnly hides ReaderFrom from io.Copy so the fallback path doesn't
// recurse through r.ResponseWriter.ReadFrom → r.ReadFrom → io.Copy.
type writerOnly struct{ w io.Writer }

func (w writerOnly) Write(b []byte) (int, error) { return w.w.Write(b) }

// remoteIP returns just the client IP from r.RemoteAddr, stripping the
// port. Uses net.SplitHostPort so IPv6 literals (`[::1]:port`) work.
// Reverse-proxy-forwarded addresses are left to the operator (e.g. a
// separate middleware that trusts X-Forwarded-For) — we don't want to
// trust that header by default.
func remoteIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
