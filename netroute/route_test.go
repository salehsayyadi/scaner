package netroute

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func loopback(t *testing.T) string {
	t.Helper()
	all, e := net.Interfaces()
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range all {
		if i.Flags&net.FlagUp != 0 && i.Flags&net.FlagLoopback != 0 {
			return i.Name
		}
	}
	t.Skip("no loopback adapter")
	return ""
}
func TestMissingAdapterNeverFallsBack(t *testing.T) {
	if Validate("") != nil {
		t.Fatal("automatic must stay compatible")
	}
	name := "scaner-no-such-network-564"
	if Validate(name) == nil {
		t.Fatal("missing adapter accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, e := DialContext(ctx, name, "tcp", "127.0.0.1:80")
	if c != nil {
		c.Close()
		t.Fatal("connected through fallback")
	}
	if e == nil {
		t.Fatal("missing adapter silently fell back")
	}
}
func TestExplicitAdapterLocalTCPAndUDP(t *testing.T) {
	if !Supported() {
		t.Skip("platform binding unsupported")
	}
	name := loopback(t)
	if e := Validate(name); e != nil {
		t.Fatal(e)
	}
	for _, network := range []string{"tcp4", "udp4", "tcp6", "udp6"} {
		t.Run(network, func(t *testing.T) {
			listenAddr := "127.0.0.1:0"
			if strings.HasSuffix(network, "6") {
				listenAddr = "[::1]:0"
			}
			var addr string
			var closeServer func()
			if strings.HasPrefix(network, "tcp") {
				ln, e := net.Listen(network, listenAddr)
				if e != nil {
					t.Fatal(e)
				}
				addr = ln.Addr().String()
				closeServer = func() { ln.Close() }
				go func() {
					c, e := ln.Accept()
					if e == nil {
						defer c.Close()
						io.Copy(c, c)
					}
				}()
			} else {
				pc, e := net.ListenPacket(network, listenAddr)
				if e != nil {
					t.Fatal(e)
				}
				addr = pc.LocalAddr().String()
				closeServer = func() { pc.Close() }
				go func() {
					b := make([]byte, 100)
					n, a, e := pc.ReadFrom(b)
					if e == nil {
						pc.WriteTo(b[:n], a)
					}
				}()
			}
			defer closeServer()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			c, e := DialContext(ctx, name, network, addr)
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(time.Second))
			if _, e = c.Write([]byte("564")); e != nil {
				t.Fatal(e)
			}
			b := make([]byte, 3)
			if _, e = io.ReadFull(c, b); e != nil || string(b) != "564" {
				t.Fatalf("bound socket failed: %s %v", b, e)
			}
		})
	}
}
func TestInterfaceListExcludesDisconnectedAndLoopback(t *testing.T) {
	all, e := Interfaces()
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range all {
		i, e := net.InterfaceByName(a.Name)
		if e != nil || i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || len(a.Addresses) == 0 {
			t.Fatalf("invalid adapter: %+v", a)
		}
	}
}
