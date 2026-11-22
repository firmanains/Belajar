package lab02

// File ini yang kamu ubah.

import "context"

// KafkaSubscriber adalah Subscriber di atas Kafka.
type KafkaSubscriber struct {
	brokers []string
}

func NewKafkaSubscriber(brokers []string) *KafkaSubscriber {
	return &KafkaSubscriber{brokers: brokers}
}

// Subscribe belum membaca apa pun: ia hanya menunggu sampai ctx dibatalkan.
func (s *KafkaSubscriber) Subscribe(ctx context.Context, topic, group string, handler Handler) error {
	<-ctx.Done()
	return nil
}
