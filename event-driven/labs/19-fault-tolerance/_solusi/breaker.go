package lab19

import (
	"sync"
	"time"
)

// Tiga keadaan circuit breaker, sebagaimana dikembalikan State().
const (
	StateClosed   = "closed"    // panggilan diteruskan
	StateOpen     = "open"      // panggilan ditolak tanpa menyentuh dependency
	StateHalfOpen = "half-open" // satu panggilan percobaan diizinkan
)

// Breaker adalah circuit breaker dengan tiga keadaan.
type Breaker struct {
	failureThreshold int
	cooldown         time.Duration
	now              func() time.Time

	mu            sync.Mutex
	open          bool
	openedAt      time.Time
	failures      int  // kegagalan berturut-turut selama closed
	trialInFlight bool // sedang ada panggilan percobaan di half-open
}

// NewBreaker membuat breaker yang terbuka setelah failureThreshold kegagalan
// berturut-turut, lalu mengizinkan satu percobaan setelah cooldown berlalu.
func NewBreaker(failureThreshold int, cooldown time.Duration, now func() time.Time) *Breaker {
	return &Breaker{failureThreshold: failureThreshold, cooldown: cooldown, now: now}
}

// State mengembalikan keadaan breaker saat ini. Half-open tidak disimpan:
// ia adalah "open yang cooldown-nya sudah lewat".
func (b *Breaker) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stateLocked()
}

func (b *Breaker) stateLocked() string {
	switch {
	case !b.open:
		return StateClosed
	case b.now().Sub(b.openedAt) >= b.cooldown:
		return StateHalfOpen
	default:
		return StateOpen
	}
}

// Do menjalankan op lewat breaker. Mutex tidak dipegang selama op berjalan,
// supaya panggilan yang lambat tidak menahan panggilan lain yang seharusnya
// langsung ditolak.
func (b *Breaker) Do(op func() error) error {
	if err := b.allow(); err != nil {
		return err
	}
	err := op()
	b.record(err)
	return err
}

func (b *Breaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.stateLocked() {
	case StateOpen:
		return ErrCircuitOpen
	case StateHalfOpen:
		if b.trialInFlight {
			return ErrCircuitOpen
		}
		b.trialInFlight = true
	}
	return nil
}

func (b *Breaker) record(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	wasTrial := b.trialInFlight
	b.trialInFlight = false

	if err == nil {
		b.open = false
		b.failures = 0
		return
	}
	b.failures++
	// Percobaan yang gagal langsung membuka lagi, tanpa menunggu threshold.
	if wasTrial || b.failures >= b.failureThreshold {
		b.open = true
		b.openedAt = b.now()
	}
}
