package lab19

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// File ini sudah jadi. Jangan diubah.

var (
	errSMTPBusy   = errors.New("SMTP 451 server busy")
	errBadAddress = fmt.Errorf("alamat email tidak valid: %w", ErrPermanent)
)

// healthySender langsung berhasil.
type healthySender struct{ sent []string }

func (s *healthySender) Send(ctx context.Context, email Email) error {
	s.sent = append(s.sent, email.BookingID)
	return nil
}

// hangingSender meniru server SMTP yang menerima koneksi lalu diam. Ia baru
// kembali ketika ctx berakhir.
type hangingSender struct{}

func (hangingSender) Send(ctx context.Context, email Email) error {
	<-ctx.Done()
	return ctx.Err()
}

// fakeClock adalah jam yang hanya maju kalau test memajukannya.
type fakeClock struct{ t time.Time }

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)}
}
func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func emailMessages(ids ...string) []Message {
	var messages []Message
	for _, id := range ids {
		messages = append(messages, Message{ID: id, Email: Email{BookingID: "B-" + id, To: id + "@example.test"}})
	}
	return messages
}

// Dunia ideal: SMTP sehat, semua message terkirim.
func TestStep0_HealthyDependency(t *testing.T) {
	sender := &healthySender{}
	dlq := &DeadLetterQueue{}
	handle := func(msg Message) error {
		return SendWithTimeout(context.Background(), sender, time.Second, msg.Email)
	}
	if err := Consume(emailMessages("m-1", "m-2", "m-3"), handle, 3, dlq); err != nil {
		t.Fatalf("Consume gagal: %v", err)
	}
	if want := []string{"B-m-1", "B-m-2", "B-m-3"}; !slices.Equal(sender.sent, want) {
		t.Fatalf("email terkirim = %v, seharusnya %v", sender.sent, want)
	}
	if got := len(dlq.Entries()); got != 0 {
		t.Fatalf("dead letter queue berisi %d message, seharusnya kosong", got)
	}
}

// Server SMTP menerima koneksi lalu diam. Tanpa timeout, satu panggilan ini
// menahan consumer selama server itu mau.
func TestStep1_OutboundCallHasTimeout(t *testing.T) {
	// Jaring pengaman supaya test tidak menggantung kalau timeout belum dipasang.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	err := SendWithTimeout(ctx, hangingSender{}, 50*time.Millisecond, Email{BookingID: "B-1"})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, seharusnya context.DeadlineExceeded", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("panggilan baru kembali setelah %v, seharusnya sekitar 50ms: timeout belum dipasang pada panggilan keluar", elapsed.Round(time.Millisecond))
	}
}

// recorder mencatat jeda yang diminta Retry tanpa benar-benar menunggu.
type recorder struct {
	delays []time.Duration
	err    error
}

func (r *recorder) Sleep(ctx context.Context, d time.Duration) error {
	r.delays = append(r.delays, d)
	return r.err
}

func retryConfig(maxAttempts int, sleep *recorder) RetryConfig {
	return RetryConfig{
		MaxAttempts: maxAttempts,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    300 * time.Millisecond,
		Sleep:       sleep.Sleep,
		Jitter:      func() float64 { return 0.5 },
	}
}

