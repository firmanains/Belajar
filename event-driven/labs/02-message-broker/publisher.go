package lab02

// File ini sudah jadi. Jangan diubah; baca sebagai contoh pemakaian franz-go.

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// KafkaPublisher adalah Publisher di atas Kafka.
type KafkaPublisher struct {
	client *kgo.Client
}

func NewKafkaPublisher(brokers []string) (*KafkaPublisher, error) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		return nil, fmt.Errorf("create kafka client: %w", err)
	}
	return &KafkaPublisher{client: client}, nil
}

// Publish menunggu sampai broker mengonfirmasi semua record sudah tersimpan.
// Record dengan key yang sama selalu masuk ke partition yang sama.
func (p *KafkaPublisher) Publish(ctx context.Context, topic string, msgs ...Message) error {
	records := make([]*kgo.Record, 0, len(msgs))
	for _, m := range msgs {
		records = append(records, &kgo.Record{Topic: topic, Key: []byte(m.Key), Value: m.Value})
	}
	if err := p.client.ProduceSync(ctx, records...).FirstErr(); err != nil {
		return fmt.Errorf("publish to %s: %w", topic, err)
	}
	return nil
}

func (p *KafkaPublisher) Close() { p.client.Close() }
