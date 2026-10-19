package scanner

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"piyazche/utils"
)

// Result represents the scan result for a single IP
type Result struct {
	TestInterface string        `json:"test_interface,omitempty"`
	IP            string        `json:"ip"`
	Range         string        `json:"range,omitempty"`
	Success       bool          `json:"success"`
	Connected     bool          `json:"connected,omitempty"` // Xray worked, even when a quality filter rejected the IP
	Latency       time.Duration `json:"latency"`
	LatencyMs     int64         `json:"latency_ms"`
	StatusCode    int           `json:"status_code,omitempty"`
	Error         string        `json:"error,omitempty"`
	TestedAt      time.Time     `json:"tested_at"`

	// Quality metrics from the warm sample loop
	AvgLatencyMs int64   `json:"avg_latency_ms,omitempty"`
	JitterMs     int64   `json:"jitter_ms,omitempty"`
	HandshakeMs  int64   `json:"handshake_ms,omitempty"`
	LossPercent  float64 `json:"loss_percent"`
	Samples      int     `json:"samples,omitempty"`

	// Raw TCP connect to the proxy port, independent of the xray tunnel — tells
	// apart "IP unreachable" from "proxy protocol/handshake failed".
	TCPMs int64 `json:"tcp_ms,omitempty"`
	TCPOK bool  `json:"tcp_ok,omitempty"`

	// Throughput (Mbps) when upload/download tests are enabled
	UploadMbps     float64 `json:"upload_mbps,omitempty"`
	UploadTested   bool    `json:"upload_tested,omitempty"`
	DownloadMbps   float64 `json:"download_mbps,omitempty"`
	DownloadTested bool    `json:"download_tested,omitempty"`
}

// ResultCollector collects and manages scan results
type ResultCollector struct {
	results []Result
	good    int // successful results, kept so SuccessCount is O(1)
	mu      sync.Mutex
	onAdd   func(Result)
	rangeOf func(string) string
}

// NewResultCollector creates a new result collector
func NewResultCollector() *ResultCollector {
	return &ResultCollector{
		results: make([]Result, 0),
	}
}

// SetOnAdd registers a callback invoked (outside the lock) for every added
// result. Used by the TUI to stream live results.
func (rc *ResultCollector) SetOnAdd(fn func(Result)) {
	rc.mu.Lock()
	rc.onAdd = fn
	rc.mu.Unlock()
}

// SetRangeResolver stamps provenance before storing or emitting a result.
func (rc *ResultCollector) SetRangeResolver(fn func(string) string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.rangeOf = fn
}

// Add adds a result to the collection
func (rc *ResultCollector) Add(result Result) {
	rc.mu.Lock()
	if result.Range == "" && rc.rangeOf != nil {
		result.Range = rc.rangeOf(result.IP)
	}
	result.LatencyMs = result.Latency.Milliseconds()
	result.TestedAt = time.Now()
	rc.results = append(rc.results, result)
	if result.Success {
		rc.good++
	}
	cb := rc.onAdd
	rc.mu.Unlock()

	if cb != nil {
		cb(result)
	}
}

// GetResults returns a copy of all results
func (rc *ResultCollector) GetResults() []Result {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	results := make([]Result, len(rc.results))
	copy(results, rc.results)
	return results
}

// GetSuccessful returns only successful results
func (rc *ResultCollector) GetSuccessful() []Result {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	var successful []Result
	for _, r := range rc.results {
		if r.Success {
			successful = append(successful, r)
		}
	}
	return successful
}

// GetSortedByLatency returns successful results sorted by QUALITY: first by
// packet-loss tier (lower loss is better — an IP that drops samples is worse
// than a slightly slower but stable one), then by latency. This keeps genuinely
// good IPs at the top instead of ones that merely had a fast best-case sample.
func (rc *ResultCollector) GetSortedByLatency() []Result {
	results := rc.GetSuccessful()
	lossTier := func(loss float64) int { return int(loss) / 10 } // 0-9%,10-19%,...
	sort.Slice(results, func(i, j int) bool {
		ti, tj := lossTier(results[i].LossPercent), lossTier(results[j].LossPercent)
		if ti != tj {
			return ti < tj
		}
		return results[i].Latency < results[j].Latency
	})
	return results
}

// Count returns the total number of results
func (rc *ResultCollector) Count() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return len(rc.results)
}

// SuccessCount returns the number of successful results
func (rc *ResultCollector) SuccessCount() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	return rc.good
}

