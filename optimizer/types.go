package optimizer

import (
	"fmt"
	"time"
)

// Range represents a min-max range for fragment parameters
type Range struct {
	Min int
	Max int
}

// Width returns the range width (max - min)
func (r Range) Width() int {
	return r.Max - r.Min
}

// Mid returns the midpoint of the range
func (r Range) Mid() int {
	return (r.Min + r.Max) / 2
}

// String returns the range as "min-max" format for xray config
func (r Range) String() string {
	return fmt.Sprintf("%d-%d", r.Min, r.Max)
}

// IsValid checks if range is valid (min < max, both positive)
func (r Range) IsValid() bool {
	return r.Min > 0 && r.Max > r.Min
}

// Zone represents a packet fragmentation zone to test
// Each zone uses a different packet fragmentation strategy
type Zone struct {
	Name          string // "tlshello", "1-3", "1-5"
	SizeRange     Range  // User-provided fragment size bounds
	IntervalRange Range  // User-provided interval bounds (milliseconds)
}

// ZoneResult contains the optimization result for one zone
type ZoneResult struct {
	Zone          string        // Zone name that was tested
	SizeRange     Range         // Best working size range found
	IntervalRange Range         // Best working interval range found
	Latency       time.Duration // Best latency achieved
	SuccessCount  int           // Number of successful tests
	TotalTests    int           // Total tests performed
	Success       bool          // Whether a working range was found
}

// FinderConfig configures the range finder behavior
type FinderConfig struct {
	MaxTriesPerZone   int     // Maximum test attempts per zone (e.g., 20)
	SuccessThreshold  float64 // Early exit when success rate >= this (0.0-1.0, e.g., 0.5)
	MinRangeWidth     int     // Stop narrowing when range width < this (e.g., 5)
	EnableCorrelation bool    // Apply size-interval inverse correlation
}

// DefaultFinderConfig returns sensible defaults
func DefaultFinderConfig() FinderConfig {
	return FinderConfig{
		MaxTriesPerZone:   20,
		SuccessThreshold:  0.5,
		MinRangeWidth:     5,
		EnableCorrelation: true,
	}
}

// TesterFunc is the function signature for testing a fragment configuration
// Parameters:
//   - zone: the packet type ("tlshello", "1-3", "1-5")
//   - sizeRange: fragment size range to test
//   - intervalRange: interval range in milliseconds to test
//
// Returns:
//   - success: true if connection succeeded
//   - latency: connection latency (only meaningful if success=true)
type TesterFunc func(zone string, sizeRange, intervalRange Range) (success bool, latency time.Duration)

// ProgressFunc is called after each test attempt
// Parameters:
//   - zone: current zone being tested
//   - attempt: current attempt number (1-based)
//   - totalAttempts: maximum attempts for this zone
//   - success: whether this attempt succeeded
//   - sizeRange: size range that was tested
//   - intervalRange: interval range that was tested
//   - latency: latency if successful, 0 if failed
type ProgressFunc func(zone string, attempt, totalAttempts int, success bool, sizeRange, intervalRange Range, latency time.Duration)
