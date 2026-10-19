package config

import (
	"encoding/json"
	"net"
	"piyazche/netroute"
	"testing"
)

func TestNetworkBindingReachesEveryPhysicalOutbound(t *testing.T) {
	if !netroute.Supported() {
		t.Skip("binding unsupported")
	}
	all, _ := net.Interfaces()
	name := ""
	for _, a := range all {
		if a.Flags&net.FlagLoopback != 0 {
			name = a.Name
			break
		}
	}
	if name == "" {
		t.Fatal("no loopback adapter")
	}
	for _, fragment := range []bool{false, true} {
		c := DefaultConfig()
		c.Network.Interface = name
		c.Fragment.Enabled = fragment
		if fragment {
			c.Fragment.Mode = "manual"
		}
		data, e := GenerateXrayConfig(c, "127.0.0.1", 12345)
		if e != nil {
			t.Fatal(e)
		}
		var cfg map[string]interface{}
		json.Unmarshal(data, &cfg)
		for _, raw := range cfg["outbounds"].([]interface{}) {
			o := raw.(map[string]interface{})
			if o["tag"] == "block" {
				continue
			}
			ss := o["streamSettings"].(map[string]interface{})
			sock := ss["sockopt"].(map[string]interface{})
			if sock["interface"] != name {
				t.Fatalf("%v not bound", o["tag"])
			}
			if fragment && o["tag"] == "proxy" && sock["dialerProxy"] != "fragment" {
				t.Fatal("lost fragment dialer proxy")
			}
		}
	}
	c := DefaultConfig()
	data, e := GenerateXrayConfig(c, "127.0.0.1", 12345)
	if e != nil || string(data) == "" {
		t.Fatal(e)
	}
	if c.Network.Interface != "" {
		t.Fatal("automatic changed")
	}
}
