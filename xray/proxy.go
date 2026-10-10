package xray

import (
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

// TestResult contains the result of a connectivity test
type TestResult struct {
	IP         string
	Success    bool
	Latency    time.Duration
	Error      error
	StatusCode int
	BytesRead  int64
}

// TestConnectivity tests connectivity through the SOCKS proxy
func TestConnectivity(socksPort int, testURL string, timeout time.Duration) *TestResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return TestConnectivityWithContext(ctx, socksPort, testURL, timeout)
}

// TestConnectivityWithContext tests connectivity with context support for cancellation
func TestConnectivityWithContext(ctx context.Context, socksPort int, testURL string, timeout time.Duration) *TestResult {
	result := &TestResult{}

	parsedURL, err := url.Parse(testURL)
	if err != nil {
		result.Error = fmt.Errorf("invalid test URL: %w", err)
		return result
	}

	socksAddr := fmt.Sprintf("127.0.0.1:%d", socksPort)
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		result.Error = fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		return result
	}

	// Route HTTP traffic through the xray SOCKS proxy
	transport := &http.Transport{
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				return cd.DialContext(dialCtx, network, addr)
			}
			return dialer.Dial(network, addr)
		},
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         parsedURL.Hostname(),
		},
		DisableKeepAlives: true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		result.Error = fmt.Errorf("failed to create request: %w", err)
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			result.Error = ctx.Err()
		} else {
			result.Error = fmt.Errorf("request failed: %w", err)
		}
		return result
	}
	defer resp.Body.Close()

	// Read the body so we measure the full request time
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024))
	if err != nil {
		if ctx.Err() != nil {
			result.Error = ctx.Err()
		} else {
			result.Error = fmt.Errorf("failed to read response: %w", err)
		}
		return result
	}

	result.Latency = time.Since(start)
	result.StatusCode = resp.StatusCode
	result.BytesRead = int64(len(body))
	result.Success = resp.StatusCode >= 200 && resp.StatusCode < 400

	return result
}

// TestSpeed performs a speed test by downloading a file
func TestSpeed(socksPort int, testURL string, timeout time.Duration) (bytesPerSecond float64, err error) {
	socksAddr := fmt.Sprintf("127.0.0.1:%d", socksPort)
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return 0, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				return cd.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		},
		DisableKeepAlives: true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	start := time.Now()
	resp, err := client.Get(testURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}

	duration := time.Since(start).Seconds()
	if duration > 0 {
		bytesPerSecond = float64(n) / duration
	}

	return bytesPerSecond, nil
}

// IsPortOpen checks if a port is open on the given host
func IsPortOpen(host string, port int) bool {
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
