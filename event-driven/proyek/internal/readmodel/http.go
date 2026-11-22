// Package readmodel membangun tampilan baca dari event dan menyajikannya.
package readmodel

// File ini sudah jadi. Jangan diubah.

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"belajar/eventdriven/proyek/internal/platform"
	"belajar/eventdriven/proyek/internal/store"
)

// AvailabilityHandler melayani GET ...?from=YYYY-MM-DD&to=YYYY-MM-DD dari
// pasangan tabel t. Dipakai untuk /tape-chart (read model) dan
// /admin/availability (write model).
func AvailabilityHandler(pool *pgxpool.Pool, t store.Tables, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		if err := platform.ValidateStay(from, to); err != nil {
			platform.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		days, err := t.Availability(r.Context(), pool, from, to)
		if err != nil {
			log.Error("query availability gagal", "error", err)
			platform.WriteError(w, http.StatusInternalServerError, "gagal membaca data")
			return
		}
		platform.WriteJSON(w, http.StatusOK, days)
	}
}

// BookingHandler melayani GET /bookings/{id} dari read model.
func BookingHandler(pool *pgxpool.Pool, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := store.ReadModel.GetBooking(r.Context(), pool, r.PathValue("id"))
		if err != nil {
			log.Error("query booking gagal", "error", err)
			platform.WriteError(w, http.StatusInternalServerError, "gagal membaca data")
			return
		}
		if b == nil {
			platform.WriteError(w, http.StatusNotFound, "booking tidak ditemukan di read model")
			return
		}
		platform.WriteJSON(w, http.StatusOK, b)
	}
}
