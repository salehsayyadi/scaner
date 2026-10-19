package appcore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"piyazche/config"
	"piyazche/utils"
	"piyazche/xray"
)

func TestConnectionCheckUsesOriginalServerWithoutQualityFilters(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(20 * time.Millisecond); w.WriteHeader(204) }))
	defer web.Close()
	port := utils.AcquirePort()
	defer utils.ReleasePort(port)
	uuid := "11111111-1111-1111-1111-111111111111"
	server := []byte(fmt.Sprintf(`{"log":{"loglevel":"none"},"inbounds":[{"listen":"127.0.0.1","port":%d,"protocol":"vless","settings":{"clients":[{"id":%q}],"decryption":"none"},"streamSettings":{"network":"tcp","security":"none"}}],"outbounds":[{"protocol":"freedom"}]}`, port, uuid))
	mgr := xray.NewManager()
	if err := mgr.Start(server, port); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()
	if err := mgr.WaitForReady(time.Second); err != nil {
		t.Fatal(err)
	}
	c, err := config.ParseConfigInput(fmt.Sprintf("vless://%s@127.0.0.1:%d?type=tcp&security=none", uuid, port))
	if err != nil {
		t.Fatal(err)
	}
	c.Scan.TestURL, c.Scan.MaxLatency, c.Scan.Timeout = web.URL, 1, 2
	r, err := NewRun(context.Background(), c, "not a valid range", ModeCheck, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.IPs) != 1 || r.IPs[0] != "127.0.0.1" {
		t.Fatal("check is using scan targets instead of original server")
	}
	if err := r.Execute(); err != nil {
		t.Fatal(err)
	}
	if r.Results.SuccessCount() != 1 {
		t.Fatalf("healthy connection rejected by quality filter: %+v", r.Results.GetResults())
	}
	// The same connection in a scan should be explicitly rejected by a strict
	// quality limit, rather than being reported as a protocol/authentication failure.
	c.Scan.PingSamples, c.Scan.LossTest, c.Scan.Threads, c.Scan.Retries = 1, false, 1, 1
	scan, err := NewRunWithSource(context.Background(), c, "127.0.0.1", ModeXray, SourceIPList, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := scan.Execute(); err != nil {
		t.Fatal(err)
	}
	rows := scan.Results.GetResults()
	if len(rows) != 1 || rows[0].Success || !rows[0].Connected || rows[0].Error == "" {
		t.Fatalf("quality filter did not reject slow connection: %+v", rows)
	}
}
