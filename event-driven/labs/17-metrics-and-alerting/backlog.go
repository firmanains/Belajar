package lab17

import (
	"sync"
	"time"
)

// File ini sudah jadi. Jangan diubah.

// Backlog meniru antrean message yang belum diproses. Di Kafka padanannya
// adalah consumer lag; di tabel outbox padanannya adalah baris yang masih pending.
type Backlog struct {
	mu       sync.Mutex
	enqueued []time.Time // waktu masuk tiap message, yang paling lama di depan
}

// Enqueue mencatat satu message baru yang masuk pada waktu at.
func (b *Backlog) Enqueue(at time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.enqueued = append(b.enqueued, at)
}

// Dequeue membuang n message tertua, seolah-olah consumer sudah memprosesnya.
func (b *Backlog) Dequeue(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n > len(b.enqueued) {
		n = len(b.enqueued)
	}
	b.enqueued = b.enqueued[n:]
}

// Len mengembalikan jumlah message yang belum diproses.
func (b *Backlog) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.enqueued)
}

// OldestAge mengembalikan umur message tertua yang belum diproses.
// Antrean kosong berarti umur nol.
func (b *Backlog) OldestAge(now time.Time) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.enqueued) == 0 {
		return 0
	}
	return now.Sub(b.enqueued[0])
}
