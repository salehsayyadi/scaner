package appcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"piyazche/scanner"
	"reflect"
	"testing"
	"time"
)

func TestResetScopesPreserveUnrelatedData(t *testing.T) {
	s := DefaultSession()
	s.Config.Proxy.UUID, s.Config.Proxy.Address = "private", "example.com"
	s.Ranges, s.Config.Scan.Timeout, s.Config.Fragment.Manual.Length, s.Debug = "192.0.2.1", 77, "55-66", true
	before := s.Config.Clone()
	s.ResetConnection()
	if s.Config.Proxy.UUID != "" || !reflect.DeepEqual(s.Config.Scan, before.Scan) || !reflect.DeepEqual(s.Config.Fragment, before.Fragment) || s.Ranges != "192.0.2.1" {
		t.Fatal("connection reset crossed scope")
	}
	s.Config.Proxy.UUID = "keep"
	s.ResetScan()
	if s.Config.Proxy.UUID != "keep" || s.Ranges != "" || s.Config.Scan.Timeout != 10 || s.Config.Fragment.Manual.Length != "55-66" || !s.Debug {
		t.Fatal("scan reset crossed scope")
	}
	s.Ranges = "192.0.2.4"
	s.ResetSettings()
	if s.Ranges != "192.0.2.4" || s.Config.Proxy.UUID != "keep" || s.Debug || s.Config.Fragment.Enabled {
		t.Fatal("settings reset crossed scope")
	}
}

func TestFullResetDeletesOnlyManagedFiles(t *testing.T) {
	dir, output := t.TempDir(), t.TempDir()
	s := DefaultSession()
	s.Config.Output.Directory = output
	for _, p := range []string{filepath.Join(dir, "session.json"), filepath.Join(dir, "first_config.jsonc"), filepath.Join(output, "2026-10-19_120000.123_results.json"), filepath.Join(output, "notes.json"), filepath.Join(dir, "keep.txt")} {
		if err := os.WriteFile(p, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := ClearStoredSession(dir, s); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(dir, "session.json"), filepath.Join(dir, "first_config.jsonc"), filepath.Join(output, "2026-10-19_120000.123_results.json")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("managed data remains", p)
		}
	}
	for _, p := range []string{filepath.Join(output, "notes.json"), filepath.Join(dir, "keep.txt")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("unrelated file deleted", p)
		}
	}
}

func TestSavedScanRetainsFailuresAndReasons(t *testing.T) {
	dir := t.TempDir()
	s := DefaultSession()
	rows := scanner.NewResultCollector()
	rows.Add(scanner.Result{IP: "192.0.2.1", Success: true, Connected: true, Latency: time.Millisecond})
	rows.Add(scanner.Result{IP: "192.0.2.2", Success: false, Connected: true, Error: "connected, but latency above limit"})
	for _, format := range []string{"csv", "json"} {
		s.Config.Output.Format = format
		p, err := SaveResults(dir, s.Config, rows, "2026-10-19_120000")
		if err != nil {
			t.Fatal(err)
		}
		var loaded []scanner.Result
		if format == "csv" {
			loaded, err = scanner.LoadResultsCSV(p)
		} else {
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			err = json.Unmarshal(b, &loaded)
		}
		if err != nil || len(loaded) != 2 || loaded[1].Success || !loaded[1].Connected || loaded[1].Error == "" {
			t.Fatalf("%s lost failure details: %+v err=%v", format, loaded, err)
		}
	}
}
