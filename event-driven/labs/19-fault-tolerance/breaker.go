package lab19

import "time"

// File ini yang kamu ubah di Step 3.

// Tiga keadaan circuit breaker, sebagaimana dikembalikan State().
const (
	StateClosed   = "closed"    // panggilan diteruskan
	StateOpen     = "open"      // panggilan ditolak tanpa menyentuh dependency
	StateHalfOpen = "half-open" // satu panggilan percobaan diizinkan
)

// Breaker adalah circuit breaker. Field untuk menyimpan keadaannya kamu
// tambahkan sendiri.
type Breaker struct {
	failureThreshold int
	cooldown         time.Duration
	now              func() time.Time
}

// NewBreaker membuat breaker yang terbuka setelah failureThreshold kegagalan
// berturut-turut, lalu mengizinkan satu percobaan setelah cooldown berlalu.
func NewBreaker(failureThreshold int, cooldown time.Duration, now func() time.Time) *Breaker {
	return &Breaker{failureThreshold: failureThreshold, cooldown: cooldown, now: now}
}

// State mengembalikan keadaan breaker saat ini. Versi ini selalu closed.
func (b *Breaker) State() string {
	return StateClosed
}

// Do menjalankan op lewat breaker. Versi ini selalu meneruskan panggilan.
func (b *Breaker) Do(op func() error) error {
	return op()
}
