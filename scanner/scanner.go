package scanner

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"piyazche/applog"
	"piyazche/config"
	"piyazche/utils"
	"piyazche/xray"

	"github.com/schollz/progressbar/v3"
)

// Scanner is the main scanner orchestrator
type Scanner struct {
	cfg         *config.Config
	ips         []string
	results     *ResultCollector
	quit        chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	startTime   time.Time
	debug       bool
	silent      bool
	onProgress  func(done, total int)
	stopOnce    sync.Once
	lifecycleMu sync.Mutex
	paused      atomic.Bool
}

// SetPaused pauses or resumes the scan (workers finish their in-flight IP, then
// block before picking up the next one).
func (s *Scanner) SetPaused(p bool) { s.paused.Store(p) }

// TogglePause flips the paused state and returns the new value.
func (s *Scanner) TogglePause() bool {
	np := !s.paused.Load()
	s.paused.Store(np)
	return np
}

// IsPaused reports whether the scan is currently paused.
func (s *Scanner) IsPaused() bool { return s.paused.Load() }

// SetSilent suppresses the scanner's own stdout UI (progress bar, headers,
// per-IP log lines). Used when driving the scanner from the TUI.
func (s *Scanner) SetSilent(silent bool) { s.silent = silent }

// SetIPs sets the IP list directly instead of loading from a file/CIDR.
func (s *Scanner) SetIPs(ips []string) { s.ips = ips }

// IPCount returns how many IPs are queued.
func (s *Scanner) IPCount() int { return len(s.ips) }

// SetOnResult streams every collected result to fn as it arrives.
func (s *Scanner) SetOnResult(fn func(Result)) { s.results.SetOnAdd(fn) }

// SetOnProgress streams progress (completed, total) updates.
func (s *Scanner) SetOnProgress(fn func(done, total int)) { s.onProgress = fn }

// NewScanner creates a new scanner
func NewScanner(cfg *config.Config) *Scanner {
	return NewScannerWithDebug(cfg, false)
}

// NewScannerWithDebug creates a new scanner with debug option
func NewScannerWithDebug(cfg *config.Config, debug bool) *Scanner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scanner{
		cfg:     cfg,
		results: NewResultCollector(),
		quit:    make(chan struct{}),
		ctx:     ctx,
		cancel:  cancel,
		debug:   debug,
	}
}

// LoadIPs loads IPs from file or CIDR string
func (s *Scanner) LoadIPs(source string, maxIPs int, shuffle bool) error {
	var ips []string
	var err error

	if _, statErr := os.Stat(source); statErr == nil {
		ips, err = utils.ParseIPsFromFile(source, s.cfg.Scan.SampleSize, shuffle)
	} else {
		ips, err = utils.ParseCIDRList(source, s.cfg.Scan.SampleSize)
		if shuffle {
			utils.ShuffleIPs(ips)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to load IPs: %w", err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no IPs found in source")
	}

	if maxIPs > 0 && maxIPs < len(ips) {
		ips = ips[:maxIPs]
	}

	s.ips = ips
	return nil
}

// warmup starts a single xray instance against the first IP and waits for it to
// be ready, paying one-time runtime/init costs so the concurrent worker burst
// that follows doesn't trip "xray not ready" on the first wave of IPs.
func (s *Scanner) warmup() {
	if len(s.ips) == 0 {
		return
	}
	port := utils.AcquirePort()
	defer utils.ReleasePort(port)

	cfgData, err := config.GenerateXrayConfig(s.cfg, s.ips[0], port)
	if err != nil {
		return
	}
	mgr := xray.NewManager()
	if err := mgr.Start(cfgData, port); err != nil {
		applog.Logf("warmup: xray start failed: %v", err)
		return
	}
	defer mgr.Stop()
	if err := mgr.WaitForReadyWithContext(s.ctx, 8*time.Second); err != nil {
		applog.Logf("warmup: not ready: %v", err)
		return
	}
	applog.Logf("warmup: xray ready")
}

// Run starts the scanning process
func (s *Scanner) Run() error {
	s.paused.Store(false)
	return s.RunWithContext(context.Background())
}

