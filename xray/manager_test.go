package xray

import (
	"bytes"
	"testing"

	"piyazche/config"

	"github.com/xtls/xray-core/infra/conf/serial"
)

// TestGeneratedConfigBuildsForEveryTransport proves the JSON produced by
// config.GenerateXrayConfig actually loads through xray-core's own
// serial.DecodeJSONConfig + Build() — the same path Manager.Start uses. This
// is the real proof a transport works: string-matching the JSON (as the
// config package's own tests do) can't catch a network name xray-core's
// loader doesn't recognize (e.g. "xhttp" vs the "splithttp" it actually
// expects), because that only fails inside Build().
func TestGeneratedConfigBuildsForEveryTransport(t *testing.T) {
	base := config.DefaultConfig()
	base.Proxy.UUID = "11111111-2222-3333-4444-555555555555"
	base.Proxy.Address = "1.2.3.4"
	base.Proxy.Port = 443
	base.Proxy.TLS = &config.TlsConfig{SNI: "example.com", ALPN: []string{"h2", "http/1.1"}, Fingerprint: "chrome"}

	cases := []struct {
		name string
		mod  func(c *config.Config)
	}{
		{"ws", func(c *config.Config) {
			c.Proxy.Type = "ws"
			c.Proxy.WS = &config.WsConfig{Host: "cdn.example.com", Path: "/ws"}
		}},
		{"xhttp", func(c *config.Config) {
			c.Proxy.Type = "xhttp"
			c.Proxy.Xhttp = &config.XhttpConfig{Host: "moj.global.ssl.fastly.net", Path: "/", Mode: "auto"}
		}},
		{"grpc", func(c *config.Config) {
			c.Proxy.Type = "grpc"
			c.Proxy.Grpc = &config.GrpcConfig{ServiceName: "svc"}
		}},
		{"httpupgrade", func(c *config.Config) {
			c.Proxy.Type = "httpupgrade"
			c.Proxy.WS = &config.WsConfig{Host: "cdn.example.com", Path: "/hu"}
		}},
		{"tcp", func(c *config.Config) {
			c.Proxy.Type = "tcp"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base.Clone()
			tc.mod(cfg)

			data, err := config.GenerateXrayConfig(cfg, cfg.Proxy.Address, 19999)
			if err != nil {
				t.Fatalf("GenerateXrayConfig: %v", err)
			}

			jsonConfig, err := serial.DecodeJSONConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("DecodeJSONConfig: %v\n%s", err, data)
			}
			if _, err := jsonConfig.Build(); err != nil {
				t.Fatalf("xray-core rejected the %s config: %v\n%s", tc.name, err, data)
			}
		})
	}
}
