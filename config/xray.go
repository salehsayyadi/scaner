package config

import (
	"encoding/json"
	"fmt"
	"piyazche/netroute"
)

// FragmentSettings represents fragment settings for xray config
type FragmentSettings struct {
	Packets  string
	Length   string
	Interval string
}

// GenerateXrayConfig creates an xray configuration for testing a specific IP
// matching the sample_config.json structure exactly
func GenerateXrayConfig(cfg *Config, targetIP string, socksPort int) ([]byte, error) {
	fragment := FragmentSettings{
		Packets:  cfg.Fragment.Packets,
		Length:   cfg.Fragment.Manual.Length,
		Interval: cfg.Fragment.Manual.Interval,
	}
	return GenerateXrayConfigWithFragment(cfg, targetIP, socksPort, fragment)
}

// GenerateXrayConfigWithFragment creates an xray configuration with specific fragment settings
func GenerateXrayConfigWithFragment(cfg *Config, targetIP string, socksPort int, fragment FragmentSettings) ([]byte, error) {
	if err := netroute.Validate(cfg.Network.Interface); err != nil {
		return nil, err
	}
	if cfg.Network.Interface != "" && cfg.Proxy.TLS != nil {
		for _, alpn := range cfg.Proxy.TLS.ALPN {
			if alpn == "h3" {
				return nil, fmt.Errorf("manual network selection currently requires TCP/HTTP2 transports; h3/QUIC is not supported")
			}
		}
	}
	uuid := cfg.Proxy.UUID
	remarkID := uuid
	if len(uuid) > 8 {
		remarkID = uuid[:8]
	}

	xrayConfig := map[string]interface{}{
		"dns":       buildDNS(),
		"inbounds":  buildInbounds(socksPort),
		"log":       buildLog(cfg),
		"outbounds": buildOutboundsWithFragment(cfg, targetIP, fragment),
		"remarks":   fmt.Sprintf("%s-%d-%s", cfg.Proxy.Type, cfg.Proxy.Port, remarkID),
		"routing":   buildRouting(),
	}

	return json.MarshalIndent(xrayConfig, "", "    ")
}

func buildDNS() map[string]interface{} {
	return map[string]interface{}{
		"hosts": map[string]interface{}{
			"dns.alidns.com": []string{
				"223.5.5.5",
				"223.6.6.6",
				"2400:3200::1",
				"2400:3200:baba::1",
			},
			"one.one.one.one": []string{
				"1.1.1.1",
				"1.0.0.1",
				"2606:4700:4700::1111",
				"2606:4700:4700::1001",
			},
			"dot.pub": []string{
				"1.12.12.12",
				"120.53.53.53",
			},
			"dns.google": []string{
				"8.8.8.8",
				"8.8.4.4",
				"2001:4860:4860::8888",
				"2001:4860:4860::8844",
			},
			"dns.quad9.net": []string{
				"9.9.9.9",
				"149.112.112.112",
				"2620:fe::fe",
				"2620:fe::9",
			},
		},
		"servers": []interface{}{
			"8.8.8.8",
			map[string]interface{}{
				"address": "8.8.8.8",
				"domains": []string{
					"domain:googleapis.cn",
					"domain:gstatic.com",
				},
			},
		},
	}
}

func buildInbounds(socksPort int) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"listen":   "127.0.0.1",
			"port":     socksPort,
			"protocol": "socks",
			"settings": map[string]interface{}{
				"auth":      "noauth",
				"udp":       true,
				"userLevel": 8,
			},
			"sniffing": map[string]interface{}{
				"destOverride": []string{"http", "tls"},
				"enabled":      true,
				"routeOnly":    false,
			},
			"tag": "socks",
		},
	}
}

func buildLog(cfg *Config) map[string]interface{} {
	logLevel := cfg.Xray.LogLevel
	if logLevel == "" {
		logLevel = "none"
	}
	return map[string]interface{}{
		"loglevel": logLevel,
	}
}

func buildOutbounds(cfg *Config, targetIP string) []map[string]interface{} {
	fragment := FragmentSettings{
		Packets:  cfg.Fragment.Packets,
		Length:   cfg.Fragment.Manual.Length,
		Interval: cfg.Fragment.Manual.Interval,
	}
	return buildOutboundsWithFragment(cfg, targetIP, fragment)
}

