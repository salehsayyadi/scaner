package tui

import (
	"fmt"
	"strconv"
	"strings"

	"piyazche/config"
)

type fkind int

const (
	fInt fkind = iota
	fFloat
	fBool
	fChoice
	fString
)

// sfield is one editable row in the Settings tab. It reads/writes directly into
// the shared *config.Config.
type sfield struct {
	label   string
	kind    fkind
	choices []string
	help    string

	display func(c *config.Config) string
	apply   func(c *config.Config, s string) // commit typed text (int/float/string)
	step    func(c *config.Config, dir int)  // inc/dec/toggle/cycle
}

func (f sfield) editable() bool {
	return f.kind == fInt || f.kind == fFloat || f.kind == fString
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if hi > 0 && v > hi {
		return hi
	}
	return v
}

func cycle(cur string, opts []string, dir int) string {
	idx := 0
	for i, o := range opts {
		if strings.EqualFold(o, cur) {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(opts)) % len(opts)
	return opts[idx]
}

func intField(label, help string, lo, hi, stepVal int, get func(*config.Config) *int) sfield {
	return sfield{
		label: label, kind: fInt, help: help,
		display: func(c *config.Config) string { return strconv.Itoa(*get(c)) },
		apply: func(c *config.Config, s string) {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				*get(c) = clampInt(n, lo, hi)
			}
		},
		step: func(c *config.Config, dir int) {
			*get(c) = clampInt(*get(c)+dir*stepVal, lo, hi)
		},
	}
}

func floatField(label, help string, step float64, get func(*config.Config) *float64) sfield {
	return sfield{
		label: label, kind: fFloat, help: help,
		display: func(c *config.Config) string { return strconv.FormatFloat(*get(c), 'f', -1, 64) },
		apply: func(c *config.Config, s string) {
			if n, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && n >= 0 {
				*get(c) = n
			}
		},
		step: func(c *config.Config, dir int) {
			v := *get(c) + float64(dir)*step
			if v < 0 {
				v = 0
			}
			*get(c) = v
		},
	}
}

func boolField(label, help string, get func(*config.Config) *bool) sfield {
	return sfield{
		label: label, kind: fBool, help: help,
		display: func(c *config.Config) string {
			if *get(c) {
				return "on"
			}
			return "off"
		},
		step: func(c *config.Config, dir int) { p := get(c); *p = !*p },
	}
}

func choiceField(label, help string, opts []string, get func(*config.Config) *string) sfield {
	return sfield{
		label: label, kind: fChoice, choices: opts, help: help,
		display: func(c *config.Config) string { return *get(c) },
		step:    func(c *config.Config, dir int) { p := get(c); *p = cycle(*p, opts, dir) },
	}
}

func stringField(label, help string, get func(*config.Config) *string) sfield {
	return sfield{
		label: label, kind: fString, help: help,
		display: func(c *config.Config) string {
			v := *get(c)
			if v == "" {
				return "(empty)"
			}
			return v
		},
		apply: func(c *config.Config, s string) { *get(c) = strings.TrimSpace(s) },
	}
}

// kbField edits an int that is stored in bytes but shown/typed in KB.
func kbField(label, help string, get func(*config.Config) *int) sfield {
	return sfield{
		label: label, kind: fInt, help: help,
		display: func(c *config.Config) string { return fmt.Sprintf("%d", *get(c)/1024) },
		apply: func(c *config.Config, s string) {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
				*get(c) = n * 1024
			}
		},
		step: func(c *config.Config, dir int) {
			kb := *get(c)/1024 + dir*128
			if kb < 1 {
				kb = 1
			}
			*get(c) = kb * 1024
		},
	}
}

// sniField edits the SNI for tls configs or the serverName for reality configs,
// whichever is active — guarding against nil sub-structs.
func sniField() sfield {
	return sfield{
		label: "SNI / serverName", kind: fString, help: "TLS SNI fed to the CDN",
		display: func(c *config.Config) string {
			if c.Proxy.TLS != nil {
				return c.Proxy.TLS.SNI
			}
			if c.Proxy.Reality != nil {
				return c.Proxy.Reality.ServerName
			}
			return ""
		},
		apply: func(c *config.Config, s string) {
			s = strings.TrimSpace(s)
			if c.Proxy.TLS != nil {
				c.Proxy.TLS.SNI = s
			} else if c.Proxy.Reality != nil {
				c.Proxy.Reality.ServerName = s
			}
		},
	}
}

