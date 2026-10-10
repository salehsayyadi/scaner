package utils

import "fmt"

// ANSI color codes
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Dim    = "\033[2m"
	Italic = "\033[3m"

	// Colors
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Gray    = "\033[90m"

	// Bright colors
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"

	// Background colors
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
)

// Color helper functions
func ColorRed(s string) string {
	return Red + s + Reset
}

func ColorGreen(s string) string {
	return Green + s + Reset
}

func ColorYellow(s string) string {
	return Yellow + s + Reset
}

func ColorBlue(s string) string {
	return Blue + s + Reset
}

func ColorCyan(s string) string {
	return Cyan + s + Reset
}

func ColorMagenta(s string) string {
	return Magenta + s + Reset
}

func ColorGray(s string) string {
	return Gray + s + Reset
}

func ColorBold(s string) string {
	return Bold + s + Reset
}

func ColorBoldCyan(s string) string {
	return Bold + Cyan + s + Reset
}

func ColorBoldGreen(s string) string {
	return Bold + Green + s + Reset
}

func ColorBoldYellow(s string) string {
	return Bold + Yellow + s + Reset
}

func ColorBoldRed(s string) string {
	return Bold + Red + s + Reset
}

func ColorBoldBlue(s string) string {
	return Bold + Blue + s + Reset
}

func ColorBoldMagenta(s string) string {
	return Bold + Magenta + s + Reset
}

// PrintHeader prints a styled header
func PrintHeader(title string) {
	line := "═══════════════════════════════════════════════════════════"
	fmt.Printf("%s%s%s\n", Cyan, line, Reset)
	fmt.Printf("%s%s  %s%s\n", Bold, Cyan, title, Reset)
	fmt.Printf("%s%s%s\n", Cyan, line, Reset)
}

// PrintSubHeader prints a styled sub-header
func PrintSubHeader(title string) {
	fmt.Printf("\n%s%s▸ %s%s\n", Bold, Yellow, title, Reset)
}

// PrintKeyValue prints a key-value pair with colors
func PrintKeyValue(key, value string) {
	fmt.Printf("  %s%-18s%s %s%s%s\n", Gray, key+":", Reset, White, value, Reset)
}

// PrintKeyValueColor prints a key-value pair with custom value color
func PrintKeyValueColor(key, value, color string) {
	fmt.Printf("  %s%-18s%s %s%s%s\n", Gray, key+":", Reset, color, value, Reset)
}

// PrintSuccess prints a success message
func PrintSuccess(msg string) {
	fmt.Printf("%s✓%s %s\n", Green, Reset, msg)
}

// PrintError prints an error message
func PrintError(msg string) {
	fmt.Printf("%s✗%s %s\n", Red, Reset, msg)
}

// PrintWarning prints a warning message
func PrintWarning(msg string) {
	fmt.Printf("%sWarning:%s %s\n", Yellow, Reset, msg)
}

// PrintInfo prints an info message
func PrintInfo(msg string) {
	fmt.Printf("%sℹ%s %s\n", Blue, Reset, msg)
}

// PrintDebug prints a debug message
func PrintDebug(msg string) {
	fmt.Printf("%s[DEBUG]%s %s\n", Magenta, Reset, msg)
}

// Box drawing characters
const (
	BoxTopLeft     = "╭"
	BoxTopRight    = "╮"
	BoxBottomLeft  = "╰"
	BoxBottomRight = "╯"
	BoxHorizontal  = "─"
	BoxVertical    = "│"
)

// PrintBox prints text in a colored box
func PrintBox(title string, lines []string, color string) {
	maxLen := len(title)
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	width := maxLen + 4

	// Top border
	fmt.Printf("%s%s", color, BoxTopLeft)
	for i := 0; i < width; i++ {
		fmt.Print(BoxHorizontal)
	}
	fmt.Printf("%s%s\n", BoxTopRight, Reset)

	// Title
	fmt.Printf("%s%s%s %s%-*s %s%s%s\n", color, BoxVertical, Reset, Bold+color, maxLen, title, Reset, color, BoxVertical+Reset)

	// Separator
	fmt.Printf("%s├", color)
	for i := 0; i < width; i++ {
		fmt.Print(BoxHorizontal)
	}
	fmt.Printf("┤%s\n", Reset)

	// Content lines
	for _, line := range lines {
		fmt.Printf("%s%s%s  %-*s  %s%s%s\n", color, BoxVertical, Reset, maxLen, line, color, BoxVertical, Reset)
	}

	// Bottom border
	fmt.Printf("%s%s", color, BoxBottomLeft)
	for i := 0; i < width; i++ {
		fmt.Print(BoxHorizontal)
	}
	fmt.Printf("%s%s\n", BoxBottomRight, Reset)
}
