// Package netroute binds individual test sockets without changing system routes.
package netroute

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Interface struct {
	Name      string
	Index     int
	Addresses []string
}

func Interfaces() ([]Interface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := []Interface{}
	for _, i := range all {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, e := i.Addrs()
		if e != nil {
			continue
		}
		v := Interface{Name: i.Name, Index: i.Index}
		for _, a := range addrs {
			ip, _, e := net.ParseCIDR(a.String())
			if e == nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				v.Addresses = append(v.Addresses, ip.String())
			}
		}
		if len(v.Addresses) > 0 {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}
func (i Interface) Label() string { return i.Name + " · " + strings.Join(i.Addresses, ", ") }
func lookup(name string) (*net.Interface, error) {
	if !Supported() {
		return nil, fmt.Errorf("explicit network selection is supported on Windows and desktop Linux only; choose Automatic")
	}
	i, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("selected network %q is unavailable: %w", name, err)
	}
	if i.Flags&net.FlagUp == 0 {
		return nil, fmt.Errorf("selected network %q is disconnected", name)
	}
	return i, nil
}

// Validate opens no connection. Socket-option permissions are checked before a run.
func Validate(name string) error {
	if name == "" {
		return nil
	}
	i, e := lookup(name)
	if e != nil {
		return e
	}
	if e := probe(i); e != nil {
		return fmt.Errorf("cannot use selected network %q: %w; no automatic fallback", name, e)
	}
	return nil
}
func control(name string) func(string, string, syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		i, e := lookup(name)
		if e != nil {
			return e
		}
		var inner error
		if e = c.Control(func(fd uintptr) { inner = bind(fd, network, i) }); e != nil {
			return e
		}
		if inner != nil {
			return fmt.Errorf("cannot bind test socket to %q: %w (no automatic fallback)", name, inner)
		}
		return nil
	}
}

// DialContext never retries on another interface when an explicit one was selected.
// Go's resolver is given the same binding for its DNS sockets. System DNS server
// addresses are retained; a VPN-only DNS server may therefore be unreachable.
func DialContext(ctx context.Context, name, network, address string) (net.Conn, error) {
	d := net.Dialer{Timeout: 16 * time.Second}
	if name != "" {
		if _, e := lookup(name); e != nil {
			return nil, e
		}
		d.Control = control(name)
		d.Resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, n, a string) (net.Conn, error) {
			dns := net.Dialer{Timeout: 5 * time.Second, Control: control(name)}
			return dns.DialContext(ctx, n, a)
		}}
	}
	return d.DialContext(ctx, network, address)
}
