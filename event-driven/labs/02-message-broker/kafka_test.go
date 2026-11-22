package lab02

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const waitTimeout = 20 * time.Second

func brokers() []string {
	if v := os.Getenv("ED_KAFKA_BROKERS"); v != "" {
		return strings.Split(v, ",")
	}
	return []string{"localhost:59092"}
}

func randomSuffix() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// env adalah perlengkapan satu test: admin client, publisher, dan satu topic
// dengan nama unik.
type env struct {
	t     *testing.T
	admin *kadm.Client
	pub   *KafkaPublisher
	topic string
}

func newEnv(t *testing.T, partitions int32) *env {
	t.Helper()
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers()...))
	if err != nil {
		t.Fatalf("membuat kafka client: %v", err)
	}
	admin := kadm.NewClient(client)
	t.Cleanup(admin.Close)

	topic := "lab02-" + randomSuffix()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	if _, err := admin.CreateTopic(ctx, partitions, 1, nil, topic); err != nil {
		t.Fatalf("Kafka tidak terjangkau di %v. Jalankan `make up` di root kursus. (%v)", brokers(), err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
		defer cancel()
		_, _ = admin.DeleteTopics(ctx, topic)
	})

	pub, err := NewKafkaPublisher(brokers())
	if err != nil {
		t.Fatalf("membuat publisher: %v", err)
	}
	t.Cleanup(pub.Close)
	return &env{t: t, admin: admin, pub: pub, topic: topic}
}

func (e *env) group(name string) string { return e.topic + "-" + name }

func (e *env) publish(keys ...string) {
	e.t.Helper()
	msgs := make([]Message, 0, len(keys))
	for _, k := range keys {
		msgs = append(msgs, Message{Key: k, Value: []byte(`{"booking_id":"` + k + `"}`)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	if err := e.pub.Publish(ctx, e.topic, msgs...); err != nil {
		e.t.Fatalf("publish gagal: %v", err)
	}
}

// consumer menjalankan satu Subscribe di goroutine dan mencatat key yang
// diterima handler-nya.
type consumer struct {
	cancel context.CancelFunc
	done   chan error

	mu   sync.Mutex
	keys []string
}

// start menjalankan Subscribe. Kalau fail tidak nil, handler mengembalikan
// error untuk pesan yang membuat fail(key) benar.
func (e *env) start(group string, fail func(key string) bool) *consumer {
	ctx, cancel := context.WithCancel(context.Background())
	c := &consumer{cancel: cancel, done: make(chan error, 1)}
	sub := NewKafkaSubscriber(brokers())
	go func() {
		c.done <- sub.Subscribe(ctx, e.topic, group, func(_ context.Context, msg Message) error {
			c.mu.Lock()
			c.keys = append(c.keys, msg.Key)
			c.mu.Unlock()
			if fail != nil && fail(msg.Key) {
				return errors.New("proses mati di tengah handler")
			}
			return nil
		})
	}()
	e.t.Cleanup(func() { c.stop(e.t) })
	return c
}

func (c *consumer) received() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.keys)
}

// stop membatalkan ctx dan menunggu Subscribe kembali. Aman dipanggil dua kali.
func (c *consumer) stop(t *testing.T) {
	t.Helper()
	c.cancel()
	select {
	case err, ok := <-c.done:
		if ok && err != nil {
			t.Errorf("Subscribe mengembalikan error setelah ctx dibatalkan: %v", err)
		}
		if ok {
			close(c.done)
		}
	case <-time.After(waitTimeout):
		t.Errorf("Subscribe tidak kembali setelah ctx dibatalkan")
	}
}

func eventually(cond func() bool) bool {
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

// partitionsPerMember melaporkan jumlah partition milik tiap anggota group,
// atau nil kalau group belum stabil.
func (e *env) partitionsPerMember(group string) []int {
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	groups, err := e.admin.DescribeGroups(ctx, group)
	if err != nil {
		return nil
	}
	g, ok := groups[group]
	if !ok || g.State != "Stable" {
		return nil
	}
	var counts []int
	for _, m := range g.Members {
		n := 0
		if assignment, ok := m.Assigned.AsConsumer(); ok {
			for _, topic := range assignment.Topics {
				n += len(topic.Partitions)
			}
		}
		counts = append(counts, n)
	}
	return counts
}

func bookingKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("B-%d", i+1)
	}
	return keys
}

