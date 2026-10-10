package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"piyazche/utils"

	// Core xray imports
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"

	// Required handlers - register in init functions
	_ "github.com/xtls/xray-core/app/dispatcher"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"

	// Optional features
	_ "github.com/xtls/xray-core/app/dns"
	_ "github.com/xtls/xray-core/app/dns/fakedns"
	_ "github.com/xtls/xray-core/app/metrics"
	_ "github.com/xtls/xray-core/app/policy"
	_ "github.com/xtls/xray-core/app/router"
	_ "github.com/xtls/xray-core/app/stats"

	// Fix dependency cycle
	_ "github.com/xtls/xray-core/transport/internet/tagged/taggedimpl"

	// Inbound and outbound proxies
	_ "github.com/xtls/xray-core/proxy/blackhole"
	_ "github.com/xtls/xray-core/proxy/dns"
	_ "github.com/xtls/xray-core/proxy/dokodemo"
	_ "github.com/xtls/xray-core/proxy/freedom"
	_ "github.com/xtls/xray-core/proxy/http"
	_ "github.com/xtls/xray-core/proxy/loopback"
	_ "github.com/xtls/xray-core/proxy/socks"
	_ "github.com/xtls/xray-core/proxy/vless/inbound"
	_ "github.com/xtls/xray-core/proxy/vless/outbound"
	_ "github.com/xtls/xray-core/proxy/vmess/inbound"
	_ "github.com/xtls/xray-core/proxy/vmess/outbound"

	// Transports
	_ "github.com/xtls/xray-core/transport/internet/grpc"
	_ "github.com/xtls/xray-core/transport/internet/http"
	_ "github.com/xtls/xray-core/transport/internet/httpupgrade"
	_ "github.com/xtls/xray-core/transport/internet/reality"
	_ "github.com/xtls/xray-core/transport/internet/splithttp"
	_ "github.com/xtls/xray-core/transport/internet/tcp"
	_ "github.com/xtls/xray-core/transport/internet/tls"
	_ "github.com/xtls/xray-core/transport/internet/udp"
	_ "github.com/xtls/xray-core/transport/internet/websocket"

	// Transport headers
	_ "github.com/xtls/xray-core/transport/internet/headers/http"
	_ "github.com/xtls/xray-core/transport/internet/headers/tls"

	// JSON config loader
	_ "github.com/xtls/xray-core/main/json"
)

// Manager handles xray instance lifecycle using embedded xray-core
type Manager struct {
	instance  *core.Instance
	socksPort int
	mu        sync.Mutex
	running   bool
	debug     bool
}

// NewManager creates a new xray manager
func NewManager() *Manager {
	return &Manager{
		debug: false,
	}
}

// NewManagerWithDebug creates a new xray manager with debug logging
func NewManagerWithDebug(debug bool) *Manager {
	return &Manager{
		debug: debug,
	}
}

// Start starts the xray instance with the given configuration
func (m *Manager) Start(configData []byte, socksPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("xray is already running")
	}

	if m.debug {
		var prettyJSON map[string]interface{}
		if err := json.Unmarshal(configData, &prettyJSON); err == nil {
			formatted, _ := json.MarshalIndent(prettyJSON, "", "  ")
			fmt.Printf("\n%s%sXray Config%s %s(port %d)%s\n", utils.Bold, utils.Cyan, utils.Reset, utils.Gray, socksPort, utils.Reset)
			fmt.Printf("%s────────────────────────────────────────%s\n", utils.Gray, utils.Reset)
			fmt.Printf("%s%s%s\n", utils.Dim, string(formatted), utils.Reset)
			fmt.Printf("%s────────────────────────────────────────%s\n\n", utils.Gray, utils.Reset)
			// Save first config to file
			if err := os.WriteFile("first_config.jsonc", formatted, 0644); err == nil {
				fmt.Printf("%sConfig saved to:%s %sfirst_config.jsonc%s\n\n", utils.Gray, utils.Reset, utils.Cyan, utils.Reset)
			}
		}
	}

	m.socksPort = socksPort

	// Load JSON config using xray-core's serial loader
	jsonConfig, err := serial.DecodeJSONConfig(bytes.NewReader(configData))
	if err != nil {
		return fmt.Errorf("failed to decode JSON config: %w", err)
	}

	// Build the protobuf config
	pbConfig, err := jsonConfig.Build()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	// Create xray instance
	instance, err := core.New(pbConfig)
	if err != nil {
		return fmt.Errorf("failed to create xray instance: %w", err)
	}

	// Start the instance
	if err := instance.Start(); err != nil {
		instance.Close()
		return fmt.Errorf("failed to start xray instance: %w", err)
	}

	m.instance = instance
	m.running = true

	return nil
}

// Stop stops the xray instance
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	var closeErr error
	if m.instance != nil {
		closeErr = m.instance.Close()
		m.instance = nil
	}
	m.running = false

	if closeErr != nil {
		return fmt.Errorf("failed to close xray instance: %w", closeErr)
	}
	return nil
}

// IsRunning returns whether xray is currently running
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// GetSocksPort returns the SOCKS proxy port
func (m *Manager) GetSocksPort() int {
	return m.socksPort
}

// WaitForReady waits for xray to be ready to accept connections
func (m *Manager) WaitForReady(timeout time.Duration) error {
	return m.WaitForReadyWithContext(nil, timeout)
}

// WaitForReadyWithContext waits for xray to be ready with context support for cancellation
func (m *Manager) WaitForReadyWithContext(ctx interface{ Done() <-chan struct{} }, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return fmt.Errorf("cancelled")
			default:
			}
		}

		if !m.IsRunning() {
			return fmt.Errorf("xray instance terminated unexpectedly")
		}

		// Poll until the SOCKS proxy port accepts connections
		if IsPortOpen("127.0.0.1", m.socksPort) {
			// Brief delay for xray to complete initialization after binding
			time.Sleep(30 * time.Millisecond)
			return nil
		}

		time.Sleep(15 * time.Millisecond)
	}

	return fmt.Errorf("xray did not become ready within %v", timeout)
}
