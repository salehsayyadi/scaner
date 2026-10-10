package appcore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"piyazche/applog"
	"piyazche/config"
	"piyazche/optimizer"
	"piyazche/scanner"
	"piyazche/utils"
	"piyazche/xray"
)

const (
	ModeXray         = "Proxy scan"
	ModeConnectivity = "Connectivity (ICMP / TCP)"
	ModeCheck        = "Check connection"
)

// Run owns an immutable configuration snapshot. Editing the UI while a scan or
// an independent optimizer is running therefore cannot race with the workers.
type Run struct {
	Config    *config.Config
	Mode      string
	Debug     bool
	IPs       []string
	Results   *scanner.ResultCollector
	Optimized *optimizer.ZoneResult // read only after Execute returns
	ctx       context.Context
	cancel    context.CancelFunc
	paused    atomic.Bool
	completed atomic.Int64
	sc        *scanner.Scanner
}

func NewRun(parent context.Context, cfg *config.Config, ranges, mode string, debug bool) (*Run, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is missing")
	}
	if mode != ModeXray && mode != ModeConnectivity && mode != ModeCheck {
		return nil, fmt.Errorf("unknown scan mode: %s", mode)
	}
	c := cfg.Clone()
	if mode != ModeConnectivity {
		if err := c.Validate(); err != nil {
			return nil, err
		}
	}
	if mode == ModeXray && c.Proxy.Method == "reality" {
		return nil, fmt.Errorf("Reality supports Check connection and fragment optimization; choose Check connection")
	}
	if c.Scan.Threads <= 0 {
		c.Scan.Threads = 1
	}
	if c.Scan.Timeout <= 0 {
		c.Scan.Timeout = 10
	}
	var ips []string
	var err error
	if mode == ModeCheck {
		if c.Proxy.Address == "" {
			return nil, fmt.Errorf("set the server address before checking a connection")
		}
		ips = []string{c.Proxy.Address}
	} else {
		ips, err = utils.ParseIPsFromText(ranges, c.Scan.SampleSize, c.Scan.Shuffle)
		if err != nil {
			return nil, err
		}
		if c.Scan.MaxIPs > 0 && len(ips) > c.Scan.MaxIPs {
			ips = ips[:c.Scan.MaxIPs]
		}
	}
	ctx, cancel := context.WithCancel(parent)
	r := &Run{Config: c, Mode: mode, IPs: ips, Debug: debug, ctx: ctx, cancel: cancel}
	if mode == ModeXray {
		r.sc = scanner.NewScannerWithDebug(c, debug)
		r.sc.SetSilent(true)
		r.sc.SetIPs(ips)
		r.Results = r.sc.GetResults()
		r.sc.SetOnProgress(func(done, _ int) { r.completed.Store(int64(done)) })
	} else {
		r.Results = scanner.NewResultCollector()
	}
	r.Results.SetOnAdd(func(v scanner.Result) {
		applog.Logf("IP=%s success=%v ping=%dms TCP=%dms handshake=%dms jitter=%dms loss=%.1f%% upload=%.2fMbps download=%.2fMbps error=%s",
			v.IP, v.Success, v.LatencyMs, v.TCPMs, v.HandshakeMs, v.JitterMs, v.LossPercent, v.UploadMbps, v.DownloadMbps, v.Error)
	})
	return r, nil
}

func (r *Run) Stop()                { r.cancel() }
func (r *Run) Cancelled() bool      { return r.ctx.Err() != nil }
func (r *Run) Progress() (int, int) { return int(r.completed.Load()), len(r.IPs) }
func (r *Run) Paused() bool         { return r.paused.Load() }
func (r *Run) TogglePause() bool {
	p := !r.paused.Load()
	r.paused.Store(p)
	if r.sc != nil {
		r.sc.SetPaused(p)
	}
	return p
}

func (r *Run) Execute() error {
	defer r.cancel()
	if err := r.ctx.Err(); err != nil {
		return err
	}
	applog.Logf("start mode=%s IPs=%d workers=%d", r.Mode, len(r.IPs), r.Config.Scan.Threads)
	if r.Mode != ModeConnectivity && r.Config.Fragment.Enabled && r.Config.Fragment.Mode == "auto" {
		target := r.Config.Fragment.Auto.TestIP
		if r.Config.Proxy.Method == "reality" || target == "" {
			target = r.IPs[0]
		}
		best, err := Optimize(r.ctx, r.Config, target, r.Debug)
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		if err != nil {
			// Same fallback as the CLI; log it rather than silently losing the option.
			applog.Logf("optimizer failed; using manual defaults: %v", err)
			r.Config.Fragment.Mode = "manual"
		} else {
			r.Optimized = best
			ApplyOptimization(r.Config, best)
		}
	}
	switch r.Mode {
	case ModeXray:
		return r.sc.RunWithContext(r.ctx)
	case ModeCheck:
		return r.check()
	default:
		return r.connectivity()
	}
}

