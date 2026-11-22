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

	// Dua context terpisah: menghentikan polling tidak boleh ikut membatalkan
	// handler yang sedang berjalan.
	pollCtx       context.Context
	stopPolling   context.CancelFunc
	handlerCtx    context.Context
	cancelHandler context.CancelFunc
	done          chan struct{}
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
	c.pollCtx, c.stopPolling = context.WithCancel(context.Background())
	c.handlerCtx, c.cancelHandler = context.WithCancel(context.Background())
	return c, nil
}

// Run membaca dan memproses record sampai Shutdown dipanggil.
func (c *Consumer) Run() {
	defer close(c.done)
	for {
		fetches := c.cl.PollFetches(c.pollCtx)
		if c.pollCtx.Err() != nil || fetches.IsClientClosed() {
			return
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			c.log.Error("fetch gagal", "topic", topic, "partition", partition, "error", err)
		})
		for _, r := range fetches.Records() {
			// Saat shutdown, sisa batch ditinggalkan tanpa commit. Kafka akan
			// mengirimnya lagi ke consumer berikutnya.
			if c.pollCtx.Err() != nil || !c.process(r) {
				return
			}
		}
	}
}

// process menjalankan handler sampai berhasil, lalu melakukan commit offset.
// Hasil false berarti berhenti tanpa commit: record ini akan dikirim ulang.
func (c *Consumer) process(r *kgo.Record) bool {
	for {
		err := c.handler(c.handlerCtx, toRecord(r))
		if err == nil {
			break
		}
		// Handler gagal: jangan commit dan jangan lanjut ke record berikutnya,
		// karena urutan di partition ini harus terjaga. Coba lagi setelah jeda.
		select {
		case <-c.pollCtx.Done():
			return false
		case <-c.handlerCtx.Done():
			return false
		case <-time.After(c.backoff):
		}
	}

	// Commit setelah handler berhasil. Kalau proses mati sebelum baris ini,
	// record diproses lagi setelah restart: at-least-once.
	// shortcut: satu commit per record itu lambat; ganti ke commit per batch
	// kalau throughput jadi masalah.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.cl.CommitRecords(ctx, r); err != nil {
		c.log.Error("commit offset gagal, record ini mungkin diproses lagi", "offset", r.Offset, "error", err)
	}
	return true
}

// Shutdown berhenti mengambil record baru dan menunggu handler yang sedang
// berjalan sampai selesai. Kalau ctx habis lebih dulu, handler dibatalkan dan
// record-nya tidak di-commit.
func (c *Consumer) Shutdown(ctx context.Context) error {
	c.stopPolling()
	var err error
	select {
	case <-c.done:
	case <-ctx.Done():
		c.cancelHandler()
		<-c.done
		err = fmt.Errorf("consumer %s dihentikan paksa: %w", c.name, ctx.Err())
	}
	c.cancelHandler()
	c.cl.Close()
	return err
}
