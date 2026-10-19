package scanner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"piyazche/applog"
	"piyazche/config"
	"piyazche/utils"
	"piyazche/xray"
)

// Worker represents a scanner worker
type Worker struct {
	id        int
	cfg       *config.Config
	results   *ResultCollector
	wg        *sync.WaitGroup
	jobs      <-chan string
	quit      <-chan struct{}
	ctx       context.Context
	processed *atomic.Int64
	logger    chan<- string
	debug     bool
	debugOnce *sync.Once
	paused    *atomic.Bool
	// onActive, when set, is told when an IP starts (true) and finishes (false).
	onActive func(ip string, active bool)
	// Optional probe dependency permits deterministic regression coverage.
	tcpConnect func(string, int, time.Duration) (time.Duration, error)
}

// NewWorker creates a new scanner worker
func NewWorker(id int, cfg *config.Config, results *ResultCollector,
	wg *sync.WaitGroup, jobs <-chan string, quit <-chan struct{}, ctx context.Context, processed *atomic.Int64, logger chan<- string, debug bool, debugOnce *sync.Once, paused *atomic.Bool) *Worker {
	return &Worker{
		id:        id,
		cfg:       cfg,
		results:   results,
		wg:        wg,
		jobs:      jobs,
		quit:      quit,
		ctx:       ctx,
		processed: processed,
		logger:    logger,
		debug:     debug,
		debugOnce: debugOnce,
		paused:    paused,
	}
}

// Start starts the worker
func (w *Worker) Start() {
	go w.run()
}

func (w *Worker) run() {
	defer w.wg.Done()

	// Stagger startup so the first wave of workers doesn't spin up xray-core all
	// at once (which made the first few IPs fail with "xray not ready").
	if w.id > 0 {
		delay := time.Duration(w.id) * 40 * time.Millisecond
		if delay > 1500*time.Millisecond {
			delay = 1500 * time.Millisecond
		}
		select {
		case <-w.ctx.Done():
			return
		case <-time.After(delay):
		}
	}

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.quit:
			return
		case ip, ok := <-w.jobs:
			if !ok {
				return
			}
			select {
			case <-w.ctx.Done():
				return
			default:
			}
			w.waitIfPaused()
			select {
			case <-w.ctx.Done():
				return
			default:
			}
			w.processIP(ip)
		}
	}
}

