package utils

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// ParseIPsFromFile reads IP addresses and CIDR blocks from a file
func ParseIPsFromFile(path string, sampleSize int, shuffle bool) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var ips []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.Contains(line, "/") {
			subnetIPs, err := ExpandCIDR(line, sampleSize)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: invalid CIDR %s: %v\n", line, err)
				continue
			}
			ips = append(ips, subnetIPs...)
		} else {
			if net.ParseIP(line) != nil {
				ips = append(ips, line)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	if shuffle {
		ShuffleIPs(ips)
	}

	return ips, nil
}

// ExpandCIDR expands a CIDR block to individual IP addresses.
// Optionally samples random IPs if sampleSize > 0.
//
// For large blocks with sampling requested it picks random addresses WITHOUT
// enumerating the whole range — enumerating e.g. a /8 (16M) or /1 (1B) would
// exhaust memory and crash.
func ExpandCIDR(cidr string, sampleSize int) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones

	// Big range + sampling -> random sample, no full enumeration.
	if sampleSize > 0 && hostBits > 16 {
		seen := make(map[string]struct{}, sampleSize)
		ips := make([]string, 0, sampleSize)
		for attempts := 0; len(ips) < sampleSize && attempts < sampleSize*16; attempts++ {
			r := randomIPInCIDR(ipnet)
			if v4 := r.To4(); v4 != nil && ones >= 24 && (v4[3] == 0 || v4[3] == 255) {
				continue
			}
			s := r.String()
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			ips = append(ips, s)
		}
		return ips, nil
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
		// Skip network and broadcast addresses for typical subnets
		if ones >= 24 && bits == 32 {
			lastOctet := ip[len(ip)-1]
			if lastOctet == 0 || lastOctet == 255 {
				continue
			}
		}
		ips = append(ips, ip.String())
	}

	if sampleSize > 0 && sampleSize < len(ips) {
		return SampleIPs(ips, sampleSize), nil
	}

	return ips, nil
}

// randomIPInCIDR returns a random address inside ipnet by randomizing the host
// bits (works for both IPv4 and IPv6).
func randomIPInCIDR(ipnet *net.IPNet) net.IP {
	ip := make(net.IP, len(ipnet.IP))
	for i := range ip {
		mask := ipnet.Mask[i]
		ip[i] = (ipnet.IP[i] & mask) | (byte(rand.Intn(256)) &^ mask)
	}
	return ip
}

// CIDRHostCount returns the number of addresses in a CIDR without enumerating
// them. ok is false if cidr is not a valid CIDR.
func CIDRHostCount(cidr string) (uint64, bool) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, false
	}
	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones
	if hostBits >= 63 {
		return ^uint64(0), true
	}
	return uint64(1) << uint(hostBits), true
}

// CountRanges cheaply estimates how many IPs a range block would queue (after
// per-subnet sampling), without expanding anything. Used for the live TUI
// preview so typing a broad CIDR never triggers a huge expansion.
func CountRanges(text string, sampleSize int) (int, bool) {
	const cap = 1_000_000_000
	total := 0
	any := false
	for _, tok := range tokenizeRanges(text) {
		if strings.Contains(tok, "/") {
			n, ok := CIDRHostCount(tok)
			if !ok {
				continue
			}
			any = true
			c := n
			if sampleSize > 0 && uint64(sampleSize) < n {
				c = uint64(sampleSize)
			}
			if c > cap {
				c = cap
			}
			total += int(c)
			if total > cap {
				total = cap
			}
		} else if net.ParseIP(tok) != nil {
			any = true
			total++
		}
	}
	return total, any
}

// tokenizeRanges splits a free-form range block into IP / CIDR tokens.
//
// Text copied from chats and web pages often contains invisible direction
// marks (U+200E/U+200F), zero-width characters, Persian/Arabic digits, Persian
// commas or labels next to the address. Those used to make every range except
// the first one invalid, so only one range was scanned. They are normalized here.
// Start-end ranges (1.2.3.0-1.2.3.255) are converted to CIDR blocks.
func tokenizeRanges(text string) []string {
	text = normalizeRangeText(text)
	text = dashRangeRe.ReplaceAllString(text, "$1-$2")
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i] // comment until end of line
		}
		fields := strings.FieldsFunc(line, func(r rune) bool {
			return unicode.IsSpace(r) || r == ',' || r == ';' || r == '|' || r == '،' || r == '؛'
		})
		for _, tok := range fields {
			tok = strings.Trim(strings.TrimSpace(tok), "\"'`()[]{}<>")
			if tok != "" {
				out = append(out, splitRangeToken(tok)...)
			}
		}
	}
	return out
}

