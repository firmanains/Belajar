package lab07

// File ini yang kamu ubah (Step 1 dan Step 2).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// eventually melaporkan apakah cond menjadi true sebelum timeout habis.
// Versi ini tidur sebentar lalu memeriksa sekali.
func eventually(timeout time.Duration, cond func() bool) bool {
	time.Sleep(50 * time.Millisecond)
	return cond()
}

// fakeChannelManager adalah channel manager tiruan: HTTP server sungguhan di
// localhost yang merekam apa yang ia terima.
type fakeChannelManager struct {
	server *httptest.Server

	// failFirst adalah jumlah request pertama yang dijawab 503.
	failFirst int
}

func newFakeChannelManager(t *testing.T, failFirst int) *fakeChannelManager {
	t.Helper()
	f := &fakeChannelManager{failFirst: failFirst}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

// handle melayani POST /availability. Versi ini menjawab 200 untuk apa pun
// dan tidak merekam apa-apa.
func (f *fakeChannelManager) handle(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (f *fakeChannelManager) url() string { return f.server.URL }

// accepted mengembalikan semua update yang dijawab 200, sesuai urutan datang.
func (f *fakeChannelManager) accepted() []AvailabilityUpdate {
	return nil
}

// requests mengembalikan jumlah semua request, termasuk yang dijawab 503.
func (f *fakeChannelManager) requests() int {
	return 0
}
