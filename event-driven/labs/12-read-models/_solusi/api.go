package lab12

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
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

const maxNights = 366

// NewAPI mengembalikan HTTP handler untuk read model.
//
//	GET /availability?room_type=deluxe&from=2026-12-01&to=2026-12-05
func NewAPI(pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /availability", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		roomType := q.Get("room_type")
		from, errFrom := time.Parse(time.DateOnly, q.Get("from"))
		to, errTo := time.Parse(time.DateOnly, q.Get("to"))
		if roomType == "" || errFrom != nil || errTo != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "butuh room_type, from, dan to (YYYY-MM-DD)"})
			return
		}
		if nights := int(to.Sub(from).Hours() / 24); nights < 1 || nights > maxNights {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to harus sesudah from, paling jauh 366 malam"})
			return
		}

		// Handler tidak menghitung apa-apa: ia hanya membaca baris yang sudah
		// disiapkan projector. generate_series mengisi malam yang belum punya baris.
		rows, err := pool.Query(r.Context(), `
			SELECT to_char(d, 'YYYY-MM-DD'),
			       COALESCE(v.rooms_sold, 0),
			       rt.capacity - COALESCE(v.rooms_sold, 0)
			FROM room_types rt
			CROSS JOIN generate_series($2::date, $3::date - 1, interval '1 day') AS d
			LEFT JOIN availability_view v ON v.room_type = rt.room_type AND v.day = d::date
			WHERE rt.room_type = $1
			ORDER BY d`,
			roomType, from.Format(time.DateOnly), to.Format(time.DateOnly))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read model tidak bisa dibaca"})
			return
		}
		days, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AvailabilityDay, error) {
			var d AvailabilityDay
			err := row.Scan(&d.Date, &d.RoomsSold, &d.Available)
			return d, err
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read model tidak bisa dibaca"})
			return
		}
		if len(days) == 0 {
			// Rentang tanggal valid selalu menghasilkan baris, jadi kosong berarti
			// room type-nya tidak ada.
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "room_type tidak dikenal"})
			return
		}
		writeJSON(w, http.StatusOK, AvailabilityResponse{RoomType: roomType, Days: days})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Gagal menulis body berarti client sudah pergi; tidak ada yang bisa dilakukan.
	_ = json.NewEncoder(w).Encode(v)
}