// buildFields lists every setting exposed in the Settings tab.
func buildFields() []sfield {
	return []sfield{
		// Connection (mostly filled from the pasted config, editable here)
		stringField("Address", "server address (domain/IP)", func(c *config.Config) *string { return &c.Proxy.Address }),
		intField("Port", "server port", 1, 65535, 1, func(c *config.Config) *int { return &c.Proxy.Port }),
		sniField(),

		// Speed / connection
		intField("Threads (speed)", "concurrent workers", 1, 512, 1, func(c *config.Config) *int { return &c.Scan.Threads }),
		intField("Timeout (s)", "per-request timeout", 1, 120, 1, func(c *config.Config) *int { return &c.Scan.Timeout }),
		intField("Retries", "attempts per IP", 1, 10, 1, func(c *config.Config) *int { return &c.Scan.Retries }),
		intField("Max latency (ms)", "drop slower IPs (0=off)", 0, 0, 100, func(c *config.Config) *int { return &c.Scan.MaxLatency }),

		// Ping / loss
		boolField("Realistic ping", "warm tunnel, take min sample", func(c *config.Config) *bool { return &c.Scan.RealisticPing }),
		boolField("Handshake test", "require + time a real TLS handshake", func(c *config.Config) *bool { return &c.Scan.HandshakeTest }),
		intField("Ping samples", "requests per IP for ping+loss", 1, 50, 1, func(c *config.Config) *int { return &c.Scan.PingSamples }),
		boolField("Packet-loss test", "measure loss %", func(c *config.Config) *bool { return &c.Scan.LossTest }),
		floatField("Max loss %", "drop lossy IPs (0=off)", 5, func(c *config.Config) *float64 { return &c.Scan.MaxLoss }),

		// Upload
		boolField("Upload test", "measure upload Mbps", func(c *config.Config) *bool { return &c.Scan.UploadTest }),
		kbField("Upload size (KB)", "payload to POST", func(c *config.Config) *int { return &c.Scan.UploadBytes }),
		floatField("Min upload (Mbps)", "drop weak uploaders (0=off)", 1, func(c *config.Config) *float64 { return &c.Scan.MinUpload }),

		// Download
		boolField("Download test", "measure download Mbps", func(c *config.Config) *bool { return &c.Scan.DownloadTest }),
		kbField("Download size (KB)", "bytes to GET", func(c *config.Config) *int { return &c.Scan.DownloadBytes }),
		floatField("Min download (Mbps)", "drop weak downloaders (0=off)", 1, func(c *config.Config) *float64 { return &c.Scan.MinDownload }),

		// (Fragment settings live on their own Fragment tab.)

		// Sampling / output
		intField("Sample / subnet", "IPs sampled per CIDR", 1, 65536, 1, func(c *config.Config) *int { return &c.Scan.SampleSize }),
		intField("Max IPs", "cap total IPs (0=all)", 0, 0, 50, func(c *config.Config) *int { return &c.Scan.MaxIPs }),
		boolField("Shuffle", "randomize IP order", func(c *config.Config) *bool { return &c.Scan.Shuffle }),
		stringField("Test URL", "204 endpoint for ping", func(c *config.Config) *string { return &c.Scan.TestURL }),
	}
}

// buildFragmentFields lists the editable fields on the Fragment tab. Note: the
// main scan always uses these manual values; "auto" optimization is a separate
// explicit action on the Fragment tab so it never disrupts an IP scan.
func buildFragmentFields() []sfield {
	return []sfield{
		boolField("Enabled", "fragment the TLS handshake", func(c *config.Config) *bool { return &c.Fragment.Enabled }),
		choiceField("Packets", "which packets to fragment", []string{"tlshello", "1-3", "1-5"}, func(c *config.Config) *string { return &c.Fragment.Packets }),
		stringField("Length", "fragment size range, e.g. 10-20", func(c *config.Config) *string { return &c.Fragment.Manual.Length }),
		stringField("Interval", "delay ms range, e.g. 10-20", func(c *config.Config) *string { return &c.Fragment.Manual.Interval }),
		intField("Opt length min", "optimizer search: size min", 1, 1500, 1, func(c *config.Config) *int { return &c.Fragment.Auto.LengthRange.Min }),
		intField("Opt length max", "optimizer search: size max", 1, 1500, 1, func(c *config.Config) *int { return &c.Fragment.Auto.LengthRange.Max }),
		intField("Opt interval min", "optimizer search: interval min", 1, 1000, 1, func(c *config.Config) *int { return &c.Fragment.Auto.IntervalRange.Min }),
		intField("Opt interval max", "optimizer search: interval max", 1, 1000, 1, func(c *config.Config) *int { return &c.Fragment.Auto.IntervalRange.Max }),
		intField("Opt max tests", "tries per zone", 1, 200, 1, func(c *config.Config) *int { return &c.Fragment.Auto.MaxTests }),
	}
}
