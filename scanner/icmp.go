package scanner

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"piyazche/config"
	"piyazche/utils"

	"github.com/schollz/progressbar/v3"
)

// ICMPScanner scans IPs using ICMP ping (no xray-core needed)
type ICMPScanner struct {
	cfg       *config.Config
	ips       []string
	results   *ResultCollector
	quit      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time
	stopOnce  sync.Once
}

// NewICMPScanner creates a new ICMP scanner
func NewICMPScanner(cfg *config.Config) *ICMPScanner {
	ctx, cancel := context.WithCancel(context.Background())
	return &ICMPScanner{
		cfg:     cfg,
		results: NewResultCollector(),
		quit:    make(chan struct{}),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// LoadIPs loads IPs from file or CIDR string
func (s *ICMPScanner) LoadIPs(source string, maxIPs int, shuffle bool) error {
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

// Run starts the ICMP scanning process
func (s *ICMPScanner) Run() error {
	if len(s.ips) == 0 {
		return fmt.Errorf("no IPs loaded")
	}

	s.startTime = time.Now()
	threads := s.cfg.Scan.Threads
	if threads <= 0 {
		threads = 16
	}

	// ICMP doesn't need long timeouts - 1 second is plenty
	timeout := time.Duration(s.cfg.Scan.Timeout) * time.Second
	if timeout == 0 || timeout > 2*time.Second {
		timeout = 1 * time.Second
	}

	// ICMP mode uses 3 retries by default
	retries := 3

	// Detect ping mode (ICMP needs root, falls back to TCP)
	pingMode := utils.PingModeString()

	fmt.Printf("%s%sConnectivity Scan%s (%s%s%s)\n", utils.Bold, utils.Cyan, utils.Reset, utils.Yellow, pingMode, utils.Reset)
	fmt.Printf("   %sIPs:%s %d  %sWorkers:%s %d  %sTimeout:%s %v  %sRetries:%s %d\n\n",
		utils.Gray, utils.Reset, len(s.ips),
		utils.Gray, utils.Reset, threads,
		utils.Gray, utils.Reset, timeout,
		utils.Gray, utils.Reset, retries)

	jobs := make(chan string, threads*2)
	logger := make(chan string, threads*4)

	bar := progressbar.NewOptions(len(s.ips),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("IPs"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetDescription("[cyan]Pinging[reset]"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionUseANSICodes(true),
	)

	var processed atomic.Int64

	logDone := make(chan struct{})
	go func() {
		defer close(logDone)
		for msg := range logger {
			fmt.Printf("\r\033[K%s\n", msg)
			bar.RenderBlank()
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go s.worker(&wg, jobs, &processed, logger, timeout, retries)
	}

	// Progress updater
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
					bar.Set(int(current))
					lastProcessed = current
				}
			}
		}
	}()

	// Feed jobs
	go func() {
		for _, ip := range s.ips {
			select {
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
	close(logger)
	<-logDone

	bar.Finish()
	fmt.Println()

	elapsed := time.Since(s.startTime)
	successCount := s.results.SuccessCount()
	totalCount := s.results.Count()

	fmt.Printf("%sScan Complete%s in %s%.1fs%s\n",
		utils.Green, utils.Reset,
		utils.Cyan, elapsed.Seconds(), utils.Reset)
	fmt.Printf("   %sSuccess:%s %s%d%s/%s%d%s\n\n",
		utils.Gray, utils.Reset,
		utils.Green, successCount, utils.Reset,
		utils.White, totalCount, utils.Reset)

	return nil
}

func (s *ICMPScanner) worker(wg *sync.WaitGroup, jobs <-chan string, processed *atomic.Int64, logger chan<- string, timeout time.Duration, retries int) {
	defer wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.quit:
			return
		case ip, ok := <-jobs:
			if !ok {
				return
			}
			select {
			case <-s.ctx.Done():
				return
			default:
			}
			s.processIP(ip, processed, logger, timeout, retries)
		}
	}
}

func (s *ICMPScanner) processIP(ip string, processed *atomic.Int64, logger chan<- string, timeout time.Duration, retries int) {
	defer processed.Add(1)

	select {
	case <-s.ctx.Done():
		return
	default:
	}

	result := utils.PingWithRetries(ip, timeout, retries)

	scanResult := Result{
		IP:      ip,
		Success: result.Success,
		Latency: result.Latency,
	}

	if result.Error != nil {
		scanResult.Error = result.Error.Error()
	}

	s.results.Add(scanResult)

	if result.Success {
		maxLatency := s.cfg.Scan.MaxLatency
		if maxLatency > 0 && result.Latency.Milliseconds() > int64(maxLatency) {
			// Exceeds max latency, mark as failed
			scanResult.Success = false
			scanResult.Error = fmt.Sprintf("latency %dms exceeds max %dms", result.Latency.Milliseconds(), maxLatency)
		} else {
			logger <- fmt.Sprintf("  %s✓%s %s%s%s  %s%dms%s",
				utils.Green, utils.Reset,
				utils.Cyan, ip, utils.Reset,
				utils.Yellow, result.Latency.Milliseconds(), utils.Reset)
		}
	}
}

// Stop stops the scanner. Safe to call multiple times.
func (s *ICMPScanner) Stop() {
	s.stopOnce.Do(func() {
		close(s.quit)
		s.cancel()
	})
}

// GetResults returns the result collector
func (s *ICMPScanner) GetResults() *ResultCollector {
	return s.results
}

// SaveResults saves results to file
func (s *ICMPScanner) SaveResults(format, path string) error {
	switch format {
	case "csv":
		return s.results.SaveToCSV(path)
	case "json":
		return s.results.SaveToJSON(path)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}
