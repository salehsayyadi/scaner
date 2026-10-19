package appcore

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"piyazche/config"
	"piyazche/scanner"
)

// EncodeResults keeps every metric, including errors in JSON. CSV matches the
// legacy column names so existing saved-scan readers continue to work.
func EncodeResults(w io.Writer, rows []scanner.Result, format string) error {
	if format == "json" {
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		return e.Encode(rows)
	}
	if format != "csv" {
		return fmt.Errorf("unsupported output format %q", format)
	}
	c := csv.NewWriter(w)
	if err := c.Write([]string{"IP", "Ping (ms)", "TCP (ms)", "Handshake (ms)", "Jitter (ms)", "Loss (%)", "Upload (Mbps)", "Download (Mbps)", "Status", "Tested At", "Connected", "Error", "Range"}); err != nil {
		return err
	}
	for _, r := range rows {
		tcp, up, down := "", "", ""
		if r.TCPOK {
			tcp = fmt.Sprint(r.TCPMs)
		}
		if r.UploadTested {
			up = fmt.Sprintf("%.2f", r.UploadMbps)
		}
		if r.DownloadTested {
			down = fmt.Sprintf("%.2f", r.DownloadMbps)
		}
		status := "failed"
		if r.Success {
			status = "success"
		}
		if err := c.Write([]string{r.IP, fmt.Sprint(r.LatencyMs), tcp, fmt.Sprint(r.HandshakeMs), fmt.Sprint(r.JitterMs), fmt.Sprintf("%.1f", r.LossPercent), up, down, status, r.TestedAt.Format(time.RFC3339), fmt.Sprint(r.Connected), r.Error, r.Range}); err != nil {
			return err
		}
	}
	c.Flush()
	return c.Error()
}

func SaveResults(dataDir string, cfg *config.Config, results *scanner.ResultCollector, stamp string) (string, error) {
	dir := OutputDirectory(dataDir, cfg)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	format := cfg.Output.Format
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		return "", fmt.Errorf("output format must be csv or json")
	}
	dest := filepath.Join(dir, stamp+"_results."+format)
	f, err := os.CreateTemp(dir, ".results-*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	err = EncodeResults(f, results.GetResults(), format)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}
