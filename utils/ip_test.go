package utils

import (
	"net"
	"testing"
)

func TestCIDRHostCount(t *testing.T) {
	if n, ok := CIDRHostCount("10.0.0.0/8"); !ok || n != 1<<24 {
		t.Errorf("/8 = %d ok=%v", n, ok)
	}
	if n, ok := CIDRHostCount("1.2.3.0/24"); !ok || n != 256 {
		t.Errorf("/24 = %d ok=%v", n, ok)
	}
	if _, ok := CIDRHostCount("not-a-cidr"); ok {
		t.Errorf("expected invalid")
	}
}

// TestExpandCIDRLargeNoEnumerate ensures a huge CIDR is sampled, not enumerated
// (enumerating /1 = 2^31 addresses would OOM/crash — this is the bug the TUI hit).
func TestExpandCIDRLargeNoEnumerate(t *testing.T) {
	ips, err := ExpandCIDR("10.0.0.0/1", 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ips) != 5 {
		t.Fatalf("want 5 sampled IPs, got %d", len(ips))
	}
	_, ipnet, _ := net.ParseCIDR("10.0.0.0/1")
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil || !ipnet.Contains(ip) {
			t.Errorf("sampled IP %q not in range", s)
		}
	}
}

func TestCountRangesCheap(t *testing.T) {
	// sampleSize 1 -> each CIDR contributes 1, plus the single IP = 3 total.
	n, ok := CountRanges("10.0.0.0/8\n9.0.0.0/4\n1.2.3.4", 1)
	if !ok || n != 3 {
		t.Fatalf("count = %d ok=%v (want 3)", n, ok)
	}
}

func TestExpandCIDRSmallStillEnumerates(t *testing.T) {
	ips, err := ExpandCIDR("1.2.3.0/24", 0)
	if err != nil {
		t.Fatal(err)
	}
	// /24 minus .0 and .255 = 254
	if len(ips) != 254 {
		t.Errorf("want 254, got %d", len(ips))
	}
}