// waitIfPaused blocks while the scan is paused (used by the TUI pause control).
// It polls every 200ms; the interval is intentional and keeps the worker
// responsive to resume/stop without the bookkeeping of a sync.Cond. A handful of
// idle wake-ups per second per worker is negligible.
func (w *Worker) waitIfPaused() {
	if w.paused == nil {
		return
	}
	for w.paused.Load() {
		select {
		case <-w.ctx.Done():
			return
		case <-w.quit:
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (w *Worker) processIP(ip string) {
	defer w.processed.Add(1)
	if w.onActive != nil {
		w.onActive(ip, true)
		defer w.onActive(ip, false)
	}

	select {
	case <-w.ctx.Done():
		return
	default:
	}

	// Give failed connections a few chances
	maxRetries := w.cfg.Scan.Retries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	result := Result{IP: ip, TestInterface: w.cfg.Network.Interface}
	probed := false
	if usePrecheck(w.cfg) {
		probed = true
		tries := 1
		if maxRetries > 1 {
			tries = 2
		}
		var err error
		for i := 0; i < tries && w.ctx.Err() == nil; i++ {
			var lat time.Duration
			if lat, err = w.probeTCP(ip, w.cfg.Proxy.Port, precheckTimeout(w.cfg)); err == nil {
				result.TCPOK, result.TCPMs = true, lat.Milliseconds()
				break
			}
		}
		if w.ctx.Err() != nil {
			return
		}
		if !result.TCPOK {
			applog.Logf("ip=%s TCP pre-check failed (%v); confirming through Xray with full timeout", ip, err)
		}
	}

	var m *xray.MeasureResult
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		m = w.measureIP(ip)
		if m.Success {
			break
		}

		// Small pause before retry so we don't flood the server
		if attempt < maxRetries-1 {
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	if !probed && w.ctx.Err() == nil {
		if lat, err := w.probeTCP(ip, w.cfg.Proxy.Port, tcpProbeTimeout(w.cfg)); err == nil {
			result.TCPOK = true
			result.TCPMs = lat.Milliseconds()
		}
	}
	if m != nil {
		result.Connected = m.Connected
		result.Success = m.Success
		result.Latency = m.MinLatency
		result.AvgLatencyMs = m.AvgLatency.Milliseconds()
		result.JitterMs = m.Jitter.Milliseconds()
		result.HandshakeMs = m.Handshake.Milliseconds()
		result.LossPercent = m.LossPercent
		result.Samples = m.Samples
		result.StatusCode = m.StatusCode
		result.UploadMbps = m.UploadMbps
		result.UploadTested = m.UploadTested
		result.DownloadMbps = m.DownloadMbps
		result.DownloadTested = m.DownloadTested
		if !m.Success && m.Error != nil {
			result.Error = m.Error.Error()
		}
	}

	w.results.Add(result)

	applog.Logf("ip=%-15s ok=%-5v ping=%5dms tcp=%5dms(ok=%v) hs=%5dms loss=%3.0f%% up=%.2f down=%.2f samples=%d err=%q",
		ip, result.Success, result.LatencyMs, result.TCPMs, result.TCPOK, result.HandshakeMs, result.LossPercent,
		result.UploadMbps, result.DownloadMbps, result.Samples, result.Error)

	if w.logger != nil {
		var logMsg string
		if result.Success {
			extra := fmt.Sprintf("%s loss %.0f%%", utils.Gray+"─"+utils.Reset, result.LossPercent)
			if result.UploadMbps > 0 {
				extra += fmt.Sprintf(" %s↑%.1fMb%s", utils.Magenta, result.UploadMbps, utils.Reset)
			}
			logMsg = fmt.Sprintf("%s✓%s %s%s%s %s─%s %s%dms%s %s",
				utils.Green, utils.Reset, utils.Cyan, ip, utils.Reset, utils.Gray, utils.Reset, utils.Yellow, result.Latency.Milliseconds(), utils.Reset, extra)
		} else {
			errMsg := result.Error
			if len(errMsg) > 50 {
				errMsg = errMsg[:50] + "..."
			}
			logMsg = fmt.Sprintf("%s✗%s %s%s%s %s─ %s%s", utils.Red, utils.Reset, utils.Gray, ip, utils.Reset, utils.Gray, errMsg, utils.Reset)
		}
		select {
		case w.logger <- logMsg:
		default:
		}
	}
}

func (w *Worker) measureIP(ip string) *xray.MeasureResult {
	select {
	case <-w.ctx.Done():
		return &xray.MeasureResult{IP: ip, Error: w.ctx.Err()}
	default:
	}

	// Grab a free port for this test's SOCKS proxy
	port := utils.AcquirePort()
	defer utils.ReleasePort(port)

	xrayConfig, err := config.GenerateXrayConfig(w.cfg, ip, port)
	if err != nil {
		return &xray.MeasureResult{
			IP:    ip,
			Error: fmt.Errorf("failed to generate xray config: %w", err),
		}
	}

	// Debug output only for the first IP to avoid log spam
	showDebug := false
	if w.debug {
		w.debugOnce.Do(func() {
			showDebug = true
			w.printFragmentDebugInfo(ip, port)
		})
	}

	manager := xray.NewManagerWithDebug(showDebug)
	if err := manager.Start(xrayConfig, port); err != nil {
		return &xray.MeasureResult{
			IP:    ip,
			Error: fmt.Errorf("failed to start xray: %w", err),
		}
	}
	defer manager.Stop()

	select {
	case <-w.ctx.Done():
		return &xray.MeasureResult{IP: ip, Error: w.ctx.Err()}
	default:
	}

	// Wait for xray to spin up and open the port. Give it a generous window —
	// under a burst of concurrent starts the first wave can take a few seconds.
	readyTimeout := 6 * time.Second
	if err := manager.WaitForReadyWithContext(w.ctx, readyTimeout); err != nil {
		return &xray.MeasureResult{
			IP:    ip,
			Error: fmt.Errorf("xray not ready: %w", err),
		}
	}

	opts := xray.MeasureOptions{
		TestURL:       w.cfg.Scan.TestURL,
		Timeout:       time.Duration(w.cfg.Scan.Timeout) * time.Second,
		Samples:       w.cfg.Scan.PingSamples,
		Realistic:     w.cfg.Scan.RealisticPing,
		LossTest:      w.cfg.Scan.LossTest,
		LossSamples:   w.cfg.Scan.LossSamples,
		HandshakeTest: w.cfg.Scan.HandshakeTest,
		MaxLatency:    w.cfg.Scan.MaxLatency,
		UploadTest:    w.cfg.Scan.UploadTest,
		UploadURL:     w.cfg.Scan.UploadURL,
		UploadBytes:   w.cfg.Scan.UploadBytes,
		DownloadTest:  w.cfg.Scan.DownloadTest,
		DownloadURL:   w.cfg.Scan.DownloadURL,
		DownloadBytes: w.cfg.Scan.DownloadBytes,
	}

	m := xray.TestMeasure(w.ctx, port, opts)
	m.IP = ip

	// Apply quality thresholds (latency / loss / throughput).
	if m.Success {
		switch {
		case w.cfg.Scan.MaxLatency > 0 && m.MinLatency.Milliseconds() > int64(w.cfg.Scan.MaxLatency):
			m.Success = false
			m.Error = fmt.Errorf("connected, but latency %dms exceeds max %dms", m.MinLatency.Milliseconds(), w.cfg.Scan.MaxLatency)
		case w.cfg.Scan.MaxLoss > 0 && m.LossPercent > w.cfg.Scan.MaxLoss:
			m.Success = false
			m.Error = fmt.Errorf("connected, but loss %.0f%% exceeds max %.0f%%", m.LossPercent, w.cfg.Scan.MaxLoss)
		case w.cfg.Scan.UploadTest && w.cfg.Scan.MinUpload > 0 && m.UploadMbps < w.cfg.Scan.MinUpload:
			m.Success = false
			m.Error = fmt.Errorf("connected, but upload %.2f Mbps below min %.2f", m.UploadMbps, w.cfg.Scan.MinUpload)
		case w.cfg.Scan.DownloadTest && w.cfg.Scan.MinDownload > 0 && m.DownloadMbps < w.cfg.Scan.MinDownload:
			m.Success = false
			m.Error = fmt.Errorf("connected, but download %.2f Mbps below min %.2f", m.DownloadMbps, w.cfg.Scan.MinDownload)
		}
	}

	// Hard per-IP log so upload/loss/speed behaviour is fully diagnosable.
	errStr := ""
	if m.Error != nil {
		errStr = m.Error.Error()
	}
	applog.Logf("MEASURE %-15s ok=%-5v hs=%dms ping=%dms avg=%dms jit=%dms loss=%.0f%%(%d) up=%.2f(t=%v e=%q) down=%.2f(t=%v e=%q) status=%d err=%q",
		ip, m.Success, m.Handshake.Milliseconds(), m.MinLatency.Milliseconds(), m.AvgLatency.Milliseconds(),
		m.Jitter.Milliseconds(), m.LossPercent, m.LossSamples,
		m.UploadMbps, m.UploadTested, m.UploadErr,
		m.DownloadMbps, m.DownloadTested, m.DownloadErr,
		m.StatusCode, errStr)

	return m
}

// usePrecheck reports whether a plain TCP connect predicts reachability. QUIC
// (h3 ALPN / quic or kcp transports) runs over UDP, so it is not pre-checked.
func usePrecheck(cfg *config.Config) bool {
	if !cfg.Scan.TCPPrecheck {
		return false
	}
	if t := strings.ToLower(cfg.Proxy.Type); t == "quic" || t == "kcp" || t == "mkcp" {
		return false
	}
	if cfg.Proxy.TLS != nil {
		for _, a := range cfg.Proxy.TLS.ALPN {
			if strings.EqualFold(strings.TrimSpace(a), "h3") {
				return false
			}
		}
	}
	return true
}

// precheckTimeout: an IP whose TCP handshake takes longer than the latency
// limit can never pass, so wait at most that long (1–5 s, ≤ scan timeout).
func precheckTimeout(cfg *config.Config) time.Duration {
	t := 2500 * time.Millisecond
	if cfg.Scan.MaxLatency > 0 {
		t = time.Duration(cfg.Scan.MaxLatency) * time.Millisecond
	}
	if t < time.Second {
		t = time.Second
	}
	if t > 5*time.Second {
		t = 5 * time.Second
	}
	if st := cfg.GetTimeout(); st > 0 && t > st {
		t = st
	}
	return t
}

// tcpProbeTimeout bounds the raw TCP-connect probe to a short, fixed budget —
// it's a cheap reachability check, not a full measurement, so it shouldn't
// stretch out to the (possibly much larger) configured scan timeout.
func tcpProbeTimeout(cfg *config.Config) time.Duration {
	t := cfg.GetTimeout()
	if t > 5*time.Second || t <= 0 {
		t = 5 * time.Second
	}
	return t
}

// printFragmentDebugInfo prints colorized fragment debug information
func (w *Worker) printFragmentDebugInfo(ip string, port int) {
	fmt.Printf("\n%s%sDebug Info - First IP Test%s\n", utils.Bold, utils.Magenta, utils.Reset)
	fmt.Printf("  %s%-18s%s %s%s%s\n", utils.Gray, "Target IP:", utils.Reset, utils.Cyan, ip, utils.Reset)
	fmt.Printf("  %s%-18s%s %s%d%s\n", utils.Gray, "Local Port:", utils.Reset, utils.Yellow, port, utils.Reset)

	// Fragment settings
	fmt.Printf("\n  %s%s▸ Fragment Configuration%s\n", utils.Bold, utils.Yellow, utils.Reset)

	modeColor := utils.Green
	if w.cfg.Fragment.Mode == "off" {
		modeColor = utils.Red
	} else if w.cfg.Fragment.Mode == "auto" {
		modeColor = utils.Blue
	}
	fmt.Printf("    %s%-16s%s %s%s%s\n", utils.Gray, "Mode:", utils.Reset, modeColor, w.cfg.Fragment.Mode, utils.Reset)
	fmt.Printf("    %s%-16s%s %s%s%s\n", utils.Gray, "Packets:", utils.Reset, utils.Magenta, w.cfg.Fragment.Packets, utils.Reset)

	if w.cfg.Fragment.Mode == "manual" || w.cfg.Fragment.Mode == "" {
		fmt.Printf("    %s%-16s%s %s%s%s %sbytes%s\n", utils.Gray, "Length:", utils.Reset,
			utils.Green, w.cfg.Fragment.Manual.Length, utils.Reset, utils.Dim, utils.Reset)
		fmt.Printf("    %s%-16s%s %s%s%s %sms%s\n", utils.Gray, "Interval:", utils.Reset,
			utils.Green, w.cfg.Fragment.Manual.Interval, utils.Reset, utils.Dim, utils.Reset)
	}

	// Connection info
	fmt.Printf("\n  %s%s▸ Connection Details%s\n", utils.Bold, utils.Yellow, utils.Reset)

	// Show SNI based on method
	if w.cfg.Proxy.Method == "tls" && w.cfg.Proxy.TLS != nil {
		fmt.Printf("    %s%-16s%s %s%s%s\n", utils.Gray, "SNI:", utils.Reset, utils.Cyan, w.cfg.Proxy.TLS.SNI, utils.Reset)
		fmt.Printf("    %s%-16s%s %s%s%s\n", utils.Gray, "Fingerprint:", utils.Reset, utils.Magenta, w.cfg.Proxy.TLS.Fingerprint, utils.Reset)
	} else if w.cfg.Proxy.Method == "reality" && w.cfg.Proxy.Reality != nil {
		fmt.Printf("    %s%-16s%s %s%s%s\n", utils.Gray, "Server Name:", utils.Reset, utils.Cyan, w.cfg.Proxy.Reality.ServerName, utils.Reset)
		fmt.Printf("    %s%-16s%s %s%s%s\n", utils.Gray, "Fingerprint:", utils.Reset, utils.Magenta, w.cfg.Proxy.Reality.Fingerprint, utils.Reset)
	}
	fmt.Printf("    %s%-16s%s %s%s%s\n", utils.Gray, "Network:", utils.Reset, utils.Green, w.cfg.Proxy.Type, utils.Reset)
	fmt.Printf("    %s%-16s%s %s%d%s\n", utils.Gray, "Remote Port:", utils.Reset, utils.Yellow, w.cfg.Proxy.Port, utils.Reset)

	fmt.Println()
}

func (w *Worker) probeTCP(ip string, port int, timeout time.Duration) (time.Duration, error) {
	if w.tcpConnect != nil {
		return w.tcpConnect(ip, port, timeout)
	}
	return utils.TCPConnectOnInterface(w.ctx, w.cfg.Network.Interface, ip, port, timeout)
}
