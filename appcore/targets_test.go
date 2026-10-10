package appcore

import (
	"context"
	"testing"
	"time"

	"piyazche/config"
	"piyazche/scanner"
	"piyazche/utils"
)

func TestRangeScanTracksEveryRange(t *testing.T) {
	c := config.DefaultConfig()
	c.Scan.SampleSize = 2
	r, err := NewRunWithSource(context.Background(), c, "10.0.0.0/24\n10.0.1.0/24\n9.9.9.9", ModeConnectivity, SourceRanges, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.IPs) != 5 {
		t.Fatalf("ips=%v", r.IPs)
	}
	snap := r.Snapshot(0)
	if len(snap.Ranges) != 3 || snap.Ranges[0].Queued != 2 || snap.Ranges[0].Hosts != 254 || snap.Ranges[2].Entered != utils.SingleIPsLabel {
		t.Fatalf("ranges=%+v", snap.Ranges)
	}
	for _, ip := range r.IPs {
		r.setActive(ip, true)
		time.Sleep(time.Millisecond)
	}
	last := r.IPs[len(r.IPs)-1]
	snap = r.Snapshot(2)
	if snap.ActiveTotal != 5 || len(snap.Active) != 2 || snap.Active[0] != last || snap.CurrentRange != r.RangeOf(last) {
		t.Fatalf("active=%+v", snap)
	}
	for _, ip := range r.IPs {
		r.setActive(ip, false)
		r.Results.Add(scanner.Result{IP: ip, Success: ip == "9.9.9.9"})
	}
	snap = r.Snapshot(0)
	if snap.ActiveTotal != 0 || snap.Ranges[0].Tested != 2 || snap.Ranges[1].Tested != 2 || snap.Ranges[2].Good != 1 {
		t.Fatalf("stats=%+v", snap.Ranges)
	}
}

func TestIPListScansExactlyAsEntered(t *testing.T) {
	c := config.DefaultConfig()
	c.Scan.SampleSize = 1
	c.Scan.Shuffle = true
	list := "9.9.9.9\n1.1.1.1\n8.8.8.8\n10.0.0.0/29"
	r, err := NewRunWithSource(context.Background(), c, list, ModeConnectivity, SourceIPList, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"9.9.9.9", "1.1.1.1", "8.8.8.8", "10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5", "10.0.0.6", "10.0.0.7"}
	if len(r.IPs) != len(want) {
		t.Fatalf("ips=%v", r.IPs)
	}
	for i := range want {
		if r.IPs[i] != want[i] {
			t.Fatalf("ips=%v", r.IPs)
		}
	}
}

func TestCheckConnectionTargetsOnlyTheServer(t *testing.T) {
	c := config.DefaultConfig()
	c.Proxy.UUID = "11111111-1111-1111-1111-111111111111"
	c.Proxy.Address = "example.com"
	if c.Proxy.TLS == nil {
		c.Proxy.TLS = &config.TlsConfig{}
	}
	c.Proxy.TLS.SNI = "example.com"
	r, err := NewRunWithSource(context.Background(), c, "185.133.35.0/16", ModeCheck, SourceRanges, false)
	if err != nil {
		t.Fatal(err)
	}
	snap := r.Snapshot(0)
	if len(r.IPs) != 1 || r.IPs[0] != "example.com" || len(snap.Ranges) != 1 || snap.Ranges[0].Entered != ServerAddressLabel {
		t.Fatalf("ips=%v ranges=%+v", r.IPs, snap.Ranges)
	}
}
