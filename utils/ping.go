package utils

import (
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// PingResult holds the result of an ICMP ping
type PingResult struct {
	Success bool
	Latency time.Duration
	Error   error
}

// PingMode indicates how pings are being performed
type PingMode int

const (
	PingModeICMP PingMode = iota // Raw ICMP (requires root)
	PingModeTCP                  // TCP connect fallback (no root needed)
)

// Global state
var (
	pingSeq     atomic.Uint32
	pingMode    PingMode
	pingModeSet sync.Once
)

// detectPingMode checks if we can use raw ICMP sockets
func detectPingMode() PingMode {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err == nil {
		conn.Close()
		return PingModeICMP
	}
	return PingModeTCP
}

// GetPingMode returns the current ping mode (for display purposes)
func GetPingMode() PingMode {
	pingModeSet.Do(func() {
		pingMode = detectPingMode()
	})
	return pingMode
}

// PingModeString returns a human-readable ping mode
func PingModeString() string {
	switch GetPingMode() {
	case PingModeICMP:
		return "ICMP"
	case PingModeTCP:
		return "TCP"
	default:
		return "unknown"
	}
}

// Ping sends an ICMP echo request or TCP connect to the target IP
func Ping(targetIP string, timeout time.Duration) PingResult {
	if timeout > 2*time.Second {
		timeout = 2 * time.Second
	}

	mode := GetPingMode()
	if mode == PingModeTCP {
		return tcpPing(targetIP, timeout)
	}
	return icmpPing(targetIP, timeout)
}

// tcpPing uses TCP connect to check reachability (works without root)
func tcpPing(targetIP string, timeout time.Duration) PingResult {
	// Try common ports: 443, 80
	ports := []int{443, 80}

	for _, port := range ports {
		addr := net.JoinHostPort(targetIP, fmt.Sprintf("%d", port))
		start := time.Now()

		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err == nil {
			latency := time.Since(start)
			conn.Close()
			return PingResult{Success: true, Latency: latency}
		}
	}

	return PingResult{Success: false, Error: fmt.Errorf("tcp connect failed")}
}

// icmpPing sends a real ICMP echo request (requires root)
func icmpPing(targetIP string, timeout time.Duration) PingResult {
	dst, err := net.ResolveIPAddr("ip4", targetIP)
	if err != nil {
		return PingResult{Success: false, Error: err}
	}

	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return PingResult{Success: false, Error: err}
	}
	defer conn.Close()

	// Generate unique ID for this ping
	seq := pingSeq.Add(1)
	echoID := int(os.Getpid() & 0xffff)
	echoSeq := int(seq & 0xffff)

	// Standard 56 bytes of ping data (like traditional ping)
	// First 8 bytes are timestamp (Unix nanos), rest is pattern
	data := make([]byte, 56)
	ts := time.Now().UnixNano()
	data[0] = byte(ts >> 56)
	data[1] = byte(ts >> 48)
	data[2] = byte(ts >> 40)
	data[3] = byte(ts >> 32)
	data[4] = byte(ts >> 24)
	data[5] = byte(ts >> 16)
	data[6] = byte(ts >> 8)
	data[7] = byte(ts)
	for i := 8; i < len(data); i++ {
		data[i] = byte(i)
	}

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   echoID,
			Seq:  echoSeq,
			Data: data,
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return PingResult{Success: false, Error: err}
	}

	conn.SetDeadline(time.Now().Add(timeout))
	start := time.Now()

	if _, err := conn.WriteTo(msgBytes, dst); err != nil {
		return PingResult{Success: false, Error: err}
	}

	reply := make([]byte, 512)
	for {
		n, peer, err := conn.ReadFrom(reply)
		if err != nil {
			return PingResult{Success: false, Error: err}
		}

		peerIP, ok := peer.(*net.IPAddr)
		if !ok || !peerIP.IP.Equal(dst.IP) {
			continue
		}

		latency := time.Since(start)

		parsed, err := icmp.ParseMessage(1, reply[:n])
		if err != nil {
			continue
		}

		if parsed.Type == ipv4.ICMPTypeEchoReply {
			if echo, ok := parsed.Body.(*icmp.Echo); ok {
				if echo.ID == echoID && echo.Seq == echoSeq {
					return PingResult{Success: true, Latency: latency}
				}
			}
		}
	}
}

// PingWithRetries pings with multiple retry attempts
func PingWithRetries(targetIP string, timeout time.Duration, retries int) PingResult {
	var lastResult PingResult

	for i := 0; i < retries; i++ {
		lastResult = Ping(targetIP, timeout)
		if lastResult.Success {
			return lastResult
		}
	}

	return lastResult
}
