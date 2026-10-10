package scanner

import (
	"path/filepath"
	"testing"
	"time"
)

// TestCSVRoundTripTCPColumn ensures the new TCP-connect column survives a
// SaveToCSV -> LoadResultsCSV round trip, and that an IP whose TCP probe
// failed (TCPOK=false) round-trips as "not tested" rather than a false 0ms.
func TestCSVRoundTripTCPColumn(t *testing.T) {
	rc := NewResultCollector()
	rc.Add(Result{IP: "1.2.3.4", Success: true, Latency: 120 * time.Millisecond, TCPOK: true, TCPMs: 45, JitterMs: 3, LossPercent: 0})
	rc.Add(Result{IP: "5.6.7.8", Success: true, Latency: 200 * time.Millisecond, TCPOK: false, JitterMs: 1, LossPercent: 10})

	path := filepath.Join(t.TempDir(), "results.csv")
	if err := rc.SaveToCSV(path); err != nil {
		t.Fatalf("SaveToCSV: %v", err)
	}

	loaded, err := LoadResultsCSV(path)
	if err != nil {
		t.Fatalf("LoadResultsCSV: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("got %d results, want 2", len(loaded))
	}

	byIP := map[string]Result{}
	for _, r := range loaded {
		byIP[r.IP] = r
	}

	got := byIP["1.2.3.4"]
	if !got.TCPOK || got.TCPMs != 45 {
		t.Errorf("1.2.3.4: TCPOK=%v TCPMs=%d, want true/45", got.TCPOK, got.TCPMs)
	}

	got2 := byIP["5.6.7.8"]
	if got2.TCPOK || got2.TCPMs != 0 {
		t.Errorf("5.6.7.8: TCPOK=%v TCPMs=%d, want false/0 (probe never succeeded)", got2.TCPOK, got2.TCPMs)
	}
}
