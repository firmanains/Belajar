package lab17

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"gopkg.in/yaml.v3"
)

// File ini sudah jadi. Jangan diubah.

const (
	processedMetric = "pms_messages_processed_total"
	durationMetric  = "pms_message_processing_duration_seconds"
	backlogMetric   = "pms_queue_backlog_messages"
	oldestMetric    = "pms_queue_oldest_message_age_seconds"
)

// fakeClock adalah jam yang hanya maju kalau test memajukannya.
type fakeClock struct{ t time.Time }

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)}
}
func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func newMessage(bookingID string) *message.Message {
	msg := message.NewMessage(watermill.NewUUID(), []byte(`{"booking_id":"`+bookingID+`"}`))
	msg.Metadata.Set("booking_id", bookingID)
	return msg
}

var errSMTP = errors.New("SMTP timeout")

// findSeries mencari satu time series berdasarkan nama metric dan label.
func findSeries(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) *dto.Metric {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather gagal: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
	series:
		for _, metric := range family.GetMetric() {
			got := map[string]string{}
			for _, pair := range metric.GetLabel() {
				got[pair.GetName()] = pair.GetValue()
			}
			for k, v := range labels {
				if got[k] != v {
					continue series
				}
			}
			return metric
		}
	}
	return nil
}

func counterValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	metric := findSeries(t, reg, name, labels)
	if metric == nil {
		t.Fatalf("metric %s %v tidak ditemukan di registry", name, labels)
	}
	return metric.GetCounter().GetValue()
}

func gaugeValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	metric := findSeries(t, reg, name, labels)
	if metric == nil {
		t.Fatalf("metric %s %v tidak ditemukan di registry", name, labels)
	}
	return metric.GetGauge().GetValue()
}

// Dunia ideal: middleware tidak mengubah perilaku handler. Hasil dan error
// diteruskan apa adanya, dan handler dipanggil tepat sekali.
func TestStep0_MiddlewareDoesNotChangeBehaviour(t *testing.T) {
	clock := newFakeClock()
	metrics := NewMetrics(prometheus.NewRegistry(), clock.Now)

	calls := 0
	reply := newMessage("B-reply")
	handler := metrics.Middleware("send_confirmation_email")(func(msg *message.Message) ([]*message.Message, error) {
		calls++
		return []*message.Message{reply}, nil
	})

	produced, err := handler(newMessage("B-1"))
	if err != nil {
		t.Fatalf("handler mengembalikan error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("handler dipanggil %d kali, seharusnya 1", calls)
	}
	if len(produced) != 1 || produced[0] != reply {
		t.Fatalf("message hasil handler tidak diteruskan apa adanya")
	}

	failing := metrics.Middleware("send_confirmation_email")(func(msg *message.Message) ([]*message.Message, error) {
		return nil, errSMTP
	})
	if _, err := failing(newMessage("B-2")); !errors.Is(err, errSMTP) {
		t.Fatalf("error dari handler = %v, seharusnya diteruskan apa adanya", err)
	}
}

// Dua message berhasil dan satu gagal. Tanpa counter, kegagalan itu hanya
// terlihat kalau ada yang kebetulan membaca log.
func TestStep1_CountsProcessedMessagesByOutcome(t *testing.T) {
	clock := newFakeClock()
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg, clock.Now)

	attempt := 0
	handler := metrics.Middleware("send_confirmation_email")(func(msg *message.Message) ([]*message.Message, error) {
		attempt++
		if attempt == 2 {
			return nil, errSMTP
		}
		return nil, nil
	})
	for i := 1; i <= 3; i++ {
		_, _ = handler(newMessage(fmt.Sprintf("B-%d", i)))
	}

	labels := map[string]string{"handler": "send_confirmation_email"}
	labels["outcome"] = "success"
	if got := counterValue(t, reg, processedMetric, labels); got != 2 {
		t.Fatalf("%s outcome=success = %v, seharusnya 2", processedMetric, got)
	}
	labels["outcome"] = "failure"
	if got := counterValue(t, reg, processedMetric, labels); got != 1 {
		t.Fatalf("%s outcome=failure = %v, seharusnya 1", processedMetric, got)
	}
}

