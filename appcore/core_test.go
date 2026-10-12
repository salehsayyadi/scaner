package appcore

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"piyazche/config"
	"piyazche/scanner"
)

func TestSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := DefaultSession()
	s.Ranges = "127.0.0.1"
	s.Config.Scan.DownloadTest = true
	s.Config.Xray.Mux.XudpConcurrency = 13
	s.Config.Proxy.Reality = &config.RealityConfig{PublicKey: "test-key", ShortId: "abcd"}
	if err := SaveSession(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSession(dir)
	if err != nil || got.Config.Xray.Mux.XudpConcurrency != 13 || got.Config.Proxy.Reality.PublicKey != "test-key" || !got.Config.Scan.DownloadTest || got.Ranges != s.Ranges {
		t.Fatalf("round trip failed: %v", err)
	}
	s.Config.Scan.Threads = 7
	if err := SaveSession(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err = LoadSession(dir)
	if err != nil || got.Config.Scan.Threads != 7 {
		t.Fatalf("overwrite failed: %v", err)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, ".session-*.tmp")); len(files) != 0 {
		t.Fatal("temporary files leaked")
	}
}

func TestCorruptSessionDoesNotBecomeEmptyConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "session.json"), []byte("{broken"), 0600)
	s, err := LoadSession(dir)
	if err == nil || s.Config == nil || s.Config.Scan.Threads != config.DefaultConfig().Scan.Threads {
		t.Fatal("corrupt session was not reported")
	}
}

func TestCancelledRunDoesNotScan(t *testing.T) {
	c := config.DefaultConfig()
	c.Scan.SampleSize = 1
	r, err := NewRun(context.Background(), c, "127.0.0.1", ModeConnectivity, false)
	if err != nil {
		t.Fatal(err)
	}
	c.Scan.Threads = 99
	if r.Config.Scan.Threads == 99 {
		t.Fatal("config snapshot is shared")
	}
	r.Stop()
	if err := r.Execute(); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
	if r.Results.Count() != 0 {
		t.Fatal("cancelled task performed work")
	}
}

func TestProxyInputValidationAndRealityRestriction(t *testing.T) {
	c := config.DefaultConfig()
	if _, err := NewRun(context.Background(), c, "127.0.0.1", ModeXray, false); err == nil {
		t.Fatal("empty connection accepted")
	}
	c.Proxy.UUID = "00000000-0000-0000-0000-000000000001"
	c.Proxy.Method = "reality"
	c.Proxy.Address = "127.0.0.1"
	c.Proxy.Reality = &config.RealityConfig{PublicKey: "test"}
	if _, err := NewRun(context.Background(), c, "127.0.0.1", ModeXray, false); err == nil {
		t.Fatal("Reality scan should suggest check mode")
	}
	if _, err := NewRun(context.Background(), c, "", ModeCheck, false); err != nil {
		t.Fatal(err)
	}
}

func TestExportPreservesMetrics(t *testing.T) {
	rows := []scanner.Result{{IP: "127.0.0.1", Success: true, LatencyMs: 21, TCPMs: 3, TCPOK: true, HandshakeMs: 9, JitterMs: 2, LossPercent: 4, UploadTested: true, UploadMbps: 8.5, DownloadTested: true, DownloadMbps: 18.5, TestedAt: time.Now()}}
	var b bytes.Buffer
	if err := EncodeResults(&b, rows, "json"); err != nil {
		t.Fatal(err)
	}
	var got []scanner.Result
	if err := json.Unmarshal(b.Bytes(), &got); err != nil || got[0].UploadMbps != 8.5 || got[0].LossPercent != 4 || got[0].HandshakeMs != 9 {
		t.Fatal("metrics lost", err)
	}
	b.Reset()
	if err := EncodeResults(&b, rows, "csv"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b.Bytes(), []byte("8.50,18.50,success")) {
		t.Fatal(b.String())
	}
	if err := EncodeResults(&b, rows, "invalid"); err == nil {
		t.Fatal("bad format accepted")
	}
}
