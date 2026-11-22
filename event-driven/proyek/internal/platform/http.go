package platform

// File ini sudah jadi. Jangan diubah.

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// WriteJSON menulis v sebagai response JSON.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // header sudah terkirim; kegagalan tulis tidak bisa dilaporkan ke klien
}

// WriteError menulis pesan error sebagai JSON.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

const dateLayout = "2006-01-02"

// ValidateStay memeriksa bahwa from dan to adalah tanggal YYYY-MM-DD, from
// sebelum to, dan rentangnya paling lama satu tahun.
func ValidateStay(from, to string) error {
	f, err := time.Parse(dateLayout, from)
	if err != nil {
		return errors.New("tanggal awal harus berformat YYYY-MM-DD")
	}
	t, err := time.Parse(dateLayout, to)
	if err != nil {
		return errors.New("tanggal akhir harus berformat YYYY-MM-DD")
	}
	if !f.Before(t) {
		return errors.New("tanggal awal harus sebelum tanggal akhir")
	}
	if t.Sub(f) > 366*24*time.Hour {
		return errors.New("rentang tanggal paling lama 366 hari")
	}
	return nil
}