func TestStep2_RetryWithBackoffAndJitter(t *testing.T) {
	ms := func(values ...int) []time.Duration {
		var out []time.Duration
		for _, v := range values {
			out = append(out, time.Duration(v)*time.Millisecond)
		}
		return out
	}

	t.Run("gangguan sesaat sembuh setelah dicoba lagi", func(t *testing.T) {
		sleep := &recorder{}
		calls := 0
		err := Retry(context.Background(), retryConfig(5, sleep), func(ctx context.Context) error {
			calls++
			if calls < 3 {
				return errSMTPBusy
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Retry mengembalikan %v, seharusnya nil: percobaan ketiga berhasil", err)
		}
		if calls != 3 {
			t.Fatalf("op dipanggil %d kali, seharusnya 3", calls)
		}
		if want := ms(50, 100); !slices.Equal(sleep.delays, want) {
			t.Fatalf("jeda = %v, seharusnya %v: backoff 100ms lalu 200ms, masing-masing dikali jitter 0.5", sleep.delays, want)
		}
	})

	t.Run("menyerah setelah MaxAttempts dan backoff dibatasi MaxDelay", func(t *testing.T) {
		sleep := &recorder{}
		calls := 0
		err := Retry(context.Background(), retryConfig(5, sleep), func(ctx context.Context) error {
			calls++
			return errSMTPBusy
		})
		if !errors.Is(err, errSMTPBusy) {
			t.Fatalf("error = %v, seharusnya membungkus error terakhir dengan %%w", err)
		}
		if calls != 5 {
			t.Fatalf("op dipanggil %d kali, seharusnya 5: retry tanpa batas memperpanjang gangguan", calls)
		}
		if want := ms(50, 100, 150, 150); !slices.Equal(sleep.delays, want) {
			t.Fatalf("jeda = %v, seharusnya %v: backoff 100, 200, lalu tertahan di MaxDelay 300", sleep.delays, want)
		}
	})

	t.Run("error permanent tidak dicoba lagi", func(t *testing.T) {
		sleep := &recorder{}
		calls := 0
		err := Retry(context.Background(), retryConfig(5, sleep), func(ctx context.Context) error {
			calls++
			return errBadAddress
		})
		if !errors.Is(err, ErrPermanent) {
			t.Fatalf("error = %v, seharusnya ErrPermanent", err)
		}
		if calls != 1 || len(sleep.delays) != 0 {
			t.Fatalf("op dipanggil %d kali dengan %d jeda, seharusnya 1 kali tanpa jeda: alamat yang salah tidak akan benar dengan menunggu", calls, len(sleep.delays))
		}
	})

	t.Run("berhenti ketika context dibatalkan saat menunggu", func(t *testing.T) {
		sleep := &recorder{err: context.Canceled}
		calls := 0
		err := Retry(context.Background(), retryConfig(5, sleep), func(ctx context.Context) error {
			calls++
			return errSMTPBusy
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, seharusnya context.Canceled", err)
		}
		if calls != 1 {
			t.Fatalf("op dipanggil %d kali, seharusnya 1: shutdown tidak boleh menunggu retry selesai", calls)
		}
	})
}

func TestStep3_CircuitBreaker(t *testing.T) {
	const threshold = 3
	const cooldown = 30 * time.Second

	type harness struct {
		clock   *fakeClock
		breaker *Breaker
		calls   int
	}
	setup := func() *harness {
		clock := newFakeClock()
		return &harness{clock: clock, breaker: NewBreaker(threshold, cooldown, clock.Now)}
	}
	fail := func(h *harness) error {
		return h.breaker.Do(func() error { h.calls++; return errSMTPBusy })
	}
	succeed := func(h *harness) error {
		return h.breaker.Do(func() error { h.calls++; return nil })
	}
	trip := func(t *testing.T, h *harness) {
		t.Helper()
		for i := 0; i < threshold; i++ {
			if err := fail(h); !errors.Is(err, errSMTPBusy) {
				t.Fatalf("selama closed, error dari op harus diteruskan apa adanya, dapat %v", err)
			}
		}
	}

	t.Run("keberhasilan mengembalikan hitungan ke nol", func(t *testing.T) {
		h := setup()
		_ = fail(h)
		_ = fail(h)
		if err := succeed(h); err != nil {
			t.Fatalf("panggilan yang berhasil mengembalikan %v", err)
		}
		_ = fail(h)
		_ = fail(h)
		if h.calls != 5 || h.breaker.State() != StateClosed {
			t.Fatalf("calls = %d, state = %s; seharusnya 5 dan closed: yang dihitung adalah kegagalan berturut-turut", h.calls, h.breaker.State())
		}
	})

	t.Run("terbuka setelah threshold dan berhenti menyentuh dependency", func(t *testing.T) {
		h := setup()
		trip(t, h)
		if got := h.breaker.State(); got != StateOpen {
			t.Fatalf("state = %s setelah %d kegagalan berturut-turut, seharusnya open", got, threshold)
		}
		for i := 0; i < 100; i++ {
			if err := fail(h); !errors.Is(err, ErrCircuitOpen) {
				t.Fatalf("panggilan saat open mengembalikan %v, seharusnya ErrCircuitOpen", err)
			}
		}
		if h.calls != threshold {
			t.Fatalf("dependency dipanggil %d kali, seharusnya %d: saat open, dependency yang sakit tidak boleh menerima beban", h.calls, threshold)
		}
	})

	t.Run("half-open setelah cooldown, percobaan berhasil menutup kembali", func(t *testing.T) {
		h := setup()
		trip(t, h)
		h.clock.Advance(cooldown - time.Second)
		if got := h.breaker.State(); got != StateOpen {
			t.Fatalf("state = %s sebelum cooldown habis, seharusnya open", got)
		}
		h.clock.Advance(time.Second)
		if got := h.breaker.State(); got != StateHalfOpen {
			t.Fatalf("state = %s setelah cooldown, seharusnya half-open", got)
		}
		if err := succeed(h); err != nil {
			t.Fatalf("panggilan percobaan mengembalikan %v, seharusnya diteruskan ke dependency", err)
		}
		if got := h.breaker.State(); got != StateClosed {
			t.Fatalf("state = %s setelah percobaan berhasil, seharusnya closed", got)
		}
		// Hitungan mulai dari nol lagi: butuh threshold kegagalan untuk terbuka.
		_ = fail(h)
		if got := h.breaker.State(); got != StateClosed {
			t.Fatalf("state = %s setelah satu kegagalan pasca-pulih, seharusnya closed", got)
		}
	})

	t.Run("percobaan gagal membuka lagi untuk satu cooldown penuh", func(t *testing.T) {
		h := setup()
		trip(t, h)
		h.clock.Advance(cooldown)
		if err := fail(h); !errors.Is(err, errSMTPBusy) {
			t.Fatalf("panggilan percobaan mengembalikan %v, seharusnya error dari dependency", err)
		}
		if got := h.breaker.State(); got != StateOpen {
			t.Fatalf("state = %s setelah percobaan gagal, seharusnya open lagi", got)
		}
		h.clock.Advance(cooldown - time.Second)
		if err := succeed(h); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("cooldown kedua belum habis tetapi panggilan diteruskan (error = %v): cooldown dihitung ulang dari percobaan yang gagal", err)
		}
		if h.calls != threshold+1 {
			t.Fatalf("dependency dipanggil %d kali, seharusnya %d", h.calls, threshold+1)
		}
	})

	t.Run("half-open hanya mengizinkan satu percobaan sekaligus", func(t *testing.T) {
		h := setup()
		trip(t, h)
		h.clock.Advance(cooldown)

		var second error
		secondCalled := false
		first := h.breaker.Do(func() error {
			// Selagi percobaan pertama masih berjalan, panggilan lain datang.
			second = h.breaker.Do(func() error { secondCalled = true; return nil })
			return nil
		})
		if first != nil {
			t.Fatalf("percobaan pertama mengembalikan %v", first)
		}
		if secondCalled || !errors.Is(second, ErrCircuitOpen) {
			t.Fatalf("panggilan kedua saat percobaan masih berjalan ikut diteruskan (error = %v): dependency yang baru pulih tidak boleh langsung dibanjiri", second)
		}
	})
}

// Message m-2 gagal terus dan m-4 tidak akan pernah berhasil. Tiga message
// lain tidak punya masalah apa pun dan tidak boleh ikut tertahan.
func TestStep4_PoisonMessageGoesToDeadLetterQueue(t *testing.T) {
	const maxAttempts = 3
	dlq := &DeadLetterQueue{}
	attempts := map[string]int{}
	var delivered []string

	handle := func(msg Message) error {
		attempts[msg.ID]++
		switch msg.ID {
		case "m-2":
			return errSMTPBusy
		case "m-4":
			return errBadAddress
		}
		delivered = append(delivered, msg.ID)
		return nil
	}

	if err := Consume(emailMessages("m-1", "m-2", "m-3", "m-4", "m-5"), handle, maxAttempts, dlq); err != nil {
		t.Fatalf("Consume berhenti dengan error %v: satu message yang gagal tidak boleh menghentikan stream", err)
	}
	if want := []string{"m-1", "m-3", "m-5"}; !slices.Equal(delivered, want) {
		t.Fatalf("message terproses = %v, seharusnya %v", delivered, want)
	}
	if attempts["m-2"] != maxAttempts {
		t.Fatalf("m-2 dicoba %d kali, seharusnya %d", attempts["m-2"], maxAttempts)
	}
	if attempts["m-4"] != 1 {
		t.Fatalf("m-4 dicoba %d kali, seharusnya 1: error permanent langsung dipinggirkan", attempts["m-4"])
	}

	entries := dlq.Entries()
	if len(entries) != 2 {
		t.Fatalf("dead letter queue berisi %d message, seharusnya 2", len(entries))
	}
	busy, bad := entries[0], entries[1]
	if busy.Message.ID != "m-2" || busy.Attempts != maxAttempts || !strings.Contains(busy.Reason, "SMTP 451") {
		t.Fatalf("entry pertama = %+v, seharusnya m-2 dengan %d percobaan dan alasan memuat \"SMTP 451\"", busy, maxAttempts)
	}
	if bad.Message.ID != "m-4" || bad.Attempts != 1 || !strings.Contains(bad.Reason, "alamat email tidak valid") {
		t.Fatalf("entry kedua = %+v, seharusnya m-4 dengan 1 percobaan dan alasan memuat \"alamat email tidak valid\"", bad)
	}
	if busy.Message.Email.To != "m-2@example.test" {
		t.Fatalf("isi message di dead letter queue berubah: %+v", busy.Message)
	}
}

// SMTP sudah pulih. Message di dead letter queue harus bisa dikembalikan ke
// stream utama tanpa risiko hilang.
func TestStep5_RequeueFromDeadLetterQueue(t *testing.T) {
	newQueue := func() *DeadLetterQueue {
		dlq := &DeadLetterQueue{}
		for _, msg := range emailMessages("m-2", "m-4") {
			dlq.Add(DeadLetter{Message: msg, Reason: "SMTP 451 server busy", Attempts: 3})
		}
		return dlq
	}

	t.Run("publish berhasil: message keluar dari dead letter queue", func(t *testing.T) {
		dlq := newQueue()
		var published []Message
		err := dlq.Requeue("m-2", func(msg Message) error {
			published = append(published, msg)
			return nil
		})
		if err != nil {
			t.Fatalf("Requeue gagal: %v", err)
		}
		if len(published) != 1 || published[0].ID != "m-2" || published[0].Email.To != "m-2@example.test" {
			t.Fatalf("yang dipublikasikan = %+v, seharusnya message m-2 utuh", published)
		}
		entries := dlq.Entries()
		if len(entries) != 1 || entries[0].Message.ID != "m-4" {
			t.Fatalf("isi dead letter queue = %+v, seharusnya tinggal m-4", entries)
		}
	})

	t.Run("publish gagal: message tetap di dead letter queue", func(t *testing.T) {
		dlq := newQueue()
		errBroker := errors.New("broker tidak terjangkau")
		err := dlq.Requeue("m-2", func(msg Message) error { return errBroker })
		if !errors.Is(err, errBroker) {
			t.Fatalf("error = %v, seharusnya membungkus error publish", err)
		}
		if got := len(dlq.Entries()); got != 2 {
			t.Fatalf("dead letter queue berisi %d message, seharusnya tetap 2: message dihapus sebelum publish berhasil berarti hilang", got)
		}
	})

	t.Run("message yang tidak ada", func(t *testing.T) {
		dlq := newQueue()
		called := false
		err := dlq.Requeue("m-99", func(msg Message) error { called = true; return nil })
		if !errors.Is(err, ErrNotInDeadLetterQueue) || called {
			t.Fatalf("error = %v, publish dipanggil = %v; seharusnya ErrNotInDeadLetterQueue tanpa publish", err, called)
		}
	})
}
