package xray

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// MeasureOptions controls what TestMeasure measures for a single IP.
type MeasureOptions struct {
	TestURL       string        // 204/lightweight URL used for ping + loss (e.g. gstatic generate_204)
	Timeout       time.Duration // overall per-request timeout (handshake)
	Samples       int           // number of measured requests for ping + loss
	Realistic     bool          // warm the tunnel first, then report MIN of samples (matches v2rayN)
	LossTest      bool          // measure packet loss via fresh concurrent probes
	LossSamples   int           // number of fresh-connection probes for loss
	HandshakeTest bool          // require + time a full cold TLS handshake through the tunnel
	MaxLatency    int           // ms; a sample slower than this counts as loss (0 = use Timeout)

	UploadTest  bool
	UploadURL   string
	UploadBytes int

	DownloadTest  bool
	DownloadURL   string
	DownloadBytes int
}

// MeasureResult holds the rich per-IP measurement.
type MeasureResult struct {
	IP             string
	Connected      bool
	Success        bool
	StatusCode     int
	MinLatency     time.Duration // realistic ping (min of warm samples)
	AvgLatency     time.Duration
	Jitter         time.Duration
	Handshake      time.Duration // cold connection-establishment latency
	HandshakeOK    bool
	LossPercent    float64
	Samples        int
	LossSamples    int
	UploadMbps     float64
	UploadTested   bool
	UploadErr      string
	DownloadMbps   float64
	DownloadTested bool
	DownloadErr    string
	Error          error
}

// newProxyClient builds an http.Client that routes through the local SOCKS5
// proxy with keep-alive ENABLED so repeated requests to the same host reuse the
// already-established tunnel (this is what makes the warm ping realistic).
//
// It deliberately has NO global Client.Timeout — every request carries its own
// context deadline, so there is a single, unambiguous timeout mechanism.
func newProxyClient(socksPort int, serverName string) (*http.Client, error) {
	socksAddr := fmt.Sprintf("127.0.0.1:%d", socksPort)
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// dialer.Dial ignores ctx entirely; go through DialContext when the
			// SOCKS5 dialer supports it (it does) so a canceled/expired sample
			// deadline actually aborts the dial instead of blocking until the OS
			// TCP timeout while the caller has already moved on.
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				return cd.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		},
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         serverName,
		},
		// Keep-alive ON: reuse the warm connection between samples.
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   false, // force HTTP/1.1 so we keep one reusable conn
		MaxIdleConns:        4,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     30 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

// doPing issues a single GET through the proxy client and returns its latency.
func doPing(ctx context.Context, client *http.Client, testURL string) (time.Duration, int, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", "piyazche/scanner")
	resp, err := client.Do(req)
	if err != nil {
		// Some errors still return a non-nil response; drain + close it so the
		// transport can recycle the connection rather than tearing it down.
		if resp != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		return 0, 0, err
	}
	defer resp.Body.Close()
	// Fully drain the (tiny 204) body so the keep-alive connection returns to
	// the pool and the next sample reuses the warm tunnel — the whole point of
	// the realistic-ping measurement.
	io.Copy(io.Discard, resp.Body)
	latency := time.Since(start)
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return latency, resp.StatusCode, fmt.Errorf("bad status %d", resp.StatusCode)
	}
	return latency, resp.StatusCode, nil
}

