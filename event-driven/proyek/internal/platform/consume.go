package platform

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Consumer membaca satu topic sebagai anggota satu consumer group dan
// menyerahkan record satu per satu ke handler.
type Consumer struct {
	name    string
	cl      *kgo.Client
	handler Handler
	backoff time.Duration
	log     *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewConsumer(brokers []string, group, topic, name string, backoff time.Duration, handler Handler, log *slog.Logger) (*Consumer, error) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), // group baru membaca dari awal topic
		kgo.DisableAutoCommit(),                           // offset di-commit oleh kode di bawah
		// Broker menahan fetch kosong paling lama selama ini. Dengan default
		// 5 detik, record pertama setelah start kadang baru diterima 5 detik
		// kemudian (teramati di test; penyebab pastinya belum ditelusuri).
		kgo.FetchMaxWait(500*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer %s: %w", name, err)
	}
	c := &Consumer{name: name, cl: cl, handler: handler, backoff: backoff, log: log.With("consumer", name), done: make(chan struct{})}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	return c, nil
}

// Run membaca dan memproses record sampai Shutdown dipanggil.
//
// TODO(step 8): versi ini melakukan commit offset sebelum handler berjalan.
// Kalau proses mati di tengah handler, Kafka menganggap record itu sudah
// selesai dan tidak pernah mengirimnya lagi: at-most-once. Ubah supaya
//  1. commit dilakukan setelah handler berhasil;
//  2. handler yang gagal dicoba lagi setelah c.backoff, tanpa commit dan tanpa
//     melompat ke record berikutnya.
func (c *Consumer) Run() {
	defer close(c.done)
	for {
		fetches := c.cl.PollFetches(c.ctx)
		if c.ctx.Err() != nil || fetches.IsClientClosed() {
			return
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			c.log.Error("fetch gagal", "topic", topic, "partition", partition, "error", err)
		})
		for _, r := range fetches.Records() {
			if err := c.cl.CommitRecords(c.ctx, r); err != nil {
				c.log.Error("commit offset gagal", "offset", r.Offset, "error", err)
			}
			if err := c.handler(c.ctx, toRecord(r)); err != nil {
				c.log.Error("handler gagal, record dilewati", "offset", r.Offset, "error", err)
			}
		}
	}
}

// Shutdown menghentikan consumer.
//
// TODO(step 8): versi ini membatalkan handler yang sedang berjalan. Ubah supaya
// Shutdown berhenti mengambil record baru, lalu menunggu handler yang sedang
// berjalan sampai selesai. Hanya kalau ctx habis lebih dulu, handler dibatalkan
// dan error dikembalikan. Petunjuk: polling dan handler butuh context yang
// berbeda.
func (c *Consumer) Shutdown(ctx context.Context) error {
	c.cancel()
	<-c.done
	c.cl.Close()
	return nil
}
