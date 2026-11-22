package lab07

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// eventually melaporkan apakah cond menjadi true sebelum timeout habis.
// Ia memeriksa berulang-ulang dengan jeda pendek: selesai secepat kondisinya
// terpenuhi, dan baru menyerah setelah timeout.
func eventually(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// fakeChannelManager adalah channel manager tiruan: HTTP server sungguhan di
// localhost yang merekam apa yang ia terima.
type fakeChannelManager struct {
	server *httptest.Server

	// failFirst adalah jumlah request pertama yang dijawab 503.
	failFirst int

	// Handler HTTP berjalan di goroutine server, test membaca dari goroutine
	// lain, jadi semua state dijaga mutex.
	mu       sync.Mutex
	total    int
	received []AvailabilityUpdate
}

func newFakeChannelManager(t *testing.T, failFirst int) *fakeChannelManager {
	t.Helper()
	f := &fakeChannelManager{failFirst: failFirst}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

// handle melayani POST /availability.
func (f *fakeChannelManager) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/availability" {
		http.NotFound(w, r)
		return
	}
	var update AvailabilityUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.total++
	if f.total <= f.failFirst {
		http.Error(w, "channel manager sedang sibuk", http.StatusServiceUnavailable)
		return
	}
	f.received = append(f.received, update)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeChannelManager) url() string { return f.server.URL }

// accepted mengembalikan semua update yang dijawab 200, sesuai urutan datang.
func (f *fakeChannelManager) accepted() []AvailabilityUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]AvailabilityUpdate(nil), f.received...)
}

// requests mengembalikan jumlah semua request, termasuk yang dijawab 503.
func (f *fakeChannelManager) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.total
}
