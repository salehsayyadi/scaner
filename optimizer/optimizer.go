package optimizer

import (
	"context"
	"fmt"
	"time"

	"piyazche/utils"
)

// Optimizer is the main entry point for fragment optimization
// It wraps Finder with additional features like context support and formatted output
type Optimizer struct {
	finder *Finder
	config FinderConfig
}

// NewOptimizer creates a new optimizer with the given config and tester function
func NewOptimizer(config FinderConfig, tester TesterFunc) *Optimizer {
	return &Optimizer{
		finder: NewFinder(config, tester),
		config: config,
	}
}

// FindOptimalRanges finds optimal fragment ranges for all three zones
// Uses default zones: tlshello, 1-3, 1-5
func (o *Optimizer) FindOptimalRanges(
	ctx context.Context,
	sizeRange Range,
	intervalRange Range,
) ([]ZoneResult, error) {
	// Define the three zones
	zones := []Zone{
		{Name: "tlshello", SizeRange: sizeRange, IntervalRange: intervalRange},
		{Name: "1-3", SizeRange: sizeRange, IntervalRange: intervalRange},
		{Name: "1-5", SizeRange: sizeRange, IntervalRange: intervalRange},
	}

	return o.FindOptimalRangesForZones(ctx, zones)
}

// FindOptimalRangesForZones finds optimal ranges for custom zones
func (o *Optimizer) FindOptimalRangesForZones(
	ctx context.Context,
	zones []Zone,
) ([]ZoneResult, error) {
	o.finder.SetProgressCallback(func(zone string, attempt, total int, success bool, sizeR, intervalR Range, latency time.Duration) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if success {
			fmt.Printf("  %s✓%s [%s%s%s] Try %s%d%s/%s%d%s: size=%s%s%s interval=%s%s%s %slatency=%dms%s\n",
				utils.Green, utils.Reset,
				utils.Cyan, zone, utils.Reset,
				utils.White, attempt, utils.Reset,
				utils.Gray, total, utils.Reset,
				utils.Green, sizeR, utils.Reset,
				utils.Green, intervalR, utils.Reset,
				utils.Yellow, latency.Milliseconds(), utils.Reset)
		} else {
			fmt.Printf("  %s✗%s [%s%s%s] Try %s%d%s/%s%d%s: size=%s%s%s interval=%s%s%s\n",
				utils.Red, utils.Reset,
				utils.Cyan, zone, utils.Reset,
				utils.White, attempt, utils.Reset,
				utils.Gray, total, utils.Reset,
				utils.Gray, sizeR, utils.Reset,
				utils.Gray, intervalR, utils.Reset)
		}
	})

	results := make([]ZoneResult, 0, len(zones))

	for _, zone := range zones {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		fmt.Printf("\n%s▸ Testing Zone:%s %s%s%s (size=%s%s%s, interval=%s%s%s)\n",
			utils.Bold+utils.Yellow, utils.Reset,
			utils.Cyan, zone.Name, utils.Reset,
			utils.Green, zone.SizeRange, utils.Reset,
			utils.Green, zone.IntervalRange, utils.Reset)

		result := o.finder.FindOne(zone)
		results = append(results, result)

		if result.Success {
			fmt.Printf("  %s✓ Found:%s size=%s%s%s interval=%s%s%s latency=%s%dms%s (success: %s%d/%d%s)\n",
				utils.Green, utils.Reset,
				utils.BrightGreen, result.SizeRange, utils.Reset,
				utils.BrightGreen, result.IntervalRange, utils.Reset,
				utils.Yellow, result.Latency.Milliseconds(), utils.Reset,
				utils.Cyan, result.SuccessCount, result.TotalTests, utils.Reset)
		} else {
			fmt.Printf("  %s✗ No working range found%s (%s%d%s tests)\n",
				utils.Red, utils.Reset,
				utils.Gray, result.TotalTests, utils.Reset)
		}
	}

	return results, nil
}

// GetBestResult returns the best result from multiple zone results
// Best = highest success rate, then lowest latency
func GetBestResult(results []ZoneResult) *ZoneResult {
	var best *ZoneResult

	for i := range results {
		r := &results[i]
		if !r.Success {
			continue
		}

		if best == nil {
			best = r
			continue
		}

		// Compare: prefer higher success rate, then lower latency
		bestRate := float64(best.SuccessCount) / float64(best.TotalTests)
		thisRate := float64(r.SuccessCount) / float64(r.TotalTests)

		if thisRate > bestRate || (thisRate == bestRate && r.Latency < best.Latency) {
			best = r
		}
	}

	return best
}

// PrintSummary prints a formatted summary of results
func PrintSummary(results []ZoneResult) {
	line := repeatString("═", 60)
	fmt.Printf("\n%s%s%s\n", utils.Cyan, line, utils.Reset)
	fmt.Printf("%s%sOPTIMIZATION RESULTS%s\n", utils.Bold, utils.Cyan, utils.Reset)
	fmt.Printf("%s%s%s\n", utils.Cyan, line, utils.Reset)

	for _, r := range results {
		if r.Success {
			rate := float64(r.SuccessCount) / float64(r.TotalTests) * 100
			fmt.Printf("%s✓%s %-10s size=%s%-10s%s interval=%s%-10s%s latency=%s%-10s%s success=%s%.0f%%%s\n",
				utils.Green, utils.Reset,
				r.Zone,
				utils.Green, r.SizeRange, utils.Reset,
				utils.Green, r.IntervalRange, utils.Reset,
				utils.Yellow, fmt.Sprintf("%dms", r.Latency.Milliseconds()), utils.Reset,
				utils.Cyan, rate, utils.Reset)
		} else {
			fmt.Printf("%s✗%s %-10s %sno working range found%s\n",
				utils.Red, utils.Reset,
				r.Zone,
				utils.Gray, utils.Reset)
		}
	}

	best := GetBestResult(results)
	fmt.Printf("%s%s%s\n", utils.Gray, repeatString("─", 60), utils.Reset)
	if best != nil {
		fmt.Printf("%s▸ BEST:%s %s%s%s with size=%s%s%s interval=%s%s%s latency=%s%dms%s\n",
			utils.Bold+utils.Green, utils.Reset,
			utils.Cyan, best.Zone, utils.Reset,
			utils.BrightGreen, best.SizeRange, utils.Reset,
			utils.BrightGreen, best.IntervalRange, utils.Reset,
			utils.Yellow, best.Latency.Milliseconds(), utils.Reset)
	} else {
		fmt.Printf("%s✗ No working configuration found%s\n", utils.Red, utils.Reset)
	}

	fmt.Printf("%s%s%s\n", utils.Cyan, line, utils.Reset)
}

func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
