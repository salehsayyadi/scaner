// Package appcore connects the native UI to the existing scanner. It deliberately
// has no UI or platform dependencies so the same engine is used on Windows and Android.
package appcore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"piyazche/config"
)

type Session struct {
	Config *config.Config `json:"config"`
	Ranges string         `json:"ranges"`
	Mode   string         `json:"mode"`
	Debug  bool           `json:"debug"`
	Top    int            `json:"top"`
}

func DefaultSession() *Session {
	return &Session{Config: config.DefaultConfig(), Mode: ModeXray, Top: 10}
}

func LoadSession(dir string) (*Session, error) {
	s := DefaultSession()
	b, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return DefaultSession(), fmt.Errorf("saved settings could not be read: %w", err)
	}
	if s.Config == nil {
		s.Config = config.DefaultConfig()
	}
	if s.Mode != ModeXray && s.Mode != ModeConnectivity && s.Mode != ModeCheck {
		s.Mode = ModeXray
	}
	return s, nil
}

// SaveSession is atomic: a crash during saving cannot truncate the prior session.
// Connections can contain credentials, so their file is private to the user.
func SaveSession(dir string, s *Session) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".session-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "session.json"))
}

func OutputDirectory(dataDir string, cfg *config.Config) string {
	p := cfg.Output.Directory
	if p == "" {
		p = "results"
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dataDir, p)
	}
	return p
}
