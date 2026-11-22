package lab02

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// KafkaSubscriber adalah Subscriber di atas Kafka.
type KafkaSubscriber struct {
	brokers []string
}

func NewKafkaSubscriber(brokers []string) *KafkaSubscriber {
	return &KafkaSubscriber{brokers: brokers}
}

func (s *KafkaSubscriber) Subscribe(ctx context.Context, topic, group string, handler Handler) error {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(s.brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		// Group yang belum punya offset mulai dari record paling awal.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		// Offset hanya boleh maju setelah handler berhasil, jadi commit
		// dilakukan sendiri di bawah, bukan otomatis oleh client.
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return fmt.Errorf("create kafka client: %w", err)
	}
	defer client.Close()

	for {
		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			return fmt.Errorf("fetch from %s partition %d: %w", errs[0].Topic, errs[0].Partition, errs[0].Err)
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()
			msg := Message{Key: string(record.Key), Value: record.Value}
			if err := handler(ctx, msg); err != nil {
				// Tidak ada commit untuk record ini. Anggota group berikutnya
				// yang mengambil partition ini akan menerimanya lagi.
				return fmt.Errorf("handle message %q: %w", msg.Key, err)
			}
			// shortcut: satu commit per record itu lambat, ganti dengan commit per batch kalau throughput jadi masalah
			if err := client.CommitRecords(ctx, record); err != nil {
				return fmt.Errorf("commit offset %d: %w", record.Offset, err)
			}
		}
	}
}