func buildOutboundsWithFragment(cfg *Config, targetIP string, fragment FragmentSettings) []map[string]interface{} {
	outbounds := []map[string]interface{}{}

	address := targetIP
	if address == "" {
		address = cfg.Proxy.Address
	}

	muxSettings := map[string]interface{}{
		"concurrency": cfg.Xray.Mux.Concurrency,
		"enabled":     cfg.Xray.Mux.Enabled,
	}
	if cfg.Xray.Mux.Enabled {
		if cfg.Xray.Mux.Concurrency <= 0 {
			muxSettings["concurrency"] = 8
		}
		if cfg.Xray.Mux.XudpConcurrency > 0 {
			muxSettings["xudpConcurrency"] = cfg.Xray.Mux.XudpConcurrency
		}
		if cfg.Xray.Mux.XudpProxyUDP443 != "" {
			muxSettings["xudpProxyUDP443"] = cfg.Xray.Mux.XudpProxyUDP443
		}
	}

	proxyOutbound := map[string]interface{}{
		"mux":      muxSettings,
		"protocol": "vless",
		"settings": map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": address,
					"port":    cfg.Proxy.Port,
					"users": []map[string]interface{}{
						{
							"encryption": "none",
							"flow":       cfg.Proxy.Flow,
							"id":         cfg.Proxy.UUID,
							"level":      8,
						},
					},
				},
			},
		},
		"streamSettings": buildStreamSettings(cfg, fragment),
		"tag":            "proxy",
	}
	outbounds = append(outbounds, proxyOutbound)

	directOutbound := map[string]interface{}{
		"protocol": "freedom",
		"settings": map[string]interface{}{
			"domainStrategy": "UseIP",
		},
		"tag": "direct",
	}
	outbounds = append(outbounds, directOutbound)

	blockOutbound := map[string]interface{}{
		"protocol": "blackhole",
		"settings": map[string]interface{}{
			"response": map[string]interface{}{
				"type": "http",
			},
		},
		"tag": "block",
	}
	outbounds = append(outbounds, blockOutbound)

	// Fragment outbound chops up TLS handshakes to slip past DPI
	fragmentEnabled := cfg.Fragment.Mode != "off" && cfg.Fragment.Enabled
	if fragmentEnabled {
		fragmentOutbound := map[string]interface{}{
			"protocol": "freedom",
			"settings": map[string]interface{}{
				"fragment": map[string]interface{}{
					"interval": fragment.Interval,
					"length":   fragment.Length,
					"packets":  fragment.Packets,
				},
				"noises": []map[string]interface{}{
					{
						"delay":  "10-16",
						"packet": "10-20",
						"type":   "rand",
					},
				},
			},
			"streamSettings": map[string]interface{}{
				"network": "tcp",
				"sockopt": map[string]interface{}{
					"TcpNoDelay": true,
					"mark":       255,
				},
			},
			"tag": "fragment",
		}

		if fragment.Interval == "" {
			fragmentOutbound["settings"].(map[string]interface{})["fragment"].(map[string]interface{})["interval"] = "10-20"
		}
		if fragment.Length == "" {
			fragmentOutbound["settings"].(map[string]interface{})["fragment"].(map[string]interface{})["length"] = "10-20"
		}
		if fragment.Packets == "" {
			fragmentOutbound["settings"].(map[string]interface{})["fragment"].(map[string]interface{})["packets"] = "tlshello"
		}

		outbounds = append(outbounds, fragmentOutbound)
	}

	if cfg.Network.Interface != "" {
		for _, out := range outbounds {
			if out["tag"] == "block" {
				continue
			}
			ss, ok := out["streamSettings"].(map[string]interface{})
			if !ok {
				ss = map[string]interface{}{}
				out["streamSettings"] = ss
			}
			sock, ok := ss["sockopt"].(map[string]interface{})
			if !ok {
				sock = map[string]interface{}{}
				ss["sockopt"] = sock
			}
			sock["interface"] = cfg.Network.Interface
		}
	}
	return outbounds
}

// xrayNetworkName translates this app's internal proxy.type into the network
// string xray-core v1.8.24 actually recognizes in streamSettings.network. The
// app (and vless:// links) call the transport "xhttp" — the newer, more common
// name — but xray-core's config loader for this pinned version only accepts
// "splithttp" (see infra/conf/transport_internet.go TransportProtocol.Build);
// emitting "xhttp" verbatim makes every xhttp-transport IP fail to start with
// "unknown transport protocol: xhttp".
func xrayNetworkName(t string) string {
	if t == "xhttp" {
		return "splithttp"
	}
	return t
}

