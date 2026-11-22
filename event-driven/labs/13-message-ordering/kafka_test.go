package lab13

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// File ini sudah jadi. Jangan diubah.

const partitions = 3

func brokers() []string {
	if v := os.Getenv("ED_KAFKA_BROKERS"); v != "" {
		return strings.Split(v, ",")
	}
	return []string{"localhost:59092"}
}

// newTopic membuat topic dengan 3 partition dan nama unik, lalu menghapusnya
// saat test selesai.
func newTopic(t *testing.T) (*kgo.Client, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers()...))
	if err != nil {
		t.Fatalf("membuat Kafka client: %v", err)
	}
	t.Cleanup(cl.Close)
	if err := cl.Ping(ctx); err != nil {
		t.Fatalf("Kafka tidak terjangkau di %v (%v): jalankan `make up` di root kursus", brokers(), err)
	}

	topic := fmt.Sprintf("lab13-bookings-%d", rand.Uint64())
	adm := kadm.NewClient(cl)
	resp, err := adm.CreateTopic(ctx, partitions, 1, nil, topic)
	if err != nil {
		t.Fatalf("membuat topic %s: %v", topic, err)
	}
	if resp.Err != nil {
		t.Fatalf("membuat topic %s: %v", topic, resp.Err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := adm.DeleteTopics(ctx, topic); err != nil {
			t.Logf("menghapus topic %s: %v", topic, err)
		}
	})
	return cl, topic
}

type received struct {
	partition int32
	offset    int64
	event     BookingEvent
}

// consumeAll membaca n record dari awal topic. Record dari satu partition
// selalu dikembalikan dalam urutan offset.
func consumeAll(t *testing.T, topic string, n int) []received {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers()...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("membuat Kafka consumer: %v", err)
	}
	defer cl.Close()

	var out []received
	for len(out) < n {
		fetches := cl.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			t.Fatalf("membaca topic %s (baru %d dari %d record): %v", topic, len(out), n, err)
		}
		fetches.EachRecord(func(r *kgo.Record) {
			var ev BookingEvent
			if err := json.Unmarshal(r.Value, &ev); err != nil {
				t.Fatalf("record di partition %d offset %d bukan BookingEvent: %v", r.Partition, r.Offset, err)
			}
			out = append(out, received{partition: r.Partition, offset: r.Offset, event: ev})
		})
	}
	return out
}

// Empat booking, masing-masing lima revisi, dikirim berselang-seling ke topic
// dengan 3 partition. Setiap revisi satu booking harus berada di partition yang
// sama, karena Kafka hanya menjaga urutan di dalam satu partition.
func TestStep1_SameBookingSamePartition(t *testing.T) {
	cl, topic := newTopic(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bookings := []string{"B-101", "B-102", "B-103", "B-104"}
	const revisions = 5
	for rev := 1; rev <= revisions; rev++ {
		for _, id := range bookings {
			ev := BookingEvent{
				EventID:   fmt.Sprintf("evt-%s-%d", id, rev),
				Name:      BookingModified,
				BookingID: id,
				Revision:  rev,
				Rooms:     rev,
			}
			if rev == 1 {
				ev.Name = BookingCreated
			}
			rec, err := RecordFor(topic, ev)
			if err != nil {
				t.Fatalf("RecordFor: %v", err)
			}
			// Satu per satu dan sinkron, supaya urutan kirim pasti.
			if err := cl.ProduceSync(ctx, rec).FirstErr(); err != nil {
				t.Fatalf("produce %s: %v", ev.EventID, err)
			}
		}
	}

	records := consumeAll(t, topic, len(bookings)*revisions)

	// Tampilkan isi tiap partition supaya terlihat dengan `go test -v`.
	byPartition := map[int32][]string{}
	for _, r := range records {
		byPartition[r.partition] = append(byPartition[r.partition], fmt.Sprintf("%s#%d", r.event.BookingID, r.event.Revision))
	}
	for p := int32(0); p < partitions; p++ {
		t.Logf("partition %d: %s", p, strings.Join(byPartition[p], " "))
	}

	partitionsOf := map[string]map[int32]bool{}
	for _, r := range records {
		if partitionsOf[r.event.BookingID] == nil {
			partitionsOf[r.event.BookingID] = map[int32]bool{}
		}
		partitionsOf[r.event.BookingID][r.partition] = true
	}
	for _, id := range bookings {
		if got := len(partitionsOf[id]); got != 1 {
			var ps []int
			for p := range partitionsOf[id] {
				ps = append(ps, int(p))
			}
			sort.Ints(ps)
			t.Fatalf("revisi booking %s tersebar di %d partition %v, seharusnya 1: consumer partition yang berbeda bisa memproses revisi baru sebelum revisi lama", id, got, ps)
		}
	}

	// Di dalam satu partition, revisi setiap booking harus naik.
	last := map[string]int{}
	for _, r := range records {
		if r.event.Revision <= last[r.event.BookingID] {
			t.Fatalf("booking %s: revision %d dibaca setelah revision %d", r.event.BookingID, r.event.Revision, last[r.event.BookingID])
		}
		last[r.event.BookingID] = r.event.Revision
	}
}
