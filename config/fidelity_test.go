package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestShareLinkPreservesWireSettings(t *testing.T) {
	cases := []struct{ name, link, security, flow string }{
		{"plain TCP", "vless://test@origin.example:80?type=tcp&security=none", "none", ""},
		{"plain WS", "vless://test@origin.example:80?type=ws&security=none&path=%2Fws", "none", ""},
		{"implicit none", "vless://test@origin.example:80?type=tcp", "none", ""},
		{"TLS default SNI", "vless://test@origin.example:443?type=ws&security=tls", "tls", ""},
		{"Vision flow", "vless://test@origin.example:443?type=tcp&security=reality&sni=example.com&pbk=test&flow=xtls-rprx-vision", "reality", "xtls-rprx-vision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := ParseConfigInput(tc.link)
			if err != nil {
				t.Fatal(err)
			}
			if c.Fragment.Enabled || c.Fragment.Mode != "off" {
				t.Fatal("link acquired unsolicited fragmentation")
			}
			data, err := GenerateXrayConfig(c, "192.0.2.10", 10808)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			out := asMap(asSlice(raw["outbounds"])[0])
			ss := asMap(out["streamSettings"])
			user := asMap(asSlice(asMap(asSlice(asMap(out["settings"])["vnext"])[0])["users"])[0])
			if asString(ss["security"]) != tc.security || asString(user["flow"]) != tc.flow {
				t.Fatalf("wire settings changed: %s", data)
			}
			if tc.security == "none" {
				if _, ok := ss["tlsSettings"]; ok {
					t.Fatal("TLS injected into plain transport")
				}
			}
			if tc.security == "tls" {
				ts := asMap(ss["tlsSettings"])
				if ts["serverName"] != "origin.example" {
					t.Fatal("SNI replaced by candidate IP")
				}
				if _, ok := ts["alpn"]; ok {
					t.Fatal("unspecified ALPN forced")
				}
			}
		})
	}
}

func TestXrayJSONPreservesFlowSecurityAndMux(t *testing.T) {
	c, err := ParseConfigInput(`{"outbounds":[{"protocol":"vless","mux":{"enabled":true,"concurrency":3},"settings":{"vnext":[{"address":"origin.example","port":80,"users":[{"id":"test","flow":"xtls-rprx-vision","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"none"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Proxy.Flow != "xtls-rprx-vision" || c.Proxy.Method != "none" || !c.Xray.Mux.Enabled || c.Xray.Mux.Concurrency != 3 {
		t.Fatalf("lost outbound options: %+v", c)
	}
}

func TestUnsupportedSettingsNotSilentlyRewritten(t *testing.T) {
	for _, q := range []string{"type=h2&security=tls", "type=tcp&security=xtls", "type=tcp&headerType=http", "type=tcp&encryption=unsupported"} {
		if _, err := ParseConfigInput("vless://test@origin.example:443?sni=origin.example&" + q); err == nil {
			t.Errorf("silently accepted %s", q)
		}
	}
	c, err := ParseConfigInput("vless://test@origin.example:443?type=ws&security=tls&sni=front.example&host=back.example&path=%2Fa%3Fed%3D2048")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := GenerateXrayConfig(c, "192.0.2.2", 10808)
	if !strings.Contains(string(data), "front.example") || !strings.Contains(string(data), "back.example") || !strings.Contains(string(data), "/a?ed=2048") {
		t.Fatal(string(data))
	}
}
