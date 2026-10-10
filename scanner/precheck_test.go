package scanner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"piyazche/config"
)

// Dead addresses must fail on the TCP pre-check instead of costing an Xray
// start plus timeout × retries each.
func TestDeadIPsFailFastWithPrecheck(t *testing.T) {
	c := config.DefaultConfig()
	c.Proxy.UUID = "11111111-1111-1111-1111-111111111111"
	c.Proxy.Address = "example.com"
	c.Proxy.Port = 1 // closed on loopback
	if c.Proxy.TLS == nil {
		c.Proxy.TLS = &config.TlsConfig{}
	}
	c.Proxy.TLS.SNI = "example.com"
	c.Scan.Threads = 40
	c.Scan.Timeout = 10
	c.Scan.Retries = 2
	var ips []string
	for i := 1; i <= 200; i++ {
		ips = append(ips, fmt.Sprintf("127.0.%d.%d", i/250, i%250+1))
	}
	s := NewScanner(c)
	s.SetSilent(true)
	s.SetIPs(ips)
	start := time.Now()
	if err := s.RunWithContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	if s.GetResults().Count() != len(ips) {
		t.Fatalf("results=%d", s.GetResults().Count())
	}
	for _, r := range s.GetResults().GetResults() {
		if r.Success || !strings.Contains(r.Error, "unreachable") {
			t.Fatalf("unexpected %+v", r)
		}
	}
	if took > 15*time.Second {
		t.Fatalf("200 dead IPs took %v", took)
	}
	t.Logf("200 dead IPs with 40 workers: %v", took)
}