func buildStreamSettings(cfg *Config, fragment FragmentSettings) map[string]interface{} {
	method := cfg.Proxy.Method
	if method == "" {
		method = "tls"
	}

	fragmentEnabled := cfg.Fragment.Mode != "off" && cfg.Fragment.Enabled

	ss := map[string]interface{}{
		"network":  xrayNetworkName(cfg.Proxy.Type),
		"security": method,
	}

	// Send traffic through the fragment outbound when enabled
	if fragmentEnabled {
		ss["sockopt"] = map[string]interface{}{
			"dialerProxy": "fragment",
		}
	}

	if method == "reality" && cfg.Proxy.Reality != nil {
		realitySettings := map[string]interface{}{
			"allowInsecure": false,
			"show":          false,
			"publicKey":     cfg.Proxy.Reality.PublicKey,
			"shortId":       cfg.Proxy.Reality.ShortId,
		}

		if cfg.Proxy.Reality.ServerName != "" {
			realitySettings["serverName"] = cfg.Proxy.Reality.ServerName
		}

		if cfg.Proxy.Reality.Fingerprint != "" {
			realitySettings["fingerprint"] = cfg.Proxy.Reality.Fingerprint
		} else {
			realitySettings["fingerprint"] = "chrome"
		}

		if cfg.Proxy.Reality.SpiderX != "" {
			realitySettings["spiderX"] = cfg.Proxy.Reality.SpiderX
		} else {
			realitySettings["spiderX"] = "/"
		}

		ss["realitySettings"] = realitySettings
	} else if method == "tls" && cfg.Proxy.TLS != nil {
		tlsSettings := map[string]interface{}{
			"allowInsecure": cfg.Proxy.TLS.AllowInsecure,
			"serverName":    firstNonEmpty(cfg.Proxy.TLS.SNI, cfg.Proxy.Address),
			"show":          false,
		}

		if len(cfg.Proxy.TLS.ALPN) > 0 {
			tlsSettings["alpn"] = cfg.Proxy.TLS.ALPN
		}

		if cfg.Proxy.TLS.Fingerprint != "" {
			tlsSettings["fingerprint"] = cfg.Proxy.TLS.Fingerprint
		} else {
			tlsSettings["fingerprint"] = "chrome"
		}

		ss["tlsSettings"] = tlsSettings
	}

	switch cfg.Proxy.Type {
	case "ws":
		wsHost := ""
		wsPath := "/"
		if cfg.Proxy.WS != nil {
			wsHost = cfg.Proxy.WS.Host
			if cfg.Proxy.WS.Path != "" {
				wsPath = cfg.Proxy.WS.Path
			}
		}
		ss["wsSettings"] = map[string]interface{}{
			"headers": map[string]string{
				"Host": wsHost,
			},
			"host": wsHost,
			"path": wsPath,
		}
	case "xhttp":
		xhttpHost := ""
		xhttpPath := "/"
		xhttpMode := "auto"
		if cfg.Proxy.Xhttp != nil {
			xhttpHost = cfg.Proxy.Xhttp.Host
			if cfg.Proxy.Xhttp.Path != "" {
				xhttpPath = cfg.Proxy.Xhttp.Path
			}
			if cfg.Proxy.Xhttp.Mode != "" {
				xhttpMode = cfg.Proxy.Xhttp.Mode
			}
		}
		ss["splithttpSettings"] = map[string]interface{}{
			"host": xhttpHost,
			"path": xhttpPath,
			"mode": xhttpMode,
		}
	case "grpc":
		serviceName := ""
		authority := ""
		multiMode := false
		idleTimeout := 60
		healthCheckTimeout := 20
		if cfg.Proxy.Grpc != nil {
			serviceName = cfg.Proxy.Grpc.ServiceName
			authority = cfg.Proxy.Grpc.Authority
			multiMode = cfg.Proxy.Grpc.MultiMode
			if cfg.Proxy.Grpc.IdleTimeout > 0 {
				idleTimeout = cfg.Proxy.Grpc.IdleTimeout
			}
			if cfg.Proxy.Grpc.HealthCheckTimeout > 0 {
				healthCheckTimeout = cfg.Proxy.Grpc.HealthCheckTimeout
			}
		}
		ss["grpcSettings"] = map[string]interface{}{
			"serviceName":          serviceName,
			"authority":            authority,
			"multiMode":            multiMode,
			"idle_timeout":         idleTimeout,
			"health_check_timeout": healthCheckTimeout,
		}
	case "httpupgrade":
		httpHost := ""
		httpPath := "/"
		if cfg.Proxy.WS != nil {
			httpHost = cfg.Proxy.WS.Host
			if cfg.Proxy.WS.Path != "" {
				httpPath = cfg.Proxy.WS.Path
			}
		}
		ss["httpupgradeSettings"] = map[string]interface{}{
			"headers": map[string]string{
				"Host": httpHost,
			},
			"host": httpHost,
			"path": httpPath,
		}
	case "tcp":
		ss["tcpSettings"] = map[string]interface{}{
			"header": map[string]interface{}{
				"type": "none",
			},
		}
	}

	return ss
}

func buildRouting() map[string]interface{} {
	return map[string]interface{}{
		"domainStrategy": "IPIfNonMatch",
		"rules": []map[string]interface{}{
			{
				"ip":          []string{"8.8.8.8"},
				"outboundTag": "proxy",
				"port":        "53",
				"type":        "field",
			},
			{
				"ip":          []string{"223.5.5.5"},
				"outboundTag": "direct",
				"port":        "53",
				"type":        "field",
			},
			{
				"network":     "udp",
				"outboundTag": "block",
				"port":        "443",
				"type":        "field",
			},
			{
				"outboundTag": "proxy",
				"port":        "0-65535",
				"type":        "field",
			},
		},
	}
}
