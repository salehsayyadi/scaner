package config

import (
	"strings"
	"testing"
)

func TestParseVlessWSTLS(t *testing.T) {
	link := "vless://11111111-2222-3333-4444-555555555555@104.16.0.1:443?encryption=none&security=tls&sni=example.com&fp=chrome&type=ws&host=cdn.example.com&path=%2Fwspath&alpn=h2%2Chttp%2F1.1#myremark"
	cfg, err := ParseConfigInput(link)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cfg.Proxy.UUID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("uuid = %q", cfg.Proxy.UUID)
	}
	if cfg.Proxy.Address != "104.16.0.1" || cfg.Proxy.Port != 443 {
		t.Errorf("addr/port = %q/%d", cfg.Proxy.Address, cfg.Proxy.Port)
	}
	if cfg.Proxy.Method != "tls" || cfg.Proxy.Type != "ws" {
		t.Errorf("method/type = %q/%q", cfg.Proxy.Method, cfg.Proxy.Type)
	}
	if cfg.Proxy.TLS == nil || cfg.Proxy.TLS.SNI != "example.com" {
		t.Errorf("sni wrong: %+v", cfg.Proxy.TLS)
	}
	if cfg.Proxy.WS == nil || cfg.Proxy.WS.Host != "cdn.example.com" || cfg.Proxy.WS.Path != "/wspath" {
		t.Errorf("ws wrong: %+v", cfg.Proxy.WS)
	}
	if len(cfg.Proxy.TLS.ALPN) != 2 {
		t.Errorf("alpn = %v", cfg.Proxy.TLS.ALPN)
	}
}

// TestParseUserConfig is the exact config the user pasted (emoji + space in the
// remark, insecure=0, path with %2F).
func TestParseUserConfig(t *testing.T) {
	link := "vless://932a23c7-2864-4874-a184-0aad82ee6708@89.116.180.87:8443?encryption=none&security=tls&sni=irmnets.com&fp=chrome&insecure=0&allowInsecure=0&type=ws&host=irmnets.com&path=%2Firmnets#\U0001F1F3\U0001F1F1 Cloudflare"
	cfg, err := ParseConfigInput(link)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cfg.Proxy.UUID != "932a23c7-2864-4874-a184-0aad82ee6708" {
		t.Errorf("uuid = %q", cfg.Proxy.UUID)
	}
	if cfg.Proxy.Address != "89.116.180.87" || cfg.Proxy.Port != 8443 {
		t.Errorf("addr/port = %q/%d", cfg.Proxy.Address, cfg.Proxy.Port)
	}
	if cfg.Proxy.Method != "tls" || cfg.Proxy.Type != "ws" {
		t.Errorf("method/type = %q/%q", cfg.Proxy.Method, cfg.Proxy.Type)
	}
	if cfg.Proxy.TLS == nil || cfg.Proxy.TLS.SNI != "irmnets.com" || cfg.Proxy.TLS.AllowInsecure {
		t.Errorf("tls wrong: %+v", cfg.Proxy.TLS)
	}
	if cfg.Proxy.WS == nil || cfg.Proxy.WS.Host != "irmnets.com" || cfg.Proxy.WS.Path != "/irmnets" {
		t.Errorf("ws wrong: %+v", cfg.Proxy.WS)
	}
}

