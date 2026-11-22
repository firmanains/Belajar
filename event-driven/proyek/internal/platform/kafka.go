package platform

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Faults dipakai test untuk menyuntikkan kegagalan secara deterministik.
// Nilai N berarti N panggilan berikutnya gagal. Di production nilainya nol.
type Faults struct {
	OutboxInsert atomic.Int64
	Publish      atomic.Int64
}

func take(c *atomic.Int64) bool {
	for {
		n := c.Load()
		if n <= 0 {
			return false
		}
		if c.CompareAndSwap(n, n-1) {
			return true
		}
	}
}

func (f *Faults) FailOutboxInsert() bool { return f != nil && take(&f.OutboxInsert) }
func (f *Faults) FailPublish() bool      { return f != nil && take(&f.Publish) }

// Record adalah satu record yang diterima consumer.
type Record struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   map[string]string
}

// Handler memproses satu record. Return nil berarti selesai dan offset boleh
// di-commit. Return error berarti record ini belum selesai diproses.
type Handler func(ctx context.Context, r Record) error

func toRecord(r *kgo.Record) Record {
	h := make(map[string]string, len(r.Headers))
	for _, kv := range r.Headers {
		h[kv.Key] = string(kv.Value)
	}
	return Record{Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Key: r.Key, Value: r.Value, Headers: h}
}

// Message adalah satu record yang akan dikirim ke Kafka.
type Message struct {
	Key     []byte
	Value   []byte
	Headers map[string]string
}

// Publisher menyembunyikan broker dari kode bisnis.
type Publisher interface {
	Publish(ctx context.Context, topic string, m Message) error
}

// KafkaPublisher mengirim record dan menunggu ack dari broker.
type KafkaPublisher struct {
	cl     *kgo.Client
	faults *Faults
}

func NewKafkaPublisher(brokers []string, faults *Faults) (*KafkaPublisher, error) {
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		return nil, fmt.Errorf("kafka publisher: %w", err)
	}
	return &KafkaPublisher{cl: cl, faults: faults}, nil
}

// Publish baru kembali setelah broker menyimpan record. Record dengan key yang
// sama selalu masuk partition yang sama, sehingga urutannya terjaga.
func (p *KafkaPublisher) Publish(ctx context.Context, topic string, m Message) error {
	if p.faults.FailPublish() {
		return errors.New("fault injection: broker tidak terjangkau")
	}
	r := &kgo.Record{Topic: topic, Key: m.Key, Value: m.Value}
	for k, v := range m.Headers {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	if err := p.cl.ProduceSync(ctx, r).FirstErr(); err != nil {
		return fmt.Errorf("publish ke %s: %w", topic, err)
	}
	return nil
}

func (p *KafkaPublisher) Close() { p.cl.Close() }

// EnsureTopics membuat topic yang belum ada. Auto-create di broker dimatikan,
// jadi jumlah partition selalu keputusan yang disengaja.
func EnsureTopics(ctx context.Context, brokers []string, partitions map[string]int32) error {
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.MetadataMinAge(50*time.Millisecond))
	if err != nil {
		return fmt.Errorf("kafka admin: %w", err)
	}
	defer cl.Close()
	adm := kadm.NewClient(cl)
	for topic, n := range partitions {
		res, err := adm.CreateTopics(ctx, n, 1, nil, topic)
		if err != nil {
			return fmt.Errorf("create topic %s: %w", topic, err)
		}
		if err := res.Error(); err != nil && !errors.Is(err, kerr.TopicAlreadyExists) {
			return fmt.Errorf("create topic %s: %w", topic, err)
		}
	}
	// Topic yang baru dibuat belum langsung punya leader. Client yang
	// menyambung terlalu cepat akan menunggu refresh metadata berikutnya
	// (5 detik), jadi tunggu di sini sampai semua partition siap.
	for topic, n := range partitions {
		for !topicReady(ctx, adm, topic, n) {
			select {
			case <-ctx.Done():
				return fmt.Errorf("menunggu topic %s siap: %w", topic, ctx.Err())
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	return nil
}

func topicReady(ctx context.Context, adm *kadm.Client, topic string, partitions int32) bool {
	details, err := adm.ListTopics(ctx, topic)
	if err != nil {
		return false
	}
	d, ok := details[topic]
	if !ok || d.Err != nil || len(d.Partitions) != int(partitions) {
		return false
	}
	for _, p := range d.Partitions {
		if p.Err != nil || p.Leader < 0 {
			return false
		}
	}
	return true
}
