// Package logging, uygulamanın yapılandırılmış logger'ını kurar.
package logging

import (
	"io"
	"log/slog"
)

// New, ortama ve seviyeye göre bir *slog.Logger oluşturur.
// Development'ta okunabilir metin, diğer ortamlarda JSON üretir.
func New(w io.Writer, env, level string) *slog.Logger {
	var lvl slog.Level

	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var h slog.Handler

	if env == "development" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(h)
}
