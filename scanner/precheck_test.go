package scanner

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"piyazche/config"
	"piyazche/utils"
	"piyazche/xray"
)

// Real local VLESS server -> SOCKS -> HTTP request, with a deliberately failed
// advisory TCP probe. No public endpoint, credentials or user's network needed.
func TestFailedPrecheckStillConfirmsThroughRealVLESS(t *testing.T) {
	for _, transport := range []string{"tcp", "ws"} {
		t.Run(transport, func(t *testing.T) {
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
			defer web.Close()
			port := utils.AcquirePort()
			defer utils.ReleasePort(port)
			uuid := "11111111-1111-1111-1111-111111111111"
			server := []byte(fmt.Sprintf(`{"log":{"loglevel":"none"},"inbounds":[{"listen":"127.0.0.1","port":%d,"protocol":"vless","settings":{"clients":[{"id":%q}],"decryption":"none"},"streamSettings":{"network":%q,"security":"none","wsSettings":{"path":"/test"}}}],"outbounds":[{"protocol":"freedom"}]}`, port, uuid, transport))
			mgr := xray.NewManager()
			if err := mgr.Start(server, port); err != nil {
				t.Fatal(err)
			}
			defer mgr.Stop()
			if err := mgr.WaitForReady(time.Second); err != nil {
				t.Fatal(err)
			}
			link := fmt.Sprintf("vless://%s@127.0.0.1:%d?type=%s&security=none&path=%%2Ftest", uuid, port, transport)
			c, err := config.ParseConfigInput(link)
			if err != nil {
				t.Fatal(err)
			}
			c.Scan.TestURL, c.Scan.Timeout = web.URL, 2
			c.Scan.Retries, c.Scan.PingSamples, c.Scan.LossTest, c.Scan.MaxLatency = 1, 1, false, 0
			collector := NewResultCollector()
			done := &atomic.Int64{}
			worker := NewWorker(0, c, collector, &sync.WaitGroup{}, nil, make(chan struct{}), context.Background(), done, nil, false, &sync.Once{}, nil)
			probes := 0
			worker.tcpConnect = func(string, int, time.Duration) (time.Duration, error) {
				probes++
				return 0, fmt.Errorf("simulated transient TCP timeout")
			}
			worker.processIP("127.0.0.1")
			rows := collector.GetResults()
			if probes == 0 || done.Load() != 1 || len(rows) != 1 || !rows[0].Success {
				t.Fatalf("working VLESS IP rejected after advisory probe: %+v (probes=%d)", rows, probes)
			}
			// Wrong credentials must still fail the real protocol test.
			c.Proxy.UUID = "22222222-2222-2222-2222-222222222222"
			c.Scan.Timeout = 1
			bad := worker.measureIP("127.0.0.1")
			if bad.Success || bad.Error == nil {
				t.Fatalf("invalid authentication accepted: %+v", bad)
			}
		})
	}
}
