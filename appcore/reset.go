package appcore

import (
	"os"
	"path/filepath"
	"regexp"
)

// ResetConnection changes only the connection, never measurement preferences.
func (s *Session) ResetConnection() { s.Config.Proxy = DefaultSession().Config.Proxy }

// ResetScan clears targets and scan tuning, preserving the connection and settings.
func (s *Session) ResetScan() {
	d := DefaultSession()
	s.Config.Scan, s.Ranges, s.Mode, s.Source = d.Config.Scan, "", d.Mode, d.Source
}

// ResetSettings resets all tuning shown in Settings, not the connection or targets.
func (s *Session) ResetSettings() {
	d := DefaultSession()
	s.Config.Scan, s.Config.Fragment, s.Config.Output, s.Config.Xray = d.Config.Scan, d.Config.Fragment, d.Config.Output, d.Config.Xray
	s.Debug = false
}

var resultFile = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}_\d{6}(\.\d+)?_results\.(csv|json)$`)

// ClearSavedResults removes only files named by SaveResults, never a whole output
// directory (which can be user-selected and contain unrelated documents).
func ClearSavedResults(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !resultFile.MatchString(e.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// ClearStoredSession removes app-managed settings/debug data only. The caller
// clears the active logger separately and resets Fyne's appearance preferences.
func ClearStoredSession(dir string, s *Session) error {
	for _, output := range []string{OutputDirectory(dir, s.Config), filepath.Join(dir, "results")} {
		if err := ClearSavedResults(output); err != nil {
			return err
		}
	}
	for _, name := range []string{"session.json", "first_config.jsonc"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
