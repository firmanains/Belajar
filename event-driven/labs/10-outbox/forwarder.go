package lab10

// File ini kamu ubah di Step 3 sampai Step 5.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Forwarder memindahkan baris outbox ke broker.
type Forwarder struct {
	pool *pgxpool.Pool
	pub  Publisher

	// BatchSize adalah jumlah baris maksimum per RunOnce.
	BatchSize int
	// MaxAttempts adalah batas percobaan sebelum sebuah baris menjadi DEAD.
	MaxAttempts int
}

func NewForwarder(pool *pgxpool.Pool, pub Publisher) *Forwarder {
	return &Forwarder{pool: pool, pub: pub, BatchSize: 50, MaxAttempts: 3}
}

// RunOnce memproses satu batch baris PENDING, urut berdasarkan id.
//
// Kontraknya:
//   - Message.ID diisi id baris outbox (sebagai string), Topic dan Payload dari barisnya.
//   - Publish berhasil: status menjadi SENT dan sent_at diisi.
//   - Publish gagal: attempts bertambah satu dan last_error diisi. Kalau attempts
//     mencapai MaxAttempts, status menjadi DEAD. Baris lain di batch tetap diproses.
//   - Return pertama adalah jumlah baris yang berhasil di-publish.
//   - Error hanya untuk kegagalan database. Publish yang gagal bukan error RunOnce.
//
// Di produksi fungsi ini dipanggil berulang oleh sebuah loop atau cron.
func (f *Forwarder) RunOnce(ctx context.Context) (int, error) {
	// Belum diimplementasikan: tidak ada baris yang diteruskan.
	return 0, nil
}