// SaveToCSV saves results to a CSV file
func (rc *ResultCollector) SaveToCSV(path string) error {
	results := rc.GetSortedByLatency()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"IP", "Ping (ms)", "TCP (ms)", "Handshake (ms)", "Jitter (ms)", "Loss (%)", "Upload (Mbps)", "Download (Mbps)", "Status", "Tested At", "Range", "Test interface"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	for _, r := range results {
		status := "failed"
		if r.StatusCode >= 200 && r.StatusCode < 300 {
			status = "success"
		}

		tcp := ""
		if r.TCPOK {
			tcp = fmt.Sprintf("%d", r.TCPMs)
		}

		row := []string{
			r.IP,
			fmt.Sprintf("%d", r.LatencyMs),
			tcp,
			fmt.Sprintf("%d", r.HandshakeMs),
			fmt.Sprintf("%d", r.JitterMs),
			fmt.Sprintf("%.1f", r.LossPercent),
			fmt.Sprintf("%.2f", r.UploadMbps),
			fmt.Sprintf("%.2f", r.DownloadMbps),
			status,
			r.TestedAt.Format(time.RFC3339), r.Range, r.TestInterface,
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	return nil
}

// SaveToJSON saves results to a JSON file
func (rc *ResultCollector) SaveToJSON(path string) error {
	results := rc.GetSortedByLatency()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// NewResultCollectorFrom builds a collector pre-filled with the given results
// (used to view a past scan loaded from CSV).
func NewResultCollectorFrom(results []Result) *ResultCollector {
	rc := NewResultCollector()
	rc.results = append(rc.results, results...)
	for _, r := range results {
		if r.Success {
			rc.good++
		}
	}
	return rc
}

// LoadResultsCSV parses a results CSV (written by SaveToCSV) back into Results.
// Columns are matched by header name so it tolerates format changes over time.
func LoadResultsCSV(path string) ([]Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("empty results file")
	}

	// Map header titles -> column index.
	idx := map[string]int{}
	for i, h := range records[0] {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	col := func(row []string, names ...string) string {
		for _, n := range names {
			if i, ok := idx[n]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
		}
		return ""
	}
	pf := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	pi := func(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }

	var out []Result
	for _, row := range records[1:] {
		ip := col(row, "ip")
		if ip == "" {
			continue
		}
		ms := pi(col(row, "ping (ms)", "latency (ms)"))
		res := Result{
			IP:          ip,
			Success:     true,
			Latency:     time.Duration(ms) * time.Millisecond,
			LatencyMs:   ms,
			HandshakeMs: pi(col(row, "handshake (ms)")),
			JitterMs:    pi(col(row, "jitter (ms)")),
			LossPercent: pf(col(row, "loss (%)")),
		}

		if status := strings.ToLower(col(row, "status")); status != "" {
			res.Success = status == "success" || status == "good" || status == "true"
		}
		res.Connected = res.Success || strings.EqualFold(col(row, "connected"), "true")
		res.Error = col(row, "error")
		res.Range = col(row, "range")
		res.TestInterface = col(row, "test interface")
		if ts := col(row, "tested at"); ts != "" {
			res.TestedAt, _ = time.Parse(time.RFC3339, ts)
		}
		if up := col(row, "upload (mbps)"); up != "" {
			res.UploadMbps = pf(up)
			res.UploadTested = res.UploadMbps > 0
		}
		if dn := col(row, "download (mbps)"); dn != "" {
			res.DownloadMbps = pf(dn)
			res.DownloadTested = res.DownloadMbps > 0
		}
		if tcp := col(row, "tcp (ms)"); tcp != "" {
			res.TCPMs = pi(tcp)
			res.TCPOK = true
		}
		out = append(out, res)
	}
	return out, nil
}

// GenerateOutputPath generates a timestamped output file path
func GenerateOutputPath(format string) string {
	timestamp := time.Now().Format("2006-01-02_150405")
	filename := fmt.Sprintf("%s_results.%s", timestamp, format)
	return filepath.Join("results", filename)
}

// PrintTopResults prints the top N results to stdout with colors
func (rc *ResultCollector) PrintTopResults(n int) {
	results := rc.GetSortedByLatency()
	if len(results) == 0 {
		fmt.Printf("%sNo successful results found.%s\n", utils.Yellow, utils.Reset)
		return
	}

	if n > len(results) {
		n = len(results)
	}

	// Only show throughput columns when at least one result has data
	showUp, showDown := false, false
	for _, r := range results {
		if r.UploadMbps > 0 {
			showUp = true
		}
		if r.DownloadMbps > 0 {
			showDown = true
		}
	}

	fmt.Printf("\n%s%sTop %d IPs by Latency%s\n", utils.Bold, utils.Cyan, n, utils.Reset)

	// Header
	fmt.Printf("%s%-4s %-17s %9s %8s %8s %7s", utils.Bold, "#", "IP", "Ping", "TCP", "Jitter", "Loss")
	if showUp {
		fmt.Printf(" %9s", "Up")
	}
	if showDown {
		fmt.Printf(" %9s", "Down")
	}
	fmt.Printf("%s\n", utils.Reset)
	fmt.Printf("%s%s%s\n", utils.Gray, repeatChar('─', 60+boolInt(showUp)*10+boolInt(showDown)*10), utils.Reset)

	for i := 0; i < n; i++ {
		r := results[i]
		latencyColor := utils.Green
		if r.LatencyMs > 2000 {
			latencyColor = utils.Red
		} else if r.LatencyMs > 1000 {
			latencyColor = utils.Yellow
		}

		lossColor := utils.Green
		if r.LossPercent > 20 {
			lossColor = utils.Red
		} else if r.LossPercent > 0 {
			lossColor = utils.Yellow
		}

		tcpStr := "-"
		if r.TCPOK {
			tcpStr = fmt.Sprintf("%dms", r.TCPMs)
		}

		fmt.Printf("%s%-3d%s %s%-17s%s %s%7dms%s %s%7s%s %s%6dms%s %s%5.0f%%%s",
			utils.Dim, i+1, utils.Reset,
			utils.Cyan, r.IP, utils.Reset,
			latencyColor, r.LatencyMs, utils.Reset,
			utils.Gray, tcpStr, utils.Reset,
			utils.Gray, r.JitterMs, utils.Reset,
			lossColor, r.LossPercent, utils.Reset)
		if showUp {
			fmt.Printf(" %s%6.2fMb%s", utils.Magenta, r.UploadMbps, utils.Reset)
		}
		if showDown {
			fmt.Printf(" %s%6.2fMb%s", utils.Blue, r.DownloadMbps, utils.Reset)
		}
		fmt.Println()
	}
	fmt.Println()
}

func repeatChar(c rune, n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
