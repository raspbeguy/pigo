// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package server

import (
	"log/slog"
	"net/http"
)

// respondError logs the structured detail server-side and writes a
// generic status-text body to the client. Keeps plugin / template /
// filesystem error strings out of public HTTP responses (they can leak
// theme paths, dependency names, or site layout) while keeping them
// visible to operators through the configured logger.
func respondError(w http.ResponseWriter, logger *slog.Logger, status int, msg string, keyvals ...any) {
	if logger != nil {
		logger.Error(msg, keyvals...)
	}
	http.Error(w, http.StatusText(status), status)
}

// dispatchLog forwards a non-nil plugin Dispatch error to the logger at
// warn level with consistent keying. Pure-observation events keep this
// shape — we don't want plugin misbehaviour during an observation hook
// to 500 the user, but silent swallow is wrong too.
func dispatchLog(logger *slog.Logger, event string, path string, err error) {
	if err == nil || logger == nil {
		return
	}
	logger.Warn("plugin dispatch failed", "event", event, "path", path, "err", err)
}
