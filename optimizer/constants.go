package optimizer

import "math"

// Golden ratio constants for optimal search convergence
//
// The golden ratio (φ ≈ 0.618) is used for shift operations.
// Narrowing uses a more moderate ratio to allow more iterations.
const (
	// Phi is the golden ratio ≈ 0.618
	Phi = 0.6180339887498949

	// NarrowRatio is how much to shrink from EACH side on success
	// Using 0.10 (10% from each side = 20% total reduction)
	// This is more moderate than golden ratio, allowing more iterations
	NarrowRatio = 0.10

	// ShiftRatio is how much to shift the range on failure
	// (1 - Phi) ≈ 0.382
	ShiftRatio = 1 - Phi

	// ClientHelloSize is approximate TLS ClientHello size in bytes
	// Used for size-interval correlation calculation
	ClientHelloSize = 300.0
)

// init validates constants at startup
func init() {
	// Verify golden ratio calculation
	expectedPhi := (math.Sqrt(5) - 1) / 2
	if math.Abs(Phi-expectedPhi) > 0.0001 {
		panic("golden ratio constant is incorrect")
	}
}
