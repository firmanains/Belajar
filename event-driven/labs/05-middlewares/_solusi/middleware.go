package lab05

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

// CorrelationIDMetadataKey adalah nama metadata tempat correlation ID dibawa
// dari satu message ke message berikutnya.
const CorrelationIDMetadataKey = "correlation_id"

// Tipe kunci privat mencegah package lain menimpa nilai ini di context.
type correlationIDKey struct{}

// CorrelationIDFromContext mengembalikan correlation ID yang ditaruh middleware
// CorrelationID di context, atau "" kalau tidak ada.
func CorrelationIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDKey{}).(string)
	return id
}

// CorrelationID memastikan setiap message yang diproses punya correlation ID,
// menaruhnya di context handler, dan menyalinnya ke semua message yang
// dihasilkan handler.
func CorrelationID(h message.HandlerFunc) message.HandlerFunc {
	return func(msg *message.Message) ([]*message.Message, error) {
		id := msg.Metadata.Get(CorrelationIDMetadataKey)
		if id == "" {
			id = watermill.NewUUID()
		}
		msg.SetContext(context.WithValue(msg.Context(), correlationIDKey{}, id))

		produced, err := h(msg)
		for _, out := range produced {
			out.Metadata.Set(CorrelationIDMetadataKey, id)
		}
		return produced, err
	}
}

// Logging mencatat awal dan akhir pemrosesan setiap message ke logger.
// Logger masuk lewat closure: begitulah middleware menerima dependency.
func Logging(logger *slog.Logger) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			log := logger.With(
				"message_uuid", msg.UUID,
				"handler", message.HandlerNameFromCtx(msg.Context()),
				"correlation_id", CorrelationIDFromContext(msg.Context()),
			)
			log.Info("message received")

			start := time.Now()
			produced, err := h(msg)
			duration := time.Since(start).Milliseconds()

			if err != nil {
				log.Error("message failed", "error", err.Error(), "duration_ms", duration)
				return produced, err
			}
			log.Info("message handled", "duration_ms", duration)
			return produced, nil
		}
	}
}

// Middlewares menentukan middleware apa saja yang dipasang di router dan
// urutannya. CorrelationID harus di luar Logging, supaya ID sudah ada di
// context ketika Logging menulis baris pertamanya.
func Middlewares(logger *slog.Logger) []message.HandlerMiddleware {
	return []message.HandlerMiddleware{
		CorrelationID,
		Logging(logger),
	}
}
