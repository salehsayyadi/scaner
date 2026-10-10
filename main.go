package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"piyazche/applog"
	"piyazche/config"
	"piyazche/optimizer"
	"piyazche/scanner"
	"piyazche/tui"
	"piyazche/utils"

	"github.com/spf13/cobra"
)

var (
	// Version and BuildTime are set via ldflags during build
	Version   = "dev"
	BuildTime = "unknown"

	configPath   string
	subnetsPath  string
	threads      int
	outputFmt    string
	maxIPs       int
	shuffle      bool
	topN         int
	debug        bool
	fragmentMode string
	testIP       string
	checkMode    bool
	muxEnabled   string // "true", "false", or "" (use config)
	scanMode     string // "xray" (default) or "icmp"
	showVersion  bool
	linkInput    string // vless:// link or JSON config pasted on the CLI
	tuiMode      bool
)

func main() {
	// No arguments at all -> launch the interactive TUI. This is the
	// zero-prerequisite path: no config.json or ips.txt needed.
	if len(os.Args) == 1 {
		if err := tui.Run(nil); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	rootCmd := &cobra.Command{
		Use:   "piyazche",
		Short: "Piyazche - Test IP connectivity using xray-core or ICMP",
		Long: `Piyazche tests Cloudflare IP addresses for connectivity
using xray-core as a proxy or ICMP ping, measuring latency and sorting results.

Run with no arguments to open the interactive TUI.

Example:
  piyazche                       # interactive TUI
  piyazche -c config.json -s ipv4.txt -t 16
  piyazche --link "vless://..." -s 104.16.0.0/24
  piyazche -s ipv4.txt --scan-mode icmp -t 32`,
		RunE: run,
	}

	rootCmd.Flags().StringVarP(&configPath, "config", "c", "config.json", "Path to config file")
	rootCmd.Flags().StringVarP(&linkInput, "link", "l", "", "vless:// link or JSON config (skips --config)")
	rootCmd.Flags().StringVarP(&subnetsPath, "subnets", "s", "ipv4.txt", "Path to subnets file or CIDR")
	rootCmd.Flags().IntVarP(&threads, "threads", "t", 0, "Number of concurrent workers (overrides config)")
	rootCmd.Flags().StringVarP(&outputFmt, "output", "o", "csv", "Output format: csv, json")
	rootCmd.Flags().IntVar(&maxIPs, "max-ips", 0, "Maximum IPs to scan (default: all)")
	rootCmd.Flags().BoolVar(&shuffle, "shuffle", true, "Shuffle IPs before scanning")
	rootCmd.Flags().IntVar(&topN, "top", 10, "Number of top results to display")
	rootCmd.Flags().BoolVar(&debug, "debug", false, "Print xray config JSON for first IP")
	rootCmd.Flags().StringVar(&fragmentMode, "fragment-mode", "", "Fragment mode: manual, auto, off (overrides config)")
	rootCmd.Flags().StringVar(&testIP, "test-ip", "", "IP address to use for fragment optimization tests (overrides config)")
	rootCmd.Flags().BoolVar(&checkMode, "check", false, "Test single connection (required for reality mode, optional for tls mode)")
	rootCmd.Flags().StringVar(&muxEnabled, "mux", "", "Enable mux: true, false (overrides config)")
	rootCmd.Flags().StringVar(&scanMode, "scan-mode", "xray", "Scan mode: xray (proxy test) or icmp (ping only)")
	rootCmd.Flags().BoolVar(&tuiMode, "tui", false, "Launch the interactive TUI")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version information")

	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			var cfg *config.Config
			if linkInput != "" {
				if c, err := config.ParseConfigInput(linkInput); err == nil {
					cfg = c
				}
			}
			return tui.Run(cfg)
		},
	}
	tuiCmd.Flags().StringVarP(&linkInput, "link", "l", "", "vless:// link or JSON config to preload")
	rootCmd.AddCommand(tuiCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	if showVersion {
		fmt.Printf("piyazche version %s (built: %s)\n", Version, BuildTime)
		return nil
	}

	if tuiMode {
		var cfg *config.Config
		if linkInput != "" {
			if c, err := config.ParseConfigInput(linkInput); err == nil {
				cfg = c
			}
		}
		return tui.Run(cfg)
	}

	applog.Init("piyazche.log")
	defer applog.Close()

	var cfg *config.Config
	var err error
	if linkInput != "" {
		cfg, err = config.ParseConfigInput(linkInput)
		if err != nil {
			return fmt.Errorf("failed to parse --link: %w", err)
		}
	} else {
		cfg, err = config.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	if threads > 0 {
		cfg.Scan.Threads = threads
	}

	if fragmentMode != "" {
		cfg.Fragment.Mode = fragmentMode
	}

	switch muxEnabled {
	case "true":
		cfg.Xray.Mux.Enabled = true
		if cfg.Xray.Mux.Concurrency <= 0 {
			cfg.Xray.Mux.Concurrency = 8
		}
	case "false":
		cfg.Xray.Mux.Enabled = false
		cfg.Xray.Mux.Concurrency = -1
	}

	isRealityMode := cfg.Proxy.Method == "reality"

	if isRealityMode {
		cfg.Fragment.Auto.TestIP = cfg.Proxy.Address
	} else if testIP != "" {
		cfg.Fragment.Auto.TestIP = testIP
	}

	cfg.PrintConfigInfo()

	if cfg.Fragment.Mode == "auto" {
		fmt.Printf("%s%sAuto Fragment Mode%s - discovering optimal settings...\n\n",
			utils.Bold, utils.Green, utils.Reset)
		optimizedSettings, err := runFragmentOptimizer(cfg)
		if err != nil {
			fmt.Printf("%sWarning:%s fragment optimizer failed: %v\n", utils.Yellow, utils.Reset, err)
			fmt.Printf("Falling back to default manual settings...\n")
		} else if optimizedSettings != nil {
			// Apply optimized settings from ZoneResult
			cfg.Fragment.Mode = "manual"
			cfg.Fragment.Packets = optimizedSettings.Zone
			cfg.Fragment.Manual.Length = optimizedSettings.SizeRange.String()
			cfg.Fragment.Manual.Interval = optimizedSettings.IntervalRange.String()
			fmt.Printf("\n%s✓ Optimized Settings Applied%s\n", utils.Green, utils.Reset)
			fmt.Printf("  %sLength:%s   %s%s%s\n", utils.Gray, utils.Reset, utils.Cyan, cfg.Fragment.Manual.Length, utils.Reset)
			fmt.Printf("  %sInterval:%s %s%s%s\n", utils.Gray, utils.Reset, utils.Cyan, cfg.Fragment.Manual.Interval, utils.Reset)
			fmt.Printf("  %sPackets:%s  %s%s%s\n\n", utils.Gray, utils.Reset, utils.Magenta, cfg.Fragment.Packets, utils.Reset)
		}
	}

	if isRealityMode {
		if !checkMode && cfg.Fragment.Mode != "auto" {
			return fmt.Errorf("scanner mode is not available for reality. Use --check flag to test connection or --fragment-mode auto to optimize fragments")
		}

		if checkMode {
			return runCheckMode(cfg)
		}

		return nil
	}

	if checkMode {
		return runCheckMode(cfg)
	}

	// ICMP mode - simple ping scan without xray
	if scanMode == "icmp" {
		return runICMPScan(cfg)
	}

	s := scanner.NewScannerWithDebug(cfg, debug)

	if err := s.LoadIPs(subnetsPath, maxIPs, shuffle); err != nil {
		return fmt.Errorf("failed to load IPs: %w", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, stopping...")
		s.Stop()
	}()

	if err := s.Run(); err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	s.GetResults().PrintTopResults(topN)

	if s.GetResults().SuccessCount() > 0 {
		outputPath := scanner.GenerateOutputPath(outputFmt)
		if err := s.SaveResults(outputFmt, outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "%sWarning:%s failed to save results: %v\n", utils.Yellow, utils.Reset, err)
		} else {
			fmt.Printf("%sResults saved to:%s %s%s%s\n", utils.Gray, utils.Reset, utils.Cyan, outputPath, utils.Reset)
		}
	}

	return nil
}

// runCheckMode tests a single connection using the configured address
func runCheckMode(cfg *config.Config) error {
	targetIP := cfg.Proxy.Address
	if targetIP == "" {
		return fmt.Errorf("proxy.address is required for check mode")
	}

	fmt.Printf("%s%sConnection Check%s\n", utils.Bold, utils.Cyan, utils.Reset)
	fmt.Printf("%s─────────────────────────────────────────%s\n", utils.Gray, utils.Reset)
	fmt.Printf("  %sTarget:%s %s%s%s\n", utils.Gray, utils.Reset, utils.Cyan, targetIP, utils.Reset)
	fmt.Printf("  %sPort:%s   %s%d%s\n", utils.Gray, utils.Reset, utils.Yellow, cfg.Proxy.Port, utils.Reset)
	fmt.Printf("  %sMethod:%s %s%s%s\n\n", utils.Gray, utils.Reset, utils.Green, cfg.Proxy.Method, utils.Reset)

	zone := cfg.Fragment.Packets
	if zone == "" {
		zone = "tlshello"
	}

	size := 10
	interval := 10

	if cfg.Fragment.Mode == "manual" || cfg.Fragment.Mode == "" {
		var minLen int
		fmt.Sscanf(cfg.Fragment.Manual.Length, "%d", &minLen)
		if minLen > 0 {
			size = minLen
		}

		var minInt int
		fmt.Sscanf(cfg.Fragment.Manual.Interval, "%d", &minInt)
		if minInt > 0 {
			interval = minInt
		}
	}

	fmt.Printf("  %sFragment:%s Size=%s%d%s, Interval=%s%d%sms, Packets=%s%s%s\n\n",
		utils.Gray, utils.Reset,
		utils.Green, size, utils.Reset,
		utils.Green, interval, utils.Reset,
		utils.Yellow, zone, utils.Reset)

	tester := optimizer.NewFragmentTester(cfg, targetIP)
	tester.WithDebug(debug)

	fmt.Printf("Testing connection...")

	result := tester.TestSingle(zone, size, interval)

	if result.Success {
		fmt.Printf("\r%s✓ Connection successful%s\n", utils.Green, utils.Reset)
		fmt.Printf("  %sLatency:%s %s%dms%s\n", utils.Gray, utils.Reset, utils.Yellow, result.Latency.Milliseconds(), utils.Reset)
	} else {
		fmt.Printf("\r%s✗ Connection failed%s\n", utils.Red, utils.Reset)
		if result.Error != "" {
			fmt.Printf("  %sError:%s %s\n", utils.Gray, utils.Reset, result.Error)
		}
	}

	return nil
}

// runFragmentOptimizer runs the fragment optimizer to find optimal settings
func runFragmentOptimizer(cfg *config.Config) (*optimizer.ZoneResult, error) {
	var testIP string

	if cfg.Fragment.Auto.TestIP != "" {
		testIP = cfg.Fragment.Auto.TestIP
	} else {
		var testIPs []string
		var err error

		if _, statErr := os.Stat(subnetsPath); statErr == nil {
			testIPs, err = utils.ParseIPsFromFile(subnetsPath, 5, true)
		} else {
			testIPs, err = utils.ParseCIDRList(subnetsPath, 5)
			utils.ShuffleIPs(testIPs)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to load IPs for optimization: %w", err)
		}

		if len(testIPs) == 0 {
			return nil, fmt.Errorf("no IPs available for optimization")
		}

		testIP = testIPs[0]
	}

	fmt.Printf("Testing fragment settings using IP: %s\n\n", testIP)

	tester := optimizer.NewFragmentTester(cfg, testIP)
	tester.WithDebug(debug)

	finderConfig := optimizer.FinderConfig{
		MaxTriesPerZone:   cfg.Fragment.Auto.MaxTests,
		SuccessThreshold:  cfg.Fragment.Auto.SuccessThreshold,
		MinRangeWidth:     5,
		EnableCorrelation: true,
	}

	if finderConfig.MaxTriesPerZone <= 0 {
		finderConfig.MaxTriesPerZone = 20
	}
	if finderConfig.SuccessThreshold <= 0 {
		finderConfig.SuccessThreshold = 0.5
	}

	opt := optimizer.NewOptimizer(finderConfig, tester.CreateTesterFunc())

	sizeRange := optimizer.Range{
		Min: cfg.Fragment.Auto.LengthRange.Min,
		Max: cfg.Fragment.Auto.LengthRange.Max,
	}
	intervalRange := optimizer.Range{
		Min: cfg.Fragment.Auto.IntervalRange.Min,
		Max: cfg.Fragment.Auto.IntervalRange.Max,
	}

	if !sizeRange.IsValid() {
		sizeRange = optimizer.Range{Min: 10, Max: 60}
	}
	if !intervalRange.IsValid() {
		intervalRange = optimizer.Range{Min: 10, Max: 32}
	}

	results, err := opt.FindOptimalRanges(context.Background(), sizeRange, intervalRange)
	if err != nil {
		return nil, err
	}

	optimizer.PrintSummary(results)

	best := optimizer.GetBestResult(results)
	if best == nil {
		return nil, fmt.Errorf("no working fragment settings found")
	}

	return best, nil
}

// runICMPScan runs ICMP ping scan without xray-core
func runICMPScan(cfg *config.Config) error {
	s := scanner.NewICMPScanner(cfg)

	if err := s.LoadIPs(subnetsPath, maxIPs, shuffle); err != nil {
		return fmt.Errorf("failed to load IPs: %w", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, stopping...")
		s.Stop()
	}()

	if err := s.Run(); err != nil {
		return fmt.Errorf("ICMP scan failed: %w", err)
	}

	s.GetResults().PrintTopResults(topN)

	if s.GetResults().SuccessCount() > 0 {
		outputPath := scanner.GenerateOutputPath(outputFmt)
		if err := s.SaveResults(outputFmt, outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "%sWarning:%s failed to save results: %v\n", utils.Yellow, utils.Reset, err)
		} else {
			fmt.Printf("%sResults saved to:%s %s%s%s\n", utils.Gray, utils.Reset, utils.Cyan, outputPath, utils.Reset)
		}
	}

	return nil
}
