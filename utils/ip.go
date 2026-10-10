package utils

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
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

// tokenizeRanges splits a free-form range block into tokens.
func tokenizeRanges(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t' || r == ';'
	})
	out := fields[:0]
	for _, tok := range fields {
		tok = strings.TrimSpace(tok)
		if tok == "" || strings.HasPrefix(tok, "#") {
			continue
		}
		out = append(out, tok)
	}
	return out
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
