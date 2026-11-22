// Package inbox menerima booking revision dari channel manager dan
// menerapkannya ke write model.
package inbox

// File ini sudah jadi. Jangan diubah.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/platform"
)

// Revision adalah satu perubahan booking yang dikirim channel manager.
// RevisionID unik per revision. RevisionNumber naik per booking: 1, 2, 3, ...
type Revision struct {
	RevisionID     string `json:"revision_id"`
	BookingID      string `json:"booking_id"`
	RevisionNumber int    `json:"revision_number"`
	Status         string `json:"status"` // new | modified | cancelled
	RoomType       string `json:"room_type"`
	CheckIn        string `json:"check_in"`
	CheckOut       string `json:"check_out"`
	Rooms          int    `json:"rooms"`
	GuestEmail     string `json:"guest_email"`
}

// Booking mengubah revision menjadi state booking yang disimpan.
func (r Revision) Booking() event.Booking {
	status := event.StatusConfirmed
	if r.Status == "cancelled" {
		status = event.StatusCancelled
	}
	return event.Booking{
		BookingID: r.BookingID, Revision: r.RevisionNumber, Status: status, RoomType: r.RoomType,
		CheckIn: r.CheckIn, CheckOut: r.CheckOut, Rooms: r.Rooms, GuestEmail: r.GuestEmail,
	}
}

func (r Revision) validate() error {
	switch {
	case r.RevisionID == "" || r.BookingID == "" || r.RoomType == "" || r.GuestEmail == "":
		return errors.New("revision_id, booking_id, room_type, dan guest_email wajib diisi")
	case r.RevisionNumber < 1:
		return errors.New("revision_number minimal 1")
	case r.Status != "new" && r.Status != "modified" && r.Status != "cancelled":
		return errors.New("status harus new, modified, atau cancelled")
	case r.Rooms < 1 || r.Rooms > 100:
		return errors.New("rooms harus antara 1 dan 100")
	}
	return platform.ValidateStay(r.CheckIn, r.CheckOut)
}

// Result adalah nasib sebuah revision.
type Result string

const (
	Applied   Result = "applied"
	Duplicate Result = "duplicate" // revision_id ini sudah pernah diterima
	Stale     Result = "stale"     // sudah ada revision_number yang lebih baru
)

const HeaderCorrelationID = "X-Correlation-ID"

// Webhook adalah handler POST /webhooks/booking-revisions. Response 200 baru
// dikirim setelah Apply selesai, artinya setelah commit. Channel manager
// menganggap selain 2xx sebagai gagal dan akan mengirim ulang.
func Webhook(svc *Service, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var rev Revision
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		if err := dec.Decode(&rev); err != nil {
			platform.WriteError(w, http.StatusBadRequest, "body bukan JSON yang valid")
			return
		}
		if err := rev.validate(); err != nil {
			platform.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		known, err := svc.roomTypeExists(r.Context(), rev.RoomType)
		if err != nil {
			log.Error("cek room type gagal", "error", err)
			platform.WriteError(w, http.StatusInternalServerError, "gagal memproses revision")
			return
		}
		if !known {
			platform.WriteError(w, http.StatusBadRequest, "room_type tidak dikenal")
			return
		}

		correlationID := r.Header.Get(HeaderCorrelationID)
		if correlationID == "" || len(correlationID) > 128 {
			correlationID = event.NewID()
		}
		w.Header().Set(HeaderCorrelationID, correlationID)
		log := log.With("correlation_id", correlationID, "booking_id", rev.BookingID, "revision_id", rev.RevisionID)

		result, err := svc.Apply(r.Context(), rev, correlationID)
		if err != nil {
			log.Error("revision gagal diterapkan", "error", err)
			platform.WriteError(w, http.StatusInternalServerError, "gagal memproses revision")
			return
		}
		log.Info("revision diterima", "result", string(result))
		platform.WriteJSON(w, http.StatusOK, map[string]string{"result": string(result)})
	}
}