func (r *Run) check() error {
	port := utils.AcquirePort()
	defer utils.ReleasePort(port)
	b, err := config.GenerateXrayConfig(r.Config, r.IPs[0], port)
	if err != nil {
		return err
	}
	m := xray.NewManagerWithDebug(r.Debug)
	if err := m.Start(b, port); err != nil {
		return err
	}
	defer m.Stop()
	if err := m.WaitForReadyWithContext(r.ctx, 10*time.Second); err != nil {
		return err
	}
	v := xray.TestConnectivityWithContext(r.ctx, port, r.Config.Scan.TestURL, r.Config.GetTimeout())
	result := scanner.Result{IP: r.IPs[0], Success: v.Success, Latency: v.Latency}
	if v.Error != nil {
		result.Error = v.Error.Error()
	}
	if result.Success && r.Config.Scan.MaxLatency > 0 && result.Latency.Milliseconds() > int64(r.Config.Scan.MaxLatency) {
		result.Success = false
		result.Error = "latency exceeds the configured maximum"
	}
	r.Results.Add(result)
	r.completed.Store(1)
	return r.ctx.Err()
}

// Connectivity uses the existing ICMP/TCP implementation. The fallback is
// explicitly identified in the UI; TCP timing is never presented as raw ICMP.
func (r *Run) connectivity() error {
	jobs := make(chan string)
	var wg sync.WaitGroup
	threads := r.Config.Scan.Threads
	if threads > len(r.IPs) {
		threads = len(r.IPs)
	}
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				for r.paused.Load() && r.ctx.Err() == nil {
					select {
					case <-r.ctx.Done():
						return
					case <-time.After(75 * time.Millisecond):
					}
				}
				if r.ctx.Err() != nil {
					return
				}
				tries := r.Config.Scan.Retries
				if tries <= 0 {
					tries = 3
				}
				var v utils.PingResult
				for n := 0; n < tries && r.ctx.Err() == nil; n++ {
					v = utils.Ping(ip, r.Config.GetTimeout())
					if v.Success {
						break
					}
				}
				if r.ctx.Err() != nil {
					return
				}
				result := scanner.Result{IP: ip, Success: v.Success, Latency: v.Latency}
				if v.Error != nil {
					result.Error = v.Error.Error()
				}
				if result.Success && r.Config.Scan.MaxLatency > 0 && v.Latency.Milliseconds() > int64(r.Config.Scan.MaxLatency) {
					result.Success = false
					result.Error = "latency exceeds the configured maximum"
				}
				if utils.GetPingMode() == utils.PingModeTCP {
					result.TCPOK, result.TCPMs = v.Success, v.Latency.Milliseconds()
				}
				r.Results.Add(result)
				r.completed.Add(1)
			}
		}()
	}
feed:
	for _, ip := range r.IPs {
		select {
		case <-r.ctx.Done():
			break feed
		case jobs <- ip:
		}
	}
	close(jobs)
	wg.Wait()
	return r.ctx.Err()
}

func Optimize(ctx context.Context, cfg *config.Config, target string, debug bool) (*optimizer.ZoneResult, error) {
	c := cfg.Clone()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if target == "" {
		return nil, fmt.Errorf("set an optimization test IP or a server address")
	}
	tester := optimizer.NewFragmentTester(c, target).WithContext(ctx).WithDebug(debug)
	fc := optimizer.DefaultFinderConfig()
	if c.Fragment.Auto.MaxTests > 0 {
		fc.MaxTriesPerZone = c.Fragment.Auto.MaxTests
	}
	if c.Fragment.Auto.SuccessThreshold > 0 {
		fc.SuccessThreshold = c.Fragment.Auto.SuccessThreshold
	}
	size := optimizer.Range{Min: c.Fragment.Auto.LengthRange.Min, Max: c.Fragment.Auto.LengthRange.Max}
	interval := optimizer.Range{Min: c.Fragment.Auto.IntervalRange.Min, Max: c.Fragment.Auto.IntervalRange.Max}
	if !size.IsValid() || !interval.IsValid() {
		return nil, fmt.Errorf("fragment min/max ranges must be positive and increasing")
	}
	finder := optimizer.NewFinder(fc, tester.CreateTesterFunc())
	finder.SetProgressCallback(func(zone string, attempt, total int, ok bool, size, interval optimizer.Range, latency time.Duration) {
		applog.Logf("optimize zone=%s attempt=%d/%d success=%v length=%s interval=%s ping=%dms", zone, attempt, total, ok, size, interval, latency.Milliseconds())
	})
	var results []optimizer.ZoneResult
	for _, zone := range []string{"tlshello", "1-3", "1-5"} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		results = append(results, finder.FindOne(optimizer.Zone{Name: zone, SizeRange: size, IntervalRange: interval}))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	best := optimizer.GetBestResult(results)
	if best == nil {
		return nil, fmt.Errorf("no working fragment settings found")
	}
	return best, nil
}

func ApplyOptimization(c *config.Config, best *optimizer.ZoneResult) {
	if best == nil {
		return
	}
	c.Fragment.Enabled = true
	c.Fragment.Mode = "manual"
	c.Fragment.Packets = best.Zone
	c.Fragment.Manual.Length = best.SizeRange.String()
	c.Fragment.Manual.Interval = best.IntervalRange.String()
}