// Publisher yang sudah jadi bisa menitipkan pesan ke Kafka, dan pesannya
// memang tersimpan di sana. Test ini membacanya langsung dengan franz-go.
func TestStep0_PublishedMessagesAreStoredInKafka(t *testing.T) {
	e := newEnv(t, 1)
	e.publish("B-1", "B-2", "B-3")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers()...),
		kgo.ConsumeTopics(e.topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("membuat kafka client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	var got []string
	for len(got) < 3 && ctx.Err() == nil {
		client.PollFetches(ctx).EachRecord(func(r *kgo.Record) { got = append(got, string(r.Key)) })
	}
	if want := []string{"B-1", "B-2", "B-3"}; !slices.Equal(got, want) {
		t.Fatalf("isi topic = %v, seharusnya %v", got, want)
	}
}

// Subscriber menerima semua pesan di topic, sesuai urutan.
func TestStep1_SubscriberReceivesMessages(t *testing.T) {
	e := newEnv(t, 1)
	want := bookingKeys(5)
	e.publish(want...)

	c := e.start(e.group("email"), nil)
	if !eventually(func() bool { return len(c.received()) >= len(want) }) {
		t.Fatalf("handler menerima %d dari %d pesan: %v", len(c.received()), len(want), c.received())
	}
	if got := c.received(); !slices.Equal(got, want) {
		t.Fatalf("handler menerima %v, seharusnya %v", got, want)
	}
}

// Dua consumer dalam satu group membagi pekerjaan: setiap pesan dikerjakan
// oleh tepat satu dari mereka.
func TestStep2_OneGroupSplitsTheWork(t *testing.T) {
	e := newEnv(t, 4)
	group := e.group("email")
	a := e.start(group, nil)
	b := e.start(group, nil)

	// Tunggu sampai Kafka selesai membagi 4 partition ke 2 anggota.
	if !eventually(func() bool {
		counts := e.partitionsPerMember(group)
		return len(counts) == 2 && counts[0] > 0 && counts[1] > 0 && counts[0]+counts[1] == 4
	}) {
		t.Fatalf("consumer group belum punya 2 anggota yang masing-masing memegang partition (partition per anggota: %v)", e.partitionsPerMember(group))
	}

	want := bookingKeys(40)
	e.publish(want...)
	if !eventually(func() bool { return len(a.received())+len(b.received()) >= len(want) }) {
		t.Fatalf("kedua consumer menerima %d dari %d pesan", len(a.received())+len(b.received()), len(want))
	}

	gotA, gotB := a.received(), b.received()
	if len(gotA) == 0 || len(gotB) == 0 {
		t.Fatalf("pekerjaan tidak terbagi: consumer A %d pesan, consumer B %d pesan", len(gotA), len(gotB))
	}
	all := slices.Sorted(slices.Values(append(gotA, gotB...)))
	if !slices.Equal(all, slices.Sorted(slices.Values(want))) {
		t.Fatalf("ada pesan yang dikerjakan dua kali atau terlewat: A=%v B=%v", gotA, gotB)
	}
}

// Dua group yang berbeda masing-masing menerima semua pesan. Inilah
// publish/subscribe: satu event, banyak pihak yang berkepentingan.
func TestStep3_EachGroupGetsEverything(t *testing.T) {
	e := newEnv(t, 1)
	want := bookingKeys(10)
	e.publish(want...)

	email := e.start(e.group("email"), nil)
	channel := e.start(e.group("channel-sync"), nil)
	if !eventually(func() bool { return len(email.received()) >= 10 && len(channel.received()) >= 10 }) {
		t.Fatalf("group email menerima %d pesan dan group channel-sync %d pesan, seharusnya masing-masing 10", len(email.received()), len(channel.received()))
	}
	if got := email.received(); !slices.Equal(got, want) {
		t.Fatalf("group email menerima %v, seharusnya %v", got, want)
	}
	if got := channel.received(); !slices.Equal(got, want) {
		t.Fatalf("group channel-sync menerima %v, seharusnya %v", got, want)
	}
}

// Consumer mati saat memproses B-2, sebelum sempat commit. Consumer pengganti
// di group yang sama harus melanjutkan dari B-2: tidak mengulang B-1, tidak
// melompati B-2.
func TestStep4_UncommittedMessageIsRedelivered(t *testing.T) {
	e := newEnv(t, 1)
	group := e.group("email")
	e.publish("B-1", "B-2", "B-3")

	first := e.start(group, func(key string) bool { return key == "B-2" })
	select {
	case err := <-first.done:
		close(first.done)
		if err == nil {
			t.Fatal("Subscribe kembali tanpa error, padahal handler gagal di B-2")
		}
	case <-time.After(waitTimeout):
		t.Fatalf("Subscribe belum kembali padahal handler seharusnya gagal di B-2 (handler menerima %v)", first.received())
	}
	if got, want := first.received(), []string{"B-1", "B-2"}; !slices.Equal(got, want) {
		t.Fatalf("consumer pertama menerima %v, seharusnya %v lalu berhenti", got, want)
	}

	second := e.start(group, nil)
	want := []string{"B-2", "B-3"}
	if !eventually(func() bool { return len(second.received()) >= len(want) }) {
		t.Fatalf("consumer pengganti menerima %v, seharusnya %v: offset sudah maju melewati pesan yang gagal diproses, jadi pesan itu hilang", second.received(), want)
	}
	if got := second.received(); !slices.Equal(got, want) {
		t.Fatalf("consumer pengganti menerima %v, seharusnya %v", got, want)
	}
}
