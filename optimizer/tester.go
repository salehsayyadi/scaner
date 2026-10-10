package optimizer

import (
	"context"
	"fmt"
	"time"

	"piyazche/config"
	"piyazche/utils"
	"piyazche/xray"
)

// FragmentTester implements fragment testing using xray-core
type FragmentTester struct {
	cfg      *config.Config
	targetIP string
	testURL  string
	timeout  time.Duration
	debug    bool
	ctx      context.Context
}

// WithContext sets a context so the tester can be cancelled mid-optimization
// (each test returns immediately once the context is done).
func (t *FragmentTester) WithContext(ctx context.Context) *FragmentTester {
	t.ctx = ctx
	return t
}

func (t *FragmentTester) cancelled() bool {
	return t.ctx != nil && t.ctx.Err() != nil
}

// NewFragmentTester creates a new fragment tester for a specific IP
func NewFragmentTester(cfg *config.Config, targetIP string) *FragmentTester {
	timeout := time.Duration(cfg.Scan.Timeout) * time.Second
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	testURL := cfg.Scan.TestURL
	if testURL == "" {
		testURL = "https://www.gstatic.com/generate_204"
	}

	return &FragmentTester{
		cfg:      cfg,
		targetIP: targetIP,
		testURL:  testURL,
		timeout:  timeout,
		debug:    false,
	}
}

// WithDebug enables debug mode
func (t *FragmentTester) WithDebug(debug bool) *FragmentTester {
	t.debug = debug
	return t
}

// CreateTesterFunc creates a TesterFunc that can be used with the Finder
func (t *FragmentTester) CreateTesterFunc() TesterFunc {
	return func(zone string, sizeRange, intervalRange Range) (bool, time.Duration) {
		// Bail out instantly once cancelled so a long FindOne unwinds quickly.
		if t.cancelled() {
			return false, 0
		}

		port := utils.AcquirePort()
		defer utils.ReleasePort(port)

		fragment := config.FragmentSettings{
			Packets:  zone,
			Length:   sizeRange.String(),
			Interval: intervalRange.String(),
		}

		xrayConfig, err := config.GenerateXrayConfigWithFragment(t.cfg, t.targetIP, port, fragment)
		if err != nil {
			return false, 0
		}

		manager := xray.NewManagerWithDebug(t.debug)
		if err := manager.Start(xrayConfig, port); err != nil {
			return false, 0
		}
		defer manager.Stop()

		ctx := t.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if err := manager.WaitForReadyWithContext(ctx, 10*time.Second); err != nil {
			return false, 0
		}

		testResult := xray.TestConnectivityWithContext(ctx, port, t.testURL, t.timeout)

		if !testResult.Success {
			return false, 0
		}

		if t.cfg.Scan.MaxLatency > 0 {
			if testResult.Latency.Milliseconds() > int64(t.cfg.Scan.MaxLatency) {
				return false, 0
			}
		}

		return true, testResult.Latency
	}
}

// TestSinglePoint tests a single fragment configuration (for check mode)
type TestSingleResult struct {
	Success bool
	Latency time.Duration
	Error   string
}

// TestSingle tests a specific fragment configuration
func (t *FragmentTester) TestSingle(zone string, size, interval int) TestSingleResult {
	result := TestSingleResult{}

	port := utils.AcquirePort()
	defer utils.ReleasePort(port)

	sizeMax := size + 5
	intervalMax := interval + 5

	fragment := config.FragmentSettings{
		Packets:  zone,
		Length:   fmt.Sprintf("%d-%d", size, sizeMax),
		Interval: fmt.Sprintf("%d-%d", interval, intervalMax),
	}

	xrayConfig, err := config.GenerateXrayConfigWithFragment(t.cfg, t.targetIP, port, fragment)
	if err != nil {
		result.Error = fmt.Sprintf("failed to generate xray config: %v", err)
		return result
	}

	manager := xray.NewManagerWithDebug(t.debug)
	if err := manager.Start(xrayConfig, port); err != nil {
		result.Error = fmt.Sprintf("failed to start xray: %v", err)
		return result
	}
	defer manager.Stop()

	if err := manager.WaitForReady(10 * time.Second); err != nil {
		result.Error = fmt.Sprintf("xray not ready: %v", err)
		return result
	}

	testResult := xray.TestConnectivity(port, t.testURL, t.timeout)

	result.Success = testResult.Success
	result.Latency = testResult.Latency

	if testResult.Error != nil {
		result.Error = testResult.Error.Error()
	}

	if result.Success && t.cfg.Scan.MaxLatency > 0 {
		if result.Latency.Milliseconds() > int64(t.cfg.Scan.MaxLatency) {
			result.Success = false
			result.Error = fmt.Sprintf("latency %dms exceeds max %dms",
				result.Latency.Milliseconds(), t.cfg.Scan.MaxLatency)
		}
	}

	return result
}
