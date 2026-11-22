package lab19

import "fmt"

// File ini yang kamu ubah di Step 4 dan Step 5.

// Consume memproses messages secara berurutan. Versi ini berhenti di message
// pertama yang gagal, sehingga semua message di belakangnya ikut tertahan.
func Consume(messages []Message, handle func(Message) error, maxAttempts int, dlq *DeadLetterQueue) error {
	for _, msg := range messages {
		if err := handle(msg); err != nil {
			return fmt.Errorf("message %s: %w", msg.ID, err)
		}
	}
	return nil
}

// Requeue mengembalikan satu message dari dead letter queue ke stream utama
// lewat publish. Versi ini belum melakukan apa pun.
func (q *DeadLetterQueue) Requeue(messageID string, publish func(Message) error) error {
	return nil
}
