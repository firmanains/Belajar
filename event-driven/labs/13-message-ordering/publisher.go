package lab13

import (
	"encoding/json"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// File ini yang kamu ubah di Step 1.

// RecordFor membungkus sebuah event menjadi record Kafka.
// Key menentukan partition: record dengan key yang sama selalu masuk ke
// partition yang sama.
func RecordFor(topic string, ev BookingEvent) (*kgo.Record, error) {
	value, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal event %s: %w", ev.EventID, err)
	}
	return &kgo.Record{
		Topic: topic,
		Key:   []byte(ev.EventID),
		Value: value,
	}, nil
}
