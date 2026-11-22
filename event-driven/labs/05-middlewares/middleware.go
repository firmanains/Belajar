package lab05

// File ini yang kamu ubah.

import (
	"context"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill/message"
)

// CorrelationIDMetadataKey adalah nama metadata tempat correlation ID dibawa
// dari satu message ke message berikutnya.
const CorrelationIDMetadataKey = "correlation_id"

// CorrelationIDFromContext mengembalikan correlation ID yang ditaruh middleware
// CorrelationID di context, atau "" kalau tidak ada.
func CorrelationIDFromContext(ctx context.Context) string {
	return ""
}

// CorrelationID memastikan setiap message yang diproses punya correlation ID,
// menaruhnya di context handler, dan menyalinnya ke semua message yang
// dihasilkan handler. Versi ini belum melakukan apa-apa.
func CorrelationID(h message.HandlerFunc) message.HandlerFunc {
	return h
}

// Logging mencatat awal dan akhir pemrosesan setiap message ke logger.
// Versi ini belum melakukan apa-apa.
func Logging(logger *slog.Logger) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return h
	}
}

// Middlewares menentukan middleware apa saja yang dipasang di router dan
// urutannya. Yang pertama di slice menjadi lapisan terluar.
func Middlewares(logger *slog.Logger) []message.HandlerMiddleware {
	return []message.HandlerMiddleware{
		Logging(logger),
		CorrelationID,
	}
}