// TestMeasure runs the realistic ping + packet-loss + (optional) upload/download
// measurement for one IP, reusing a single warm tunnel where possible.
func TestMeasure(ctx context.Context, socksPort int, opts MeasureOptions) *MeasureResult {
	res := &MeasureResult{}

	parsedURL, err := url.Parse(opts.TestURL)
	if err != nil {
		res.Error = fmt.Errorf("invalid test URL: %w", err)
		return res
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// Per-sample deadline: a sample slower than this is counted as packet loss so
	// laggy IPs are not reported as good. Defaults to MaxLatency, capped to the
	// overall timeout.
	deadline := timeout
	if opts.MaxLatency > 0 {
		deadline = time.Duration(opts.MaxLatency) * time.Millisecond
	}
	if deadline > timeout {
		deadline = timeout
	}
	if deadline < 300*time.Millisecond {
		deadline = 300 * time.Millisecond
	}

	client, err := newProxyClient(socksPort, parsedURL.Hostname())
	if err != nil {
		res.Error = err
		return res
	}
	defer client.CloseIdleConnections()

	// Warmup / handshake: establish the tunnel + TLS once. Its latency is the
	// cold handshake time and is excluded from the steady-state ping.
	if opts.Realistic || opts.HandshakeTest {
		select {
		case <-ctx.Done():
			res.Error = ctx.Err()
			return res
		default:
		}
		hsCtx, cancel := context.WithTimeout(ctx, timeout)
		hs, code, werr := doPing(hsCtx, client, opts.TestURL)
		cancel()
		if werr != nil {
			// Handshake failed -> the IP is effectively dead for this config.
			res.StatusCode = code
			res.Error = fmt.Errorf("handshake failed: %w", werr)
			res.LossPercent = 100
			res.HandshakeOK = false
			return res
		}
		res.Handshake = hs
		res.HandshakeOK = true
	}

	// Warm ping samples (reuse the established connection) -> realistic min/avg/
	// jitter, matching v2rayN's "real delay". A sample slower than `deadline` is
	// dropped from the stats (it's covered by the loss probes below).
	var latencies []time.Duration
	pingSamples := opts.Samples
	if pingSamples <= 0 {
		pingSamples = 4
	}
	lastStatus := res.StatusCode
	for i := 0; i < pingSamples; i++ {
		if ctx.Err() != nil {
			res.Error = ctx.Err()
			break
		}
		sCtx, cancel := context.WithTimeout(ctx, deadline)
		lat, code, perr := doPing(sCtx, client, opts.TestURL)
		cancel()
		if code != 0 {
			lastStatus = code
		}
		if perr != nil {
			if ctx.Err() != nil {
				res.Error = ctx.Err()
				break
			}
			continue
		}
		if lat <= deadline {
			latencies = append(latencies, lat)
		}
	}
	res.Samples = pingSamples
	res.StatusCode = lastStatus

	if len(latencies) > 0 {
		min, sum := latencies[0], time.Duration(0)
		for _, l := range latencies {
			if l < min {
				min = l
			}
			sum += l
		}
		res.MinLatency = min
		res.AvgLatency = sum / time.Duration(len(latencies))
		var jitterSum time.Duration
		for i := 1; i < len(latencies); i++ {
			d := latencies[i] - latencies[i-1]
			if d < 0 {
				d = -d
			}
			jitterSum += d
		}
		if len(latencies) > 1 {
			res.Jitter = jitterSum / time.Duration(len(latencies)-1)
		}
	} else if res.Handshake > 0 {
		// No warm sample landed within the deadline; fall back to the handshake.
		res.MinLatency = res.Handshake
		res.AvgLatency = res.Handshake
	} else {
		// Nothing measured at all (no handshake, no warm sample) -> not usable.
		res.Success = false
		if res.Error == nil {
			res.Error = fmt.Errorf("no latency samples")
		}
		res.LossPercent = 100
		return res
	}
	res.Success = true
	res.Connected = true

	// Packet loss: fresh, CONCURRENT probes. Each one re-dials through the tunnel
	// to this edge, so the loss % reflects how reliably NEW connections succeed —
	// a flaky edge drops/times-out a fraction of them. Reusing one warm
	// connection (the ping path) almost never drops, which is why loss looked
	// like ~0 before.
	if opts.LossTest && ctx.Err() == nil {
		n := opts.LossSamples
		if n <= 0 {
			n = 6
		}
		// Each loss probe opens a brand-new connection (full TLS/fragment
		// handshake), not a reused warm one — that structurally takes longer than
		// the warm-ping `deadline`. Budgeting loss probes to `deadline` made
		// virtually every fresh connection register as "lost" even on IPs that
		// just proved themselves reachable via HandshakeTest above (which
		// correctly uses the fuller `timeout` budget). Reuse that same budget here.
		res.LossPercent = measureLoss(ctx, socksPort, opts.TestURL, n, timeout)
		res.LossSamples = n
	}

	// Optional throughput tests reuse the same SOCKS proxy (new host = new stream).
	// Bounded so an IP with no upload fails fast instead of stalling the scan.
	speedTimeout := timeout
	if speedTimeout < 10*time.Second {
		speedTimeout = 10 * time.Second
	}
	if speedTimeout > 15*time.Second {
		speedTimeout = 15 * time.Second
	}

	if opts.UploadTest && opts.UploadURL != "" && ctx.Err() == nil {
		res.UploadTested = true // recorded even on failure so 0 means "no upload", not "skipped"
		if mbps, uerr := measureUpload(ctx, socksPort, opts.UploadURL, opts.UploadBytes, speedTimeout); uerr == nil {
			res.UploadMbps = mbps
		} else {
			res.UploadErr = uerr.Error()
		}
	}
	if opts.DownloadTest && opts.DownloadURL != "" && ctx.Err() == nil {
		res.DownloadTested = true
		if mbps, derr := measureDownload(ctx, socksPort, opts.DownloadURL, opts.DownloadBytes, speedTimeout); derr == nil {
			res.DownloadMbps = mbps
		} else {
			res.DownloadErr = derr.Error()
		}
	}

	return res
}

// measureLoss fires n fresh-connection probes concurrently and returns the loss
// percentage (failed or too-slow probes / n). Each probe uses its own client so
// it re-dials through the tunnel, exposing edge flakiness that a reused warm
// connection hides.
func measureLoss(ctx context.Context, socksPort int, testURL string, n int, deadline time.Duration) float64 {
	if n <= 0 {
		n = 1
	}
	host := ""
	if u, err := url.Parse(testURL); err == nil {
		host = u.Hostname()
	}

	type probe struct {
		lat time.Duration
		err error
	}
	results := make(chan probe, n)
	for i := 0; i < n; i++ {
		go func() {
			client, err := newProxyClient(socksPort, host)
			if err != nil {
				results <- probe{err: err}
				return
			}
			defer client.CloseIdleConnections()
			pCtx, cancel := context.WithTimeout(ctx, deadline)
			defer cancel()
			lat, _, perr := doPing(pCtx, client, testURL)
			results <- probe{lat: lat, err: perr}
		}()
	}

	failed := 0
	for i := 0; i < n; i++ {
		select {
		case p := <-results:
			if p.err != nil || p.lat > deadline {
				failed++
			}
		case <-ctx.Done():
			// Scan aborted. The caller discards this result. The channel is
			// buffered to n, so the still-running probes can send and exit
			// without leaking — we just stop waiting for them.
			return 100
		}
	}
	return float64(failed) / float64(n) * 100
}

// measureUpload POSTs a fixed-size body through the tunnel and returns Mbps.
func measureUpload(ctx context.Context, socksPort int, uploadURL string, nbytes int, timeout time.Duration) (float64, error) {
	if nbytes <= 0 {
		nbytes = 1000000
	}
	parsed, err := url.Parse(uploadURL)
	if err != nil {
		return 0, err
	}
	client, err := newProxyClient(socksPort, parsed.Hostname())
	if err != nil {
		return 0, err
	}
	defer client.CloseIdleConnections()

	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload := bytes.Repeat([]byte{0}, nbytes)

	start := time.Now()
	req, err := http.NewRequestWithContext(opCtx, "POST", uploadURL, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(nbytes)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start).Seconds()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("upload bad status %d", resp.StatusCode)
	}
	// Implausibly fast => the body wasn't actually transferred (e.g. instant
	// reject); don't report a garbage multi-Gbps number.
	if elapsed < 0.0005 {
		return 0, fmt.Errorf("transfer too fast to measure")
	}
	return float64(nbytes) * 8 / elapsed / 1e6, nil
}