// Setiap message memakan 250 ms. Histogram harus mencatat ketiganya,
// termasuk yang gagal: handler yang gagal setelah timeout 30 detik adalah
// data yang paling ingin dilihat.
func TestStep2_ObservesProcessingDuration(t *testing.T) {
	clock := newFakeClock()
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg, clock.Now)

	attempt := 0
	handler := metrics.Middleware("sync_channel_manager")(func(msg *message.Message) ([]*message.Message, error) {
		clock.Advance(250 * time.Millisecond)
		attempt++
		if attempt == 3 {
			return nil, errors.New("channel manager HTTP 503")
		}
		return nil, nil
	})
	for i := 1; i <= 3; i++ {
		_, _ = handler(newMessage(fmt.Sprintf("B-%d", i)))
	}

	metric := findSeries(t, reg, durationMetric, map[string]string{"handler": "sync_channel_manager"})
	if metric == nil || metric.GetHistogram() == nil {
		t.Fatalf("histogram %s{handler=\"sync_channel_manager\"} tidak ditemukan di registry", durationMetric)
	}
	histogram := metric.GetHistogram()
	if got := histogram.GetSampleCount(); got != 3 {
		t.Fatalf("histogram mencatat %d pengamatan, seharusnya 3: message yang gagal juga harus diukur", got)
	}
	if got := histogram.GetSampleSum(); got < 0.74 || got > 0.76 {
		t.Fatalf("total durasi = %v detik, seharusnya 0.75: satuan harus detik dan diukur dengan jam yang disuntikkan", got)
	}
}

// 50 booking berbeda diproses oleh dua handler. Jumlah time series tidak
// boleh ikut tumbuh bersama jumlah booking.
func TestStep3_LabelCardinalityStaysBounded(t *testing.T) {
	clock := newFakeClock()
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg, clock.Now)

	for _, name := range []string{"send_confirmation_email", "sync_channel_manager"} {
		handler := metrics.Middleware(name)(func(msg *message.Message) ([]*message.Message, error) {
			if strings.HasSuffix(msg.Metadata.Get("booking_id"), "7") {
				return nil, errSMTP
			}
			return nil, nil
		})
		for i := 1; i <= 50; i++ {
			_, _ = handler(newMessage(fmt.Sprintf("B-%d", i)))
		}
	}

	series, err := testutil.GatherAndCount(reg, processedMetric)
	if err != nil {
		t.Fatalf("gather gagal: %v", err)
	}
	if series == 0 {
		t.Fatalf("%s belum ada di registry", processedMetric)
	}
	if series > 4 {
		t.Fatalf("%s punya %d time series untuk 2 handler x 2 outcome, seharusnya paling banyak 4: ada label yang nilainya tumbuh bersama data", processedMetric, series)
	}

	problems, err := testutil.GatherAndLint(reg)
	if err != nil {
		t.Fatalf("lint gagal: %v", err)
	}
	for _, problem := range problems {
		t.Errorf("penamaan metric %s melanggar konvensi Prometheus: %s", problem.Metric, problem.Text)
	}
}

