package utils

import (
	"net"
	"testing"
)

func inCIDR(ip, cidr string) bool {
	_, n, err := net.ParseCIDR(cidr)
	return err == nil && n.Contains(net.ParseIP(ip))
}

func TestPlanRangesCountsHostsAndQueue(t *testing.T) {
	ranges, total, invalid := PlanRanges("185.133.35.0/16\n208.86.168.0/24, 1.1.1.1 8.8.8.8\nnot-an-ip", 5, false)
	if invalid != 1 {
		t.Fatalf("invalid=%d", invalid)
	}
	if len(ranges) != 3 {
		t.Fatalf("ranges=%+v", ranges)
	}
	if ranges[0].Network != "185.133.0.0/16" || ranges[0].Hosts != 65536 || ranges[0].Queued != 5 {
		t.Fatalf("first=%+v", ranges[0])
	}
	if ranges[0].Label() != "185.133.35.0/16 → 185.133.0.0/16" {
		t.Fatalf("label=%q", ranges[0].Label())
	}
	if ranges[1].Hosts != 254 || ranges[1].Queued != 5 {
		t.Fatalf("second=%+v", ranges[1])
	}
	if ranges[2].Entered != SingleIPsLabel || ranges[2].Queued != 2 {
		t.Fatalf("singles=%+v", ranges[2])
	}
	if total != 12 {
		t.Fatalf("total=%d", total)
	}
	_, all, _ := PlanRanges("10.0.0.0/16\n10.1.0.0/24", 0, false)
	if all != 65536+254 {
		t.Fatalf("all=%d", all)
	}
}

func TestBuildTargetsRangeScan(t *testing.T) {
	p, err := BuildTargets("10.0.0.0/24\n10.0.1.0/24\n10.9.9.9", 3, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.IPs) != 7 || len(p.Owner) != 7 {
		t.Fatalf("ips=%v", p.IPs)
	}
	want := []int{3, 3, 1}
	for i, r := range p.Ranges {
		if r.Queued != want[i] {
			t.Fatalf("range %d queued=%d", i, r.Queued)
		}
	}
	for i, ip := range p.IPs {
		o := p.Ranges[p.Owner[i]]
		if o.Network != "" && !inCIDR(ip, o.Network) {
			t.Fatalf("%s attributed to %s", ip, o.Network)
		}
	}
	all, err := BuildTargets("10.0.0.0/24", 0, false, false)
	if err != nil || len(all.IPs) != 254 {
		t.Fatalf("all=%v err=%v", all, err)
	}
}

func TestBuildTargetsIPListKeepsOrderAndDoesNotSample(t *testing.T) {
	p, err := BuildTargets("9.9.9.9\n1.1.1.1\n10.0.0.0/30\n1.1.1.1", 1, true, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"9.9.9.9", "1.1.1.1", "10.0.0.1", "10.0.0.2", "10.0.0.3"}
	if len(p.IPs) != len(want) {
		t.Fatalf("ips=%v", p.IPs)
	}
	for i := range want {
		if p.IPs[i] != want[i] {
			t.Fatalf("ips=%v", p.IPs)
		}
	}
}

func TestBuildTargetsRejectsHugeExpansion(t *testing.T) {
	if _, err := BuildTargets("10.0.0.0/8", 0, false, false); err == nil {
		t.Fatal("a /8 with 0 = all must be rejected instead of exhausting memory")
	}
	if p, err := BuildTargets("10.0.0.0/8", 10, false, false); err != nil || len(p.IPs) != 10 {
		t.Fatalf("sampled /8 failed: %v", err)
	}
	p, _ := BuildTargets("10.0.0.0/24\n10.0.1.0/24", 0, false, false)
	p.Truncate(300)
	if len(p.IPs) != 300 || p.Ranges[0].Queued != 254 || p.Ranges[1].Queued != 46 {
		t.Fatalf("truncate=%+v", p.Ranges)
	}
}
