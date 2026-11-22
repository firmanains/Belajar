package lab08

// File ini sudah jadi. Jangan diubah; baca saja.

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
)

// Broker adalah message broker tiruan di memory. Ia meniru dua sifat Kafka
// yang tidak dimiliki gochannel (sama dengan broker di Lab 06):
//
//  1. Setiap topic adalah satu partition. Message diantar berurutan, dan
//     message berikutnya baru diantar setelah message sekarang mendapat ack.
//     Nack berarti message yang sama diantar lagi.
//  2. Consumer group. Subscriber dalam group yang sama berbagi message;
//     group yang berbeda masing-masing menerima semua message.
//
// Group baru hanya menerima message yang terbit setelah ia subscribe.
type Broker struct {
	redeliveryDelay time.Duration

	mu     sync.Mutex
	topics map[string]map[string]*group // topic -> nama group -> group

	closing   chan struct{}
	closeOnce sync.Once
}

func NewBroker() *Broker {
	return &Broker{
		redeliveryDelay: time.Millisecond,
		topics:          map[string]map[string]*group{},
		closing:         make(chan struct{}),
	}
}

// Publish menaruh salinan message di antrean setiap group yang subscribe ke topic.
func (b *Broker) Publish(topic string, messages ...*message.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, g := range b.topics[topic] {
		for _, msg := range messages {
			select {
			case g.queue <- msg.Copy():
			case <-b.closing:
				return errors.New("broker closed")
			}
		}
	}
	return nil
}

func (b *Broker) Close() error {
	b.closeOnce.Do(func() { close(b.closing) })
	return nil
}

// Subscriber mengembalikan message.Subscriber yang bergabung ke consumer group
// bernama groupName.
func (b *Broker) Subscriber(groupName string) message.Subscriber {
	return &groupSubscriber{broker: b, groupName: groupName}
}

type group struct {
	queue chan *message.Message

	mu      sync.Mutex
	members []*member
	next    int
}

func (b *Broker) group(topic, name string) *group {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.topics[topic] == nil {
		b.topics[topic] = map[string]*group{}
	}
	g, ok := b.topics[topic][name]
	if !ok {
		g = &group{queue: make(chan *message.Message, 1024)}
		b.topics[topic][name] = g
		go b.deliver(g)
	}
	return g
}

// deliver adalah jantung broker: satu message pada satu waktu, diulang sampai ack.
func (b *Broker) deliver(g *group) {
	for {
		var msg *message.Message
		select {
		case msg = <-g.queue:
		case <-b.closing:
			return
		}

		for !b.deliverOnce(g, msg) {
			select {
			case <-time.After(b.redeliveryDelay):
			case <-b.closing:
				return
			}
		}
	}
}

// deliverOnce mengantar msg ke satu member group dan menunggu jawabannya.
// Hasil true berarti ack.
func (b *Broker) deliverOnce(g *group, msg *message.Message) bool {
	m := g.pick()
	if m == nil {
		return false
	}

	// Setiap pengantaran memakai salinan baru, karena ack/nack hanya bisa
	// dipanggil sekali per message.
	attempt := msg.Copy()
	attempt.SetContext(m.ctx)
	if !m.send(attempt) {
		return false
	}

	select {
	case <-attempt.Acked():
		return true
	case <-attempt.Nacked():
		return false
	case <-m.done:
		return false
	case <-b.closing:
		return false
	}
}

// pick memilih member berikutnya secara bergiliran, atau nil kalau group kosong.
func (g *group) pick() *member {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.members) == 0 {
		return nil
	}
	m := g.members[g.next%len(g.members)]
	g.next++
	return m
}

func (g *group) add(m *member) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.members = append(g.members, m)
}

func (g *group) remove(m *member) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, existing := range g.members {
		if existing == m {
			g.members = append(g.members[:i], g.members[i+1:]...)
			return
		}
	}
}

type member struct {
	ctx  context.Context
	out  chan *message.Message
	done chan struct{}

	sendMu    sync.Mutex
	closed    bool
	closeOnce sync.Once
}

func (m *member) send(msg *message.Message) bool {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()

	if m.closed {
		return false
	}
	select {
	case m.out <- msg:
		return true
	case <-m.done:
		return false
	}
}

func (m *member) close() {
	m.closeOnce.Do(func() {
		close(m.done)
		// Kunci ini menjamin tidak ada send yang sedang berjalan saat out ditutup.
		m.sendMu.Lock()
		defer m.sendMu.Unlock()
		m.closed = true
		close(m.out)
	})
}

type groupSubscriber struct {
	broker    *Broker
	groupName string

	mu      sync.Mutex
	members []*member
}

func (s *groupSubscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error) {
	g := s.broker.group(topic, s.groupName)
	m := &member{ctx: ctx, out: make(chan *message.Message), done: make(chan struct{})}
	g.add(m)

	s.mu.Lock()
	s.members = append(s.members, m)
	s.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-m.done:
		case <-s.broker.closing:
		}
		g.remove(m)
		m.close()
	}()

	return m.out, nil
}

func (s *groupSubscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.members {
		m.close()
	}
	return nil
}