// TestParseVlessXhttpFastly is the user's real xhttp-over-Fastly link (fp=firefox,
// mode=auto, host != sni — the CDN-fronting case where the SNI hits Fastly's edge
// but the Host header carries the actual Fastly service domain).
func TestParseVlessXhttpFastly(t *testing.T) {
	link := "vless://dc34c72e-39dd-4581-ae4f-06cb0b394bb7@140.248.177.64:443?type=xhttp&security=tls&sni=global.fastly.com&fp=firefox&path=%2F&host=moj.global.ssl.fastly.net&mode=auto&encryption=none#\U0001F1F3\U0001F1F1 Fastly"
	cfg, err := ParseConfigInput(link)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cfg.Proxy.UUID != "dc34c72e-39dd-4581-ae4f-06cb0b394bb7" {
		t.Errorf("uuid = %q", cfg.Proxy.UUID)
	}
	if cfg.Proxy.Address != "140.248.177.64" || cfg.Proxy.Port != 443 {
		t.Errorf("addr/port = %q/%d", cfg.Proxy.Address, cfg.Proxy.Port)
	}
	if cfg.Proxy.Method != "tls" || cfg.Proxy.Type != "xhttp" {
		t.Errorf("method/type = %q/%q", cfg.Proxy.Method, cfg.Proxy.Type)
	}
	if cfg.Proxy.TLS == nil || cfg.Proxy.TLS.SNI != "global.fastly.com" || cfg.Proxy.TLS.Fingerprint != "firefox" {
		t.Errorf("tls wrong: %+v", cfg.Proxy.TLS)
	}
	if cfg.Proxy.Xhttp == nil || cfg.Proxy.Xhttp.Host != "moj.global.ssl.fastly.net" || cfg.Proxy.Xhttp.Path != "/" || cfg.Proxy.Xhttp.Mode != "auto" {
		t.Errorf("xhttp wrong: %+v", cfg.Proxy.Xhttp)
	}

	// Must also lower into a valid xray-core v1.8.24 outbound. xray-core's own
	// config loader only recognizes the network name "splithttp" (with
	// "splithttpSettings") — NOT "xhttp"/"xhttpSettings" — even though the app's
	// internal Config.Proxy.Type and vless:// links both call it "xhttp". Emitting
	// "xhttp" verbatim makes xray-core reject the config with "unknown transport
	// protocol: xhttp" and every IP fails.
	data, err := GenerateXrayConfig(cfg, cfg.Proxy.Address, 10808)
	if err != nil {
		t.Fatalf("GenerateXrayConfig failed: %v", err)
	}
	out := string(data)
	if strings.Contains(out, `"network": "xhttp"`) || strings.Contains(out, `"xhttpSettings"`) {
		t.Errorf("generated xray config uses the app-internal \"xhttp\" name instead of xray-core's \"splithttp\":\n%s", out)
	}
	if !strings.Contains(out, `"network": "splithttp"`) || !strings.Contains(out, `"splithttpSettings"`) {
		t.Errorf("generated xray config missing splithttp stream settings:\n%s", out)
	}
}

func TestParseVlessReality(t *testing.T) {
	link := "vless://aaaa@1.2.3.4:8443?security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBKEY123&sid=abcd&spx=%2F&type=tcp&flow=xtls-rprx-vision#r"
	cfg, err := ParseConfigInput(link)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cfg.Proxy.Method != "reality" {
		t.Fatalf("method = %q", cfg.Proxy.Method)
	}
	if cfg.Proxy.Reality == nil || cfg.Proxy.Reality.PublicKey != "PUBKEY123" || cfg.Proxy.Reality.ShortId != "abcd" {
		t.Errorf("reality wrong: %+v", cfg.Proxy.Reality)
	}
	if cfg.Proxy.Reality.ServerName != "www.microsoft.com" {
		t.Errorf("serverName = %q", cfg.Proxy.Reality.ServerName)
	}
	if cfg.Proxy.Port != 8443 || cfg.Proxy.Type != "tcp" {
		t.Errorf("port/type = %d/%q", cfg.Proxy.Port, cfg.Proxy.Type)
	}
}

func TestParseXrayJSON(t *testing.T) {
	js := `{
      "outbounds": [
        {
          "protocol": "vless",
          "settings": { "vnext": [ { "address": "cf.example.com", "port": 443, "users": [ { "id": "uuid-xyz" } ] } ] },
          "streamSettings": {
            "network": "ws",
            "security": "tls",
            "tlsSettings": { "serverName": "sni.example.com", "alpn": ["h2"], "fingerprint": "chrome" },
            "wsSettings": { "path": "/p", "headers": { "Host": "h.example.com" } }
          }
        },
        { "protocol": "freedom", "settings": { "fragment": { "packets": "1-3", "length": "40-60", "interval": "5-10" } } }
      ]
    }`
	cfg, err := ParseConfigInput(js)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cfg.Proxy.UUID != "uuid-xyz" || cfg.Proxy.Address != "cf.example.com" {
		t.Errorf("vnext wrong: %q %q", cfg.Proxy.UUID, cfg.Proxy.Address)
	}
	if cfg.Proxy.Type != "ws" || cfg.Proxy.WS == nil || cfg.Proxy.WS.Host != "h.example.com" || cfg.Proxy.WS.Path != "/p" {
		t.Errorf("ws wrong: %+v", cfg.Proxy.WS)
	}
	if cfg.Fragment.Packets != "1-3" || cfg.Fragment.Manual.Length != "40-60" {
		t.Errorf("fragment wrong: %+v", cfg.Fragment)
	}
}
