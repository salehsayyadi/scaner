package utils

import (
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"strings"
)

// MaxGeneratedTargets bounds how many addresses a single run may generate so a
// broad range with "0 = all" (for example a /8 or an IPv6 /64) cannot exhaust
// memory. Users can still narrow ranges or set IPs per range.
const MaxGeneratedTargets = 4_000_000

// SingleIPsLabel groups individual addresses in range summaries.
const SingleIPsLabel = "Individual IPs"

// RangeInfo describes one entered range (or the group of individual IPs).
type RangeInfo struct {
	Entered string // as typed by the user, e.g. 185.133.35.0/16
	Network string // normalized network, e.g. 185.133.0.0/16
	Hosts   uint64 // usable addresses in the range
	Queued  int    // addresses that will be tested
}

// Label returns the entered range plus its normalized network when they differ.
func (r RangeInfo) Label() string {
	if r.Network != "" && r.Network != r.Entered {
		return r.Entered + " → " + r.Network
	}
	return r.Entered
}

// TargetPlan is the expanded list of addresses together with the range each
// address came from (Owner[i] indexes Ranges for IPs[i]).
type TargetPlan struct {
	IPs     []string
	Owner   []int
	Ranges  []RangeInfo
	Invalid int // tokens that were neither an IP nor a CIDR
}

// usableHosts mirrors ExpandCIDR: for IPv4 blocks of /24 or smaller, addresses
// ending in .0 or .255 are skipped.
func usableHosts(ipnet *net.IPNet) uint64 {
	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones
	if hostBits >= 63 {
		return ^uint64(0)
	}
	n := uint64(1) << uint(hostBits)
	if v4 := ipnet.IP.To4(); v4 != nil && bits == 32 && ones >= 24 {
		first := int(v4[3])
		if first == 0 {
			n--
		}
		if first+int(uint64(1)<<uint(hostBits))-1 == 255 && n > 0 {
			n--
		}
	}
	return n
}

// queuedFor returns how many addresses ExpandCIDR will produce for a range
// with the given per-range sample size (0 = all).
func queuedFor(hosts uint64, perRange int) uint64 {
	if perRange > 0 && uint64(perRange) < hosts {
		return uint64(perRange)
	}
	return hosts
}

type plannedToken struct {
	tok   string
	owner int
	cidr  bool
}

// PlanRanges summarizes the targets without expanding them, so the preview stays
// instant even for very large ranges. ipList means "scan exactly as entered":
// no sampling (every address of a CIDR is included).
func PlanRanges(text string, perRange int, ipList bool) (ranges []RangeInfo, total uint64, invalid int) {
	ranges, total, invalid, _ = planRanges(text, perRange, ipList)
	return
}

func planRanges(text string, perRange int, ipList bool) (ranges []RangeInfo, total uint64, invalid int, toks []plannedToken) {
	if ipList {
		perRange = 0
	}
	single := -1
	for _, tok := range tokenizeRanges(text) {
		if strings.Contains(tok, "/") {
			_, ipnet, err := net.ParseCIDR(tok)
			if err != nil {
				invalid++
				continue
			}
			h := usableHosts(ipnet)
			q := queuedFor(h, perRange)
			if q > MaxGeneratedTargets+1 {
				q = MaxGeneratedTargets + 1
			}
			toks = append(toks, plannedToken{tok: tok, owner: len(ranges), cidr: true})
			ranges = append(ranges, RangeInfo{Entered: tok, Network: ipnet.String(), Hosts: h, Queued: int(q)})
			total += q
			continue
		}
		if net.ParseIP(tok) == nil {
			invalid++
			continue
		}
		if single < 0 {
			single = len(ranges)
			ranges = append(ranges, RangeInfo{Entered: SingleIPsLabel})
		}
		toks = append(toks, plannedToken{tok: tok, owner: single})
		ranges[single].Hosts++
		ranges[single].Queued++
		total++
	}
	return ranges, total, invalid, toks
}

// BuildTargets expands ranges into the addresses to scan.
//
// Range scan (ipList=false): each CIDR contributes perRange random addresses
// (0 = every usable address); individual IPs are kept. The list is shuffled when
// requested.
//
// IP list (ipList=true): addresses are scanned exactly in the entered order;
// CIDRs are expanded completely, nothing is sampled or shuffled.
//
// Duplicate addresses are tested once (attributed to the first range).
func BuildTargets(text string, perRange int, ipList, shuffle bool) (*TargetPlan, error) {
	if ipList {
		perRange, shuffle = 0, false
	}
	ranges, total, invalid, toks := planRanges(text, perRange, ipList)
	if len(ranges) == 0 {
		return nil, fmt.Errorf("no valid IPs or CIDRs found")
	}
	if total > MaxGeneratedTargets {
		return nil, fmt.Errorf("these ranges would queue %s IPs (limit %s); set IPs per range or narrow the ranges", formatCount(total), formatCount(MaxGeneratedTargets))
	}
	p := &TargetPlan{Ranges: ranges, Invalid: invalid}
	seen := make(map[string]struct{}, int(total))
	add := func(ip string, owner int) {
		if _, dup := seen[ip]; dup {
			return
		}
		seen[ip] = struct{}{}
		p.IPs = append(p.IPs, ip)
		p.Owner = append(p.Owner, owner)
	}
	for _, t := range toks {
		if !t.cidr {
			add(t.tok, t.owner)
			continue
		}
		ips, err := ExpandCIDR(t.tok, perRange)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			add(ip, t.owner)
		}
	}
	if len(p.IPs) == 0 {
		return nil, fmt.Errorf("no usable addresses in the entered ranges")
	}
	if shuffle {
		perm := randPerm(len(p.IPs))
		ips := make([]string, len(p.IPs))
		owner := make([]int, len(p.IPs))
		for i, j := range perm {
			ips[i], owner[i] = p.IPs[j], p.Owner[j]
		}
		p.IPs, p.Owner = ips, owner
	}
	p.Recount()
	return p, nil
}

// Truncate keeps the first n addresses and updates the per-range queue counts.
func (p *TargetPlan) Truncate(n int) {
	if n <= 0 || n >= len(p.IPs) {
		return
	}
	p.IPs, p.Owner = p.IPs[:n], p.Owner[:n]
	p.Recount()
}

// Recount sets each range's Queued to the number of addresses actually queued.
func (p *TargetPlan) Recount() {
	for i := range p.Ranges {
		p.Ranges[i].Queued = 0
	}
	for _, o := range p.Owner {
		if o >= 0 && o < len(p.Ranges) {
			p.Ranges[o].Queued++
		}
	}
}

func formatCount(n uint64) string {
	s := new(big.Int).SetUint64(n).String()
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func randPerm(n int) []int { return rand.Perm(n) }
