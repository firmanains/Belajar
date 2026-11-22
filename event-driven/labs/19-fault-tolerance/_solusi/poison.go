package lab19

import (
	"errors"
	"fmt"
)

// Consume memproses messages secara berurutan. Message yang gagal dicoba
// sampai maxAttempts kali, lalu dipinggirkan ke dead letter queue supaya
// message di belakangnya tetap mengalir. Error permanent langsung dipinggirkan.
func Consume(messages []Message, handle func(Message) error, maxAttempts int, dlq *DeadLetterQueue) error {
	for _, msg := range messages {
		var err error
		attempts := 0
		for attempts < maxAttempts {
			attempts++
			if err = handle(msg); err == nil || errors.Is(err, ErrPermanent) {
				break
			}
		}
		if err != nil {
			dlq.Add(DeadLetter{Message: msg, Reason: err.Error(), Attempts: attempts})
		}
	}
	return nil
}

// Requeue mengembalikan satu message dari dead letter queue ke stream utama.
// Publish dulu, hapus kemudian: kalau proses mati di antaranya, message
// terkirim dua kali, tidak hilang.
func (q *DeadLetterQueue) Requeue(messageID string, publish func(Message) error) error {
	for i, entry := range q.entries {
		if entry.Message.ID != messageID {
			continue
		}
		if err := publish(entry.Message); err != nil {
			return fmt.Errorf("requeue message %s: %w", messageID, err)
		}
		q.entries = append(q.entries[:i], q.entries[i+1:]...)
		return nil
	}
	return fmt.Errorf("requeue message %s: %w", messageID, ErrNotInDeadLetterQueue)
}