// measureDownload GETs a fixed-size body through the tunnel and returns Mbps.
func measureDownload(ctx context.Context, socksPort int, downloadURL string, nbytes int, timeout time.Duration) (float64, error) {
	if nbytes <= 0 {
		nbytes = 2000000
	}
	// Cloudflare's __down endpoint takes a ?bytes=N query parameter.
	full := downloadURL
	if u, err := url.Parse(downloadURL); err == nil {
		q := u.Query()
		if q.Get("bytes") == "" {
			q.Set("bytes", fmt.Sprintf("%d", nbytes))
			u.RawQuery = q.Encode()
			full = u.String()
		}
	}
	parsed, _ := url.Parse(full)
	host := ""
	if parsed != nil {
		host = parsed.Hostname()
	}
	client, err := newProxyClient(socksPort, host)
	if err != nil {
		return 0, err
	}
	defer client.CloseIdleConnections()

	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(opCtx, "GET", full, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("download bad status %d", resp.StatusCode)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}
	elapsed := time.Since(start).Seconds()
	if n <= 0 {
		return 0, fmt.Errorf("zero transfer")
	}
	if elapsed < 0.0005 {
		return 0, fmt.Errorf("transfer too fast to measure")
	}
	return float64(n) * 8 / elapsed / 1e6, nil
}