// Consumer berhenti total. Counter berhenti naik, dan itu terlihat sama
// dengan "tidak ada booking malam ini". Yang membedakan keduanya adalah
// kondisi antrean.
func TestStep4_ExposesBacklogAndOldestMessageAge(t *testing.T) {
	clock := newFakeClock()
	reg := prometheus.NewRegistry()
	backlog := &Backlog{}
	if err := RegisterQueueMetrics(reg, "confirmation_emails", backlog, clock.Now); err != nil {
		t.Fatalf("RegisterQueueMetrics gagal: %v", err)
	}
	labels := map[string]string{"queue": "confirmation_emails"}

	if got := gaugeValue(t, reg, oldestMetric, labels); got != 0 {
		t.Fatalf("antrean kosong: %s = %v, seharusnya 0", oldestMetric, got)
	}

	for i := 0; i < 3; i++ {
		backlog.Enqueue(clock.Now())
	}
	clock.Advance(90 * time.Second)
	backlog.Enqueue(clock.Now())

	if got := gaugeValue(t, reg, backlogMetric, labels); got != 4 {
		t.Fatalf("%s = %v, seharusnya 4", backlogMetric, got)
	}
	if got := gaugeValue(t, reg, oldestMetric, labels); got != 90 {
		t.Fatalf("%s = %v, seharusnya 90: nilainya harus dihitung saat registry dibaca, bukan saat didaftarkan", oldestMetric, got)
	}

	// Tidak ada yang memproses. Lima menit kemudian umur message tertua harus
	// ikut naik walaupun tidak ada satu baris kode consumer pun yang berjalan.
	clock.Advance(5 * time.Minute)
	if got := gaugeValue(t, reg, oldestMetric, labels); got != 390 {
		t.Fatalf("%s = %v, seharusnya 390 setelah consumer diam 5 menit", oldestMetric, got)
	}

	backlog.Dequeue(3)
	if got := gaugeValue(t, reg, backlogMetric, labels); got != 1 {
		t.Fatalf("%s = %v, seharusnya 1 setelah tiga message diproses", backlogMetric, got)
	}
	if got := gaugeValue(t, reg, oldestMetric, labels); got != 300 {
		t.Fatalf("%s = %v, seharusnya 300", oldestMetric, got)
	}
}

type ruleFile struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []rule `yaml:"rules"`
	} `yaml:"groups"`
}

type rule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

// Test ini hanya memeriksa struktur alerts.yaml. Ia tidak menjalankan
// Prometheus, jadi sintaks PromQL tidak divalidasi.
func TestStep5_AlertRulesAreSymptomBased(t *testing.T) {
	raw, err := os.ReadFile("alerts.yaml")
	if err != nil {
		t.Fatalf("membaca alerts.yaml: %v", err)
	}
	var file ruleFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("alerts.yaml bukan YAML yang valid: %v", err)
	}
	rules := map[string]rule{}
	for _, group := range file.Groups {
		for _, r := range group.Rules {
			rules[r.Alert] = r
		}
	}

	if _, found := rules["HighCPU"]; found {
		t.Errorf("alert HighCPU masih ada: ia menembak berdasarkan penyebab, bukan gejala")
	}

	required := []struct {
		name     string
		contains []string
		why      string
	}{
		{"OldestMessageTooOld", []string{oldestMetric, ">"}, "bandingkan umur message tertua dengan ambang batas"},
		{"HighFailureRatio", []string{processedMetric, `outcome="failure"`, "rate(", "/"}, "harus berupa rasio gagal terhadap total, bukan jumlah mentah"},
		{"DeadLetterGrowing", []string{"pms_dead_letter_messages_total", "increase("}, "pakai increase() pada counter dead letter"},
	}
	for _, want := range required {
		r, found := rules[want.name]
		if !found {
			t.Errorf("alert %s belum ada di alerts.yaml", want.name)
			continue
		}
		for _, fragment := range want.contains {
			if !strings.Contains(r.Expr, fragment) {
				t.Errorf("%s: expr tidak memuat %q (%s)", want.name, fragment, want.why)
			}
		}
		if strings.Count(r.Expr, "(") != strings.Count(r.Expr, ")") {
			t.Errorf("%s: jumlah kurung buka dan tutup di expr tidak sama", want.name)
		}
		hold, err := time.ParseDuration(r.For)
		if err != nil || hold < time.Minute {
			t.Errorf("%s: `for` = %q, seharusnya durasi minimal 1m supaya lonjakan sesaat tidak membangunkan orang", want.name, r.For)
		}
		if severity := r.Labels["severity"]; severity != "page" && severity != "ticket" {
			t.Errorf("%s: labels.severity = %q, seharusnya page atau ticket", want.name, severity)
		}
		if r.Annotations["summary"] == "" {
			t.Errorf("%s: annotations.summary kosong", want.name)
		}
		if r.Annotations["runbook"] == "" {
			t.Errorf("%s: annotations.runbook kosong: alert tanpa langkah pertama hanya memindahkan kepanikan", want.name)
		}
	}
}
