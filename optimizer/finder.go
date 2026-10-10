package optimizer

// Finder finds optimal fragment ranges using golden-ratio based search
//
// Algorithm overview:
// 1. Start with user's full range [min, max]
// 2. Test the range
// 3. On SUCCESS: narrow the range toward center (remove 38% total)
// 4. On FAILURE: shift to unexplored region
// 5. Repeat until:
//   - Success threshold reached (e.g., 50% of tests succeeded)
//   - Max tries exhausted
//   - Range too narrow to continue
//
// The algorithm uses golden ratio for mathematically optimal convergence.
type Finder struct {
	config     FinderConfig
	tester     TesterFunc
	onProgress ProgressFunc
}

// NewFinder creates a new range finder
func NewFinder(config FinderConfig, tester TesterFunc) *Finder {
	if !config.isValid() {
		config = DefaultFinderConfig()
	}
	return &Finder{
		config: config,
		tester: tester,
	}
}

// SetProgressCallback sets an optional callback for progress updates
func (f *Finder) SetProgressCallback(cb ProgressFunc) {
	f.onProgress = cb
}

// FindAll finds optimal ranges for all provided zones
// Returns a map of zone name -> ZoneResult
func (f *Finder) FindAll(zones []Zone) map[string]ZoneResult {
	results := make(map[string]ZoneResult, len(zones))

	for _, zone := range zones {
		if !zone.SizeRange.IsValid() || !zone.IntervalRange.IsValid() {
			results[zone.Name] = ZoneResult{
				Zone:    zone.Name,
				Success: false,
			}
			continue
		}

		result := f.findZone(zone)
		results[zone.Name] = result
	}

	return results
}

// FindOne finds optimal range for a single zone
func (f *Finder) FindOne(zone Zone) ZoneResult {
	if !zone.SizeRange.IsValid() || !zone.IntervalRange.IsValid() {
		return ZoneResult{Zone: zone.Name, Success: false}
	}
	return f.findZone(zone)
}

// findZone implements the core search algorithm for one zone
func (f *Finder) findZone(zone Zone) ZoneResult {
	currentSize := zone.SizeRange
	currentInterval := zone.IntervalRange

	if f.config.EnableCorrelation {
		currentInterval = correlateInterval(currentSize, zone.IntervalRange)
	}

	var best *ZoneResult
	successCount := 0
	failStreak := 0
	totalTests := 0

	for attempt := 1; attempt <= f.config.MaxTriesPerZone; attempt++ {
		currentSize = clampRange(currentSize, zone.SizeRange)
		currentInterval = clampRange(currentInterval, zone.IntervalRange)

		success, latency := f.tester(zone.Name, currentSize, currentInterval)
		totalTests++

		if f.onProgress != nil {
			f.onProgress(zone.Name, attempt, f.config.MaxTriesPerZone,
				success, currentSize, currentInterval, latency)
		}

		if success {
			successCount++
			failStreak = 0

			if best == nil || latency < best.Latency {
				best = &ZoneResult{
					Zone:          zone.Name,
					SizeRange:     currentSize,
					IntervalRange: currentInterval,
					Latency:       latency,
					Success:       true,
				}
			}

			// Early exit when we have enough successful samples
			requiredSuccesses := int(float64(f.config.MaxTriesPerZone) * f.config.SuccessThreshold)
			if requiredSuccesses < 3 {
				requiredSuccesses = 3
			}
			if successCount >= requiredSuccesses {
				break
			}

			// Narrow toward center to refine the working range
			if currentSize.Width() > f.config.MinRangeWidth*2 {
				currentSize = narrow(currentSize)
				currentInterval = narrow(currentInterval)
			}

		} else {
			failStreak++

			if failStreak >= 3 && best != nil {
				break
			}

			// Shift search to unexplored region
			currentSize, currentInterval = shift(
				currentSize, currentInterval,
				zone.SizeRange, zone.IntervalRange,
				attempt,
			)

			if f.config.EnableCorrelation {
				currentInterval = correlateInterval(currentSize, zone.IntervalRange)
			}
		}
	}

	if best != nil {
		best.SuccessCount = successCount
		best.TotalTests = totalTests
		return *best
	}

	return ZoneResult{
		Zone:       zone.Name,
		Success:    false,
		TotalTests: totalTests,
	}
}

