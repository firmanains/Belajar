package lab10

// File ini sudah jadi. Jangan diubah; baca saja.

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Message adalah satu kiriman ke broker. ID dipakai consumer untuk mengenali duplikat.
type Message struct {
	ID      string
	Topic   string
	Payload []byte
}

// Publisher adalah satu-satunya hal yang service ini tahu tentang broker.
type Publisher interface {
	Publish(ctx context.Context, m Message) error
}

// FakePublisher menggantikan Kafka di lab ini. Ia mencatat semua message yang
// sampai dan bisa disuruh gagal dengan tiga cara.
type FakePublisher struct {
	// Delay meniru broker yang lambat.
	Delay time.Duration

	mu          sync.Mutex
	published   []Message
	calls       int
	failNext    int
	loseAckNext int
	failWhen    func(Message) bool
}

var ErrBrokerDown = errors.New("broker tidak terjangkau")

// FailNext membuat n panggilan Publish berikutnya gagal. Message tidak sampai.
func (p *FakePublisher) FailNext(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNext = n
}

// LoseAckNext membuat n panggilan Publish berikutnya return error padahal
// message sudah sampai di broker. Ini meniru ack yang hilang di jaringan.
func (p *FakePublisher) LoseAckNext(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loseAckNext = n
}

// FailWhen membuat Publish selalu gagal untuk message yang cocok.
func (p *FakePublisher) FailWhen(match func(Message) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failWhen = match
}

func (p *FakePublisher) Publish(ctx context.Context, m Message) error {
	if p.Delay > 0 {
		select {
		case <-time.After(p.Delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.failWhen != nil && p.failWhen(m) {
		return ErrBrokerDown
	}
	if p.failNext > 0 {
		p.failNext--
		return ErrBrokerDown
	}
	p.published = append(p.published, m)
	if p.loseAckNext > 0 {
		p.loseAckNext--
		return errors.New("timeout menunggu ack dari broker")
	}
	return nil
}

// Published mengembalikan salinan semua message yang sampai di broker.
func (p *FakePublisher) Published() []Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Message(nil), p.published...)
}

// Calls mengembalikan berapa kali Publish dipanggil, berhasil maupun gagal.
func (p *FakePublisher) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}