// RunWithContext lets native/mobile callers cancel warm-up and workers without
// racing Stop against initialization. Run keeps its original CLI semantics.
func (s *Scanner) RunWithContext(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return err
	}
	if len(s.ips) == 0 {
		return fmt.Errorf("no IPs loaded")
	}

	s.startTime = time.Now()
	// Make the scanner reusable: a prior Stop() cancelled the context and closed
	// quit, so recreate them (otherwise a second Run() would exit instantly).
	s.lifecycleMu.Lock()
	s.ctx, s.cancel = context.WithCancel(parent)
	s.quit = make(chan struct{})
	s.stopOnce = sync.Once{}
	s.lifecycleMu.Unlock()
	threads := s.cfg.Scan.Threads
	if threads <= 0 {
		threads = 16
	}
	total := len(s.ips)

	if !s.silent {
		fmt.Printf("%s%sStarting Scan%s\n", utils.Bold, utils.Cyan, utils.Reset)
		fmt.Printf("   %sIPs:%s %d  %sWorkers:%s %d\n\n", utils.Gray, utils.Reset, total, utils.Gray, utils.Reset, threads)
	}

	// Warm up xray-core once before the worker burst so the first wave of IPs
	// doesn't fail with "xray not ready" while the runtime pays one-time costs.
	s.warmup()

	jobs := make(chan string, threads*2)

	// In silent mode (TUI), don't spawn the stdout logger; workers get a nil
	// channel and stream results via the OnAdd callback instead.
	var logger chan string
	logDone := make(chan struct{})
	var bar *progressbar.ProgressBar
	if !s.silent {
		logger = make(chan string, threads*4)
		bar = progressbar.NewOptions(total,
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("IPs"),
			progressbar.OptionSetWidth(40),
			progressbar.OptionSetDescription("[cyan]Scanning[reset]"),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "[green]=[reset]",
				SaucerHead:    "[green]>[reset]",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
			progressbar.OptionUseANSICodes(true),
		)
		go func() {
			defer close(logDone)
			for msg := range logger {
				fmt.Printf("\r\033[K%s\n", msg)
				bar.RenderBlank()
			}
		}()
	} else {
		close(logDone)
	}

	var processed atomic.Int64
	var debugOnce sync.Once

	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		worker := NewWorker(i, s.cfg, s.results, &wg, jobs, s.quit, s.ctx, &processed, logger, s.debug, &debugOnce, &s.paused)
		worker.Start()
	}

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		lastProcessed := int64(0)
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				current := processed.Load()
				if current != lastProcessed {
					if s.silent {
						if s.onProgress != nil {
							s.onProgress(int(current), total)
						}
					} else {
						bar.Set(int(current))
					}
					lastProcessed = current
				}
			}
		}
	}()

	go func() {
		for _, ip := range s.ips {
			select {
			case <-s.ctx.Done():
				close(jobs)
				return
			case <-s.quit:
				close(jobs)
				return
			case jobs <- ip:
			}
		}
		close(jobs)
	}()

	wg.Wait()
	close(done)
	if logger != nil {
		close(logger)
	}
	<-logDone

	if s.silent {
		if s.onProgress != nil {
			s.onProgress(int(processed.Load()), total)
		}
		return parent.Err()
	}

	bar.Finish()
	s.printSummary()

	return parent.Err()
}

// Stop gracefully stops the scanner. Safe to call multiple times.
func (s *Scanner) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.stopOnce.Do(func() {
		s.paused.Store(false) // release any paused workers so they can observe the stop
		s.cancel()
		close(s.quit)
	})
}

// GetResults returns the result collector
func (s *Scanner) GetResults() *ResultCollector {
	return s.results
}

func (s *Scanner) printSummary() {
	duration := time.Since(s.startTime)
	total := s.results.Count()
	successful := s.results.SuccessCount()
	var successRate float64
	if total > 0 {
		successRate = float64(successful) / float64(total) * 100
	}

	fmt.Printf("\n%s%sScan Complete%s\n", utils.Bold, utils.Cyan, utils.Reset)
	fmt.Printf("  %s%-18s%s %s%v%s\n", utils.Gray, "Duration:", utils.Reset, utils.White, duration.Round(time.Second), utils.Reset)
	fmt.Printf("  %s%-18s%s %s%d%s\n", utils.Gray, "Total IPs tested:", utils.Reset, utils.White, total, utils.Reset)

	rateColor := utils.Green
	if successRate < 20 {
		rateColor = utils.Red
	} else if successRate < 50 {
		rateColor = utils.Yellow
	}
	fmt.Printf("  %s%-18s%s %s%d%s (%s%.1f%%%s)\n",
		utils.Gray, "Successful:", utils.Reset,
		utils.Green, successful, utils.Reset,
		rateColor, successRate, utils.Reset)

	if successful > 0 {
		sorted := s.results.GetSortedByLatency()
		latencyColor := utils.Green
		if sorted[0].LatencyMs > 2000 {
			latencyColor = utils.Red
		} else if sorted[0].LatencyMs > 1000 {
			latencyColor = utils.Yellow
		}
		fmt.Printf("  %s%-18s%s %s%dms%s %s(%s)%s\n",
			utils.Gray, "Best latency:", utils.Reset,
			latencyColor, sorted[0].LatencyMs, utils.Reset,
			utils.Dim, sorted[0].IP, utils.Reset)
	}
}

// SaveResults saves results to file
func (s *Scanner) SaveResults(format string, path string) error {
	if path == "" {
		path = GenerateOutputPath(format)
	}

	switch format {
	case "csv":
		return s.results.SaveToCSV(path)
	case "json":
		return s.results.SaveToJSON(path)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}