var (
	dashRangeRe = regexp.MustCompile(`(\d{1,3}(?:\.\d{1,3}){3})\s*[-–—]\s*(\d{1,3}(?:\.\d{1,3}){3})`)
	ipv4TokRe   = regexp.MustCompile(`\d{1,3}(?:\.\d{1,3}){3}(?:/\d{1,2})?`)
)

func normalizeRangeText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch {
		case r >= '۰' && r <= '۹':
			r = '0' + (r - '۰')
		case r >= '٠' && r <= '٩':
			r = '0' + (r - '٠')
		case r == '٫' || r == '。' || r == '．':
			r = '.'
		case r == '／':
			r = '/'
		case r == '\uFEFF' || unicode.Is(unicode.Cf, r):
			r = ' '
		}
		if r == '\n' {
		} else if !unicode.IsSpace(r) {
		}
		b.WriteRune(r)
	}
	return b.String()
}

// splitRangeToken returns the usable IP/CIDR tokens inside tok. Invalid
// tokens are returned unchanged so callers can count them as invalid.
func splitRangeToken(tok string) []string {
	if isIPOrCIDR(tok) {
		return []string{tok}
	}
	if i := strings.IndexAny(tok, "-–—"); i > 0 {
		if cidrs, ok := rangeToCIDRs(tok[:i], strings.TrimLeft(tok[i:], "-–—")); ok {
			return cidrs
		}
	}
	if found := ipv4TokRe.FindAllString(tok, -1); len(found) > 0 {
		var out []string
		for _, f := range found {
			if isIPOrCIDR(f) {
				out = append(out, f)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{tok}
}

func isIPOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	return net.ParseIP(s) != nil
}

// rangeToCIDRs converts an inclusive IPv4 start-end range to CIDR blocks.
func rangeToCIDRs(a, b string) ([]string, bool) {
	s, e := net.ParseIP(strings.TrimSpace(a)).To4(), net.ParseIP(strings.TrimSpace(b)).To4()
	if s == nil || e == nil {
		return nil, false
	}
	start := uint64(binary.BigEndian.Uint32(s))
	end := uint64(binary.BigEndian.Uint32(e))
	if start > end {
		return nil, false
	}
	var out []string
	for start <= end {
		size := uint(0)
		for size < 32 {
			block := uint64(1) << (size + 1)
			if start%block != 0 || start+block-1 > end {
				break
			}
			size++
		}
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, uint32(start))
		out = append(out, fmt.Sprintf("%s/%d", ip, 32-size))
		start += uint64(1) << size
	}
	return out, true
}

// incrementIP increments an IP address by 1
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// SampleIPs randomly samples n IPs from the list
func SampleIPs(ips []string, n int) []string {
	if n >= len(ips) {
		return ips
	}

	result := make([]string, len(ips))
	copy(result, ips)

	for i := 0; i < n; i++ {
		j := i + rand.Intn(len(result)-i)
		result[i], result[j] = result[j], result[i]
	}

	return result[:n]
}

// ShuffleIPs shuffles the IP list in place
func ShuffleIPs(ips []string) {
	rand.Shuffle(len(ips), func(i, j int) {
		ips[i], ips[j] = ips[j], ips[i]
	})
}

// ParseCIDRList parses a comma-separated list of CIDRs
func ParseCIDRList(input string, sampleSize int) ([]string, error) {
	var ips []string
	cidrs := strings.Split(input, ",")

	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}

		if strings.Contains(cidr, "/") {
			subnetIPs, err := ExpandCIDR(cidr, sampleSize)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %s: %w", cidr, err)
			}
			ips = append(ips, subnetIPs...)
		} else {
			if net.ParseIP(cidr) != nil {
				ips = append(ips, cidr)
			} else {
				return nil, fmt.Errorf("invalid IP: %s", cidr)
			}
		}
	}

	return ips, nil
}

// ParseIPsFromText parses a free-form block of IPs/CIDRs (separated by newlines,
// commas, or whitespace), expanding CIDRs and skipping invalid tokens. Used by
// the TUI range tab where the user pastes ranges directly.
func ParseIPsFromText(text string, sampleSize int, shuffle bool) ([]string, error) {
	var ips []string
	for _, tok := range tokenizeRanges(text) {
		if strings.Contains(tok, "/") {
			subnetIPs, err := ExpandCIDR(tok, sampleSize)
			if err != nil {
				continue
			}
			ips = append(ips, subnetIPs...)
		} else if net.ParseIP(tok) != nil {
			ips = append(ips, tok)
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no valid IPs or CIDRs found")
	}

	if shuffle {
		ShuffleIPs(ips)
	}
	return ips, nil
}

// IsValidIP checks if a string is a valid IP address
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsValidCIDR checks if a string is a valid CIDR block
func IsValidCIDR(cidr string) bool {
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}
