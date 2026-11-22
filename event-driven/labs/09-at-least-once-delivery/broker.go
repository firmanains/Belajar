package lab09

import "fmt"

// File ini sudah jadi. Jangan diubah; baca saja.

// BookingCreated adalah event: fakta bahwa sebuah booking sudah terjadi.
type BookingCreated struct {
	BookingID string
	RoomType  string
	Rooms     int
}

// Message adalah amplop event. ID dibuat pengirim setiap kali ia mengirim,
// jadi satu booking bisa datang dalam dua Message dengan ID berbeda.
type Message struct {
	ID    string
	Event BookingCreated
}

// Handler adalah kode consumer. Return nil berarti ack: "sudah kuproses".
type Handler func(Message) error

// Broker tiruan dengan jaminan at-least-once, seperti Kafka dan RabbitMQ:
// message dikirim ulang sampai broker menerima ack.
type Broker struct {
	// LoseFirstAck meniru ack yang hilang di jaringan: handler sudah berhasil,
	// tapi broker tidak tahu, jadi message yang sama dikirim sekali lagi.
	LoseFirstAck bool

	queue []Message
}

const maxAttempts = 5

func (b *Broker) Publish(m Message) { b.queue = append(b.queue, m) }

// Run mengantar semua message ke handler. Message yang gagal terus sampai
// maxAttempts dianggap mati (lab 19 membahas ke mana message seperti ini pergi).
func (b *Broker) Run(h Handler) error {
	for _, m := range b.queue {
		ackLost := b.LoseFirstAck
		for attempt := 1; ; attempt++ {
			if attempt > maxAttempts {
				return fmt.Errorf("message %s gagal %d kali dan dianggap mati", m.ID, maxAttempts)
			}
			if err := h(m); err != nil {
				continue // handler gagal: redelivery
			}
			if ackLost {
				ackLost = false
				continue // handler berhasil, tapi ack hilang: redelivery
			}
			break
		}
	}
	b.queue = nil
	return nil
}
