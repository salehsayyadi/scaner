package appcore

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"piyazche/config"
	"piyazche/scanner"
	"piyazche/utils"
)

func rangeFixture() ([]scanner.Result, []utils.RangeInfo) {
	var rows []scanner.Result
	var infos []utils.RangeInfo
	for i := 0; i < 40; i++ {
		key := fmt.Sprintf("198.18.%d.0/24", i)
		infos = append(infos, utils.RangeInfo{Entered: key, Network: key})
		for n := 1; n <= 5; n++ {
			rows = append(rows, scanner.Result{IP: fmt.Sprintf("198.18.%d.%d", i, n), Range: key, Success: true, LatencyMs: int64(60 - n*10), LossPercent: float64(n * 4), UploadTested: true, UploadMbps: float64(n), DownloadTested: true, DownloadMbps: float64(60 - n*10)})
		}
		rows = append(rows, scanner.Result{IP: fmt.Sprintf("198.18.%d.99", i), Range: key, Success: false, DownloadTested: true, DownloadMbps: 999, LatencyMs: 0})
	}
	return rows, infos
}
func TestFortyRangesThreeSelectedTwoEach(t *testing.T) {
	rows, infos := rangeFixture()
	original := append([]scanner.Result(nil), rows...)
	specs := PickRangeSpecs(rows, infos)
	if len(specs) != 40 {
		t.Fatal(len(specs))
	}
	for _, tc := range []struct {
		rank  string
		first int
	}{{RankPing, 5}, {RankDownload, 1}, {RankUpload, 5}, {RankQuality, 2}} {
		prefs := PickPreferences{Selected: []string{specs[0].Key, specs[12].Key, specs[39].Key}, PerRange: 2, Rank: tc.rank}
		groups := BuildRangePicks(rows, specs, prefs)
		if len(groups) != 3 || len(FlattenRangePicks(groups)) != 6 {
			t.Fatalf("%s wrong group counts: %+v", tc.rank, groups)
		}
		for _, group := range groups {
			if len(group.Rows) != 2 || group.Eligible != 5 || !strings.HasSuffix(group.Rows[0].IP, fmt.Sprintf(".%d", tc.first)) {
				t.Fatalf("%s incorrect ranking: %+v", tc.rank, group)
			}
			for _, row := range group.Rows {
				if row.Range != group.Key || !row.Success {
					t.Fatal("cross-range or failed row")
				}
			}
		}
		text := RangePicksText(groups, true)
		if strings.Count(text, "────────────────────") != 2 {
			t.Fatal("missing group separators")
		}
	}
	if !reflect.DeepEqual(rows, original) {
		t.Fatal("view mutated scan results")
	}
	if len(BuildRangePicks(rows, specs, PickPreferences{PerRange: 2, Rank: RankPing})) != 0 {
		t.Fatal("empty selection unexpectedly means all")
	}
}
func TestMeasuredSpeedEligibilityAndDeduplication(t *testing.T) {
	specs := []RangeSpec{{"192.0.2.0/24", "test"}}
	rows := []scanner.Result{
		{IP: "192.0.2.2", Range: specs[0].Key, Success: true, DownloadTested: false, DownloadMbps: 1000},
		{IP: "192.0.2.3", Range: specs[0].Key, Success: true, DownloadTested: true, DownloadMbps: math.NaN()},
		{IP: "192.0.2.4", Range: specs[0].Key, Success: true, DownloadTested: true, DownloadMbps: 5},
		{IP: "192.0.2.4", Range: specs[0].Key, Success: true, DownloadTested: true, DownloadMbps: 10},
	}
	groups := BuildRangePicks(rows, specs, PickPreferences{Selected: []string{specs[0].Key}, PerRange: 2, Rank: RankDownload})
	if len(groups) != 1 || len(groups[0].Rows) != 1 || groups[0].Rows[0].DownloadMbps != 10 {
		t.Fatalf("unknown speed/duplicates included: %+v", groups)
	}
}
func TestStoredOwnershipAndLegacyOverlap(t *testing.T) {
	specs := []RangeSpec{{"192.0.2.0/24", "broad"}, {"192.0.2.0/25", "narrow"}}
	rows := []scanner.Result{{IP: "192.0.2.5", Success: true}, {IP: "192.0.2.6", Range: "192.0.2.0/25", Success: true}}
	groups := BuildRangePicks(rows, specs, PickPreferences{Selected: []string{specs[0].Key, specs[1].Key}, PerRange: 2, Rank: RankPing})
	if len(groups[0].Rows) != 1 || groups[0].Rows[0].IP != "192.0.2.5" || len(groups[1].Rows) != 1 || groups[1].Rows[0].IP != "192.0.2.6" {
		t.Fatalf("ownership not stable: %+v", groups)
	}
}
func TestRunStampsRangeBeforeStorageAndSessionKeepsFilters(t *testing.T) {
	c := config.DefaultConfig()
	r, err := NewRunWithSource(context.Background(), c, "192.0.2.0/24\n198.51.100.0/24", ModeConnectivity, SourceRanges, false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	r.Results.Add(scanner.Result{IP: "198.51.100.9", Success: true})
	if r.Results.GetResults()[0].Range != "198.51.100.0/24" {
		t.Fatal("range provenance not stamped")
	}
	s := DefaultSession()
	s.Picks = PickPreferences{Selected: []string{"198.51.100.0/24"}, PerRange: 2, Rank: RankDownload, Initialized: true}
	dir := t.TempDir()
	if err := SaveSession(dir, s); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadSession(dir)
	if err != nil || !reflect.DeepEqual(s.Picks, saved.Picks) {
		t.Fatal("range preferences not persisted", err)
	}
}