// narrow shrinks a range toward its center by NarrowRatio from each side
func narrow(r Range) Range {
	shrink := int(float64(r.Width()) * NarrowRatio)
	if shrink < 1 {
		shrink = 1
	}

	newMin := r.Min + shrink
	newMax := r.Max - shrink

	if newMin >= newMax || newMax-newMin < 2 {
		return r
	}

	return Range{
		Min: newMin,
		Max: newMax,
	}
}

// shift moves search to unexplored region after failure, cycling through strategies
func shift(
	currentSize, currentInterval Range,
	boundsSize, boundsInterval Range,
	attempt int,
) (Range, Range) {
	strategy := (attempt - 1) % 4

	switch strategy {
	case 0:
		return shiftLower(currentSize, boundsSize),
			shiftLower(currentInterval, boundsInterval)
	case 1:
		return shiftUpper(currentSize, boundsSize),
			shiftUpper(currentInterval, boundsInterval)
	case 2:
		// Small chunks + longer gaps works well against packet inspection
		return shiftLower(currentSize, boundsSize),
			shiftUpper(currentInterval, boundsInterval)
	case 3:
		return widen(currentSize, boundsSize),
			widen(currentInterval, boundsInterval)
	}

	return currentSize, currentInterval
}

// shiftLower moves range toward lower values
func shiftLower(current, bounds Range) Range {
	shiftAmt := int(float64(current.Width()) * ShiftRatio)
	if shiftAmt < 1 {
		shiftAmt = 1
	}
	return Range{
		Min: maxInt(bounds.Min, current.Min-shiftAmt),
		Max: maxInt(bounds.Min+5, current.Max-shiftAmt),
	}
}

// shiftUpper moves range toward higher values
func shiftUpper(current, bounds Range) Range {
	shiftAmt := int(float64(current.Width()) * ShiftRatio)
	if shiftAmt < 1 {
		shiftAmt = 1
	}
	return Range{
		Min: minInt(bounds.Max-5, current.Min+shiftAmt),
		Max: minInt(bounds.Max, current.Max+shiftAmt),
	}
}

// widen expands range back toward original bounds
func widen(current, bounds Range) Range {
	expandAmt := int(float64(current.Width()) * ShiftRatio)
	if expandAmt < 1 {
		expandAmt = 1
	}
	return Range{
		Min: maxInt(bounds.Min, current.Min-expandAmt),
		Max: minInt(bounds.Max, current.Max+expandAmt),
	}
}

// correlateInterval adjusts interval range based on size using inverse relationship
//
// Physics basis:
// - TLS ClientHello ≈ 300 bytes
// - Fragment size S → Number of fragments ≈ 300/S
// - More fragments need longer intervals to evade DPI reassembly
// - Therefore: interval ∝ 1/size (inverse relationship)
//
// Formula: idealInterval = ClientHelloSize / midpointSize
func correlateInterval(sizeRange Range, intervalBounds Range) Range {
	midSize := sizeRange.Mid()
	if midSize <= 0 {
		midSize = 1
	}

	// Calculate ideal interval using inverse relationship
	idealInterval := int(ClientHelloSize / float64(midSize))

	// Clamp ideal to bounds
	idealInterval = clampInt(idealInterval, intervalBounds.Min, intervalBounds.Max)

	// Use half of original interval width for the correlated range
	// This keeps a reasonable exploration space
	halfWidth := intervalBounds.Width() / 2
	if halfWidth < 5 {
		halfWidth = 5
	}

	return Range{
		Min: clampInt(idealInterval-halfWidth/2, intervalBounds.Min, intervalBounds.Max-2),
		Max: clampInt(idealInterval+halfWidth/2, intervalBounds.Min+2, intervalBounds.Max),
	}
}

// clampRange ensures a range stays within bounds
func clampRange(r, bounds Range) Range {
	return Range{
		Min: clampInt(r.Min, bounds.Min, bounds.Max-1),
		Max: clampInt(r.Max, bounds.Min+1, bounds.Max),
	}
}

// isValid checks if config has valid values
func (c FinderConfig) isValid() bool {
	return c.MaxTriesPerZone > 0 &&
		c.SuccessThreshold > 0 && c.SuccessThreshold <= 1 &&
		c.MinRangeWidth > 0
}

// Helper functions
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
