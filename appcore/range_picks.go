package appcore

import (
	"fmt"
	"math"
	"net/netip"
	"sort"
	"strings"

	"piyazche/scanner"
	"piyazche/utils"
)

const (
	RankQuality  = "Quality"
	RankPing     = "Best ping"
	RankDownload = "Best download"
	RankUpload   = "Best upload"
	OtherRange   = "Individual / unassigned IPs"
)

type PickPreferences struct {
	Selected    []string `json:"selected"`
	PerRange    int      `json:"perRange"`
	Rank        string   `json:"rank"`
	Initialized bool     `json:"initialized"`
}

func DefaultPickPreferences() PickPreferences { return PickPreferences{PerRange: 2, Rank: RankQuality} }
func (p *PickPreferences) Normalize() {
	if p.PerRange < 1 || p.PerRange > 1000 {
		p.PerRange = 2
	}
	switch p.Rank {
	case RankQuality, RankPing, RankDownload, RankUpload:
	default:
		p.Rank = RankQuality
	}
}

type RangeSpec struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}
type RangeGroup struct {
	RangeSpec
	Eligible int              `json:"eligible"`
	Rows     []scanner.Result `json:"rows"`
}

func rangeKey(info utils.RangeInfo) string {
	if info.Network != "" {
		return info.Network
	}
	return info.Entered
}

// PickRangeSpecs keeps input order and de-duplicates normalized ranges. Stored
// provenance is authoritative; inference is only for legacy results without it.
func PickRangeSpecs(rows []scanner.Result, infos []utils.RangeInfo) []RangeSpec {
	var specs []RangeSpec
	seen := map[string]bool{}
	add := func(key, label string) {
		if key != "" && !seen[key] {
			seen[key] = true
			specs = append(specs, RangeSpec{key, label})
		}
	}
	for _, info := range infos {
		add(rangeKey(info), info.Label())
	}
	for _, row := range rows {
		if row.Range != "" {
			add(row.Range, row.Range)
		}
	}
	for _, row := range rows {
		if row.Range == "" && inferRange(row.IP, specs) == "" {
			add(OtherRange, OtherRange)
		}
	}
	return specs
}
func inferRange(ip string, specs []RangeSpec) string {
	address, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	address = address.Unmap()
	// First entered matching range owns overlapping addresses, like BuildTargets.
	for _, spec := range specs {
		if prefix, err := netip.ParsePrefix(spec.Key); err == nil && prefix.Contains(address) {
			return spec.Key
		}
	}
	for _, spec := range specs {
		if spec.Key == utils.SingleIPsLabel || spec.Key == OtherRange {
			return spec.Key
		}
	}
	return ""
}

func metricEligible(row scanner.Result, rank string) bool {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
	if !row.Success || row.LatencyMs < 0 || !finite(row.LossPercent) {
		return false
	}
	switch rank {
	case RankDownload:
		return row.DownloadTested && finite(row.DownloadMbps)
	case RankUpload:
		return row.UploadTested && finite(row.UploadMbps)
	}
	return true
}
func betterPick(a, b scanner.Result, rank string) bool {
	switch rank {
	case RankDownload:
		if a.DownloadMbps != b.DownloadMbps {
			return a.DownloadMbps > b.DownloadMbps
		}
	case RankUpload:
		if a.UploadMbps != b.UploadMbps {
			return a.UploadMbps > b.UploadMbps
		}
	case RankQuality:
		if x, y := int(a.LossPercent)/10, int(b.LossPercent)/10; x != y {
			return x < y
		}
	}
	if a.LatencyMs != b.LatencyMs {
		return a.LatencyMs < b.LatencyMs
	}
	if a.LossPercent != b.LossPercent {
		return a.LossPercent < b.LossPercent
	}
	if a.JitterMs != b.JitterMs {
		return a.JitterMs < b.JitterMs
	}
	x, e1 := netip.ParseAddr(a.IP)
	y, e2 := netip.ParseAddr(b.IP)
	if e1 == nil && e2 == nil {
		return x.Compare(y) < 0
	}
	return a.IP < b.IP
}

// BuildRangePicks ranks within each selected range, not globally. No failed or
// unmeasured-speed row can be promoted by a sort. Empty selection means none.
func BuildRangePicks(rows []scanner.Result, specs []RangeSpec, prefs PickPreferences) []RangeGroup {
	prefs.Normalize()
	selected := map[string]bool{}
	for _, key := range prefs.Selected {
		selected[key] = true
	}
	buckets := map[string]map[string]scanner.Result{}
	for _, row := range rows {
		if !metricEligible(row, prefs.Rank) {
			continue
		}
		key := row.Range
		if key == "" {
			key = inferRange(row.IP, specs)
		}
		if !selected[key] {
			continue
		}
		if buckets[key] == nil {
			buckets[key] = map[string]scanner.Result{}
		}
		old, ok := buckets[key][row.IP]
		if !ok || betterPick(row, old, prefs.Rank) {
			row.Range = key
			buckets[key][row.IP] = row
		}
	}
	groups := []RangeGroup{}
	for _, spec := range specs {
		if !selected[spec.Key] {
			continue
		}
		group := RangeGroup{RangeSpec: spec, Rows: []scanner.Result{}}
		for _, row := range buckets[spec.Key] {
			group.Rows = append(group.Rows, row)
		}
		sort.Slice(group.Rows, func(i, j int) bool { return betterPick(group.Rows[i], group.Rows[j], prefs.Rank) })
		group.Eligible = len(group.Rows)
		if len(group.Rows) > prefs.PerRange {
			group.Rows = group.Rows[:prefs.PerRange]
		}
		groups = append(groups, group)
	}
	return groups
}
func FlattenRangePicks(groups []RangeGroup) []scanner.Result {
	rows := []scanner.Result{}
	for _, group := range groups {
		rows = append(rows, group.Rows...)
	}
	return rows
}
func RangePicksText(groups []RangeGroup, grouped bool) string {
	var parts []string
	for _, group := range groups {
		if len(group.Rows) == 0 {
			continue
		}
		var ips []string
		for _, row := range group.Rows {
			ips = append(ips, row.IP)
		}
		block := strings.Join(ips, "\n")
		if grouped {
			block = fmt.Sprintf("%s\n%s", group.Label, block)
		}
		parts = append(parts, block)
	}
	sep := "\n"
	if grouped {
		sep = "\n────────────────────\n"
	}
	return strings.Join(parts, sep)
}
