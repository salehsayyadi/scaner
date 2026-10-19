package appcore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"piyazche/config"
	"piyazche/scanner"
	"testing"
)

func TestNetworkPersistenceAndResetScopes(t *testing.T) {
	s := DefaultSession()
	s.Config.Network.Interface = "Wi-Fi"
	dir := t.TempDir()
	if e := SaveSession(dir, s); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadSession(dir)
	if e != nil || loaded.Config.Network.Interface != "Wi-Fi" {
		t.Fatal("network selection lost", e)
	}
	s.ResetConnection()
	s.ResetScan()
	if s.Config.Network.Interface != "Wi-Fi" {
		t.Fatal("scoped resets changed network")
	}
	s.ResetSettings()
	if s.Config.Network.Interface != "" {
		t.Fatal("settings reset failed")
	}
}
func TestMissingNetworkRejectedForEveryRunMode(t *testing.T) {
	c := config.DefaultConfig()
	c.Network.Interface = "scaner-missing-564"
	for _, mode := range []string{ModeConnectivity, ModeXray, ModeCheck} {
		if _, e := NewRun(context.Background(), c, "127.0.0.1", mode, false); e == nil {
			t.Fatal("missing network accepted", mode)
		}
	}
}
func TestCSVRetainsTestInterface(t *testing.T) {
	var b bytes.Buffer
	if e := EncodeResults(&b, []scanner.Result{{IP: "192.0.2.1", Success: true, TestInterface: "Wi-Fi", Range: "192.0.2.0/24"}}, "csv"); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "results.csv")
	os.WriteFile(p, b.Bytes(), 0600)
	rows, e := scanner.LoadResultsCSV(p)
	if e != nil || len(rows) != 1 || rows[0].TestInterface != "Wi-Fi" {
		t.Fatal("interface provenance lost", e)
	}
}
