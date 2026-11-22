package lab12

// File ini kamu ubah di Step 4.

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AvailabilityResponse adalah bentuk jawaban GET /availability.
type AvailabilityResponse struct {
	RoomType string            `json:"room_type"`
	Days     []AvailabilityDay `json:"days"`
}

// AvailabilityDay adalah satu malam di tape chart.
type AvailabilityDay struct {
	Date      string `json:"date"` // YYYY-MM-DD
	RoomsSold int    `json:"rooms_sold"`
	Available int    `json:"available"`
}

// NewAPI mengembalikan HTTP handler untuk read model.
//
//	GET /availability?room_type=deluxe&from=2026-12-01&to=2026-12-05
//
// Kontraknya:
//   - from dan to berformat YYYY-MM-DD; to tidak ikut dihitung, seperti CheckOut.
//   - 200 dengan AvailabilityResponse: satu AvailabilityDay per malam, urut tanggal.
//     Malam tanpa booking tetap muncul dengan rooms_sold 0.
//   - 400 kalau parameter kosong, tanggal tidak valid, to tidak sesudah from,
//     atau rentangnya lebih dari 366 malam.
//   - 404 kalau room_type tidak ada di tabel room_types.
func NewAPI(pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /availability", func(w http.ResponseWriter, r *http.Request) {
		// Belum diimplementasikan.
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "belum diimplementasikan"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Gagal menulis body berarti client sudah pergi; tidak ada yang bisa dilakukan.
	_ = json.NewEncoder(w).Encode(v)
}
