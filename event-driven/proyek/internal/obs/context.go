// Package obs berisi logging dan metrics.
package obs

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

// WithLogger menitipkan logger di context.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// LoggerFrom mengambil logger dari context, atau fallback kalau tidak ada.
func LoggerFrom(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return fallback
}
