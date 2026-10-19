//go:build windows

package netroute

import (
	"encoding/binary"
	"net"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsUnicastInterfaceSocketOptions(t *testing.T) {
	name := loopback(t)
	i, e := net.InterfaceByName(name)
	if e != nil {
		t.Fatal(e)
	}
	for _, family := range []int{syscall.AF_INET, syscall.AF_INET6} {
		fd, e := syscall.Socket(family, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
		if e != nil {
			t.Fatal(e)
		}
		network, level := "tcp4", syscall.IPPROTO_IP
		if family == syscall.AF_INET6 {
			network, level = "tcp6", syscall.IPPROTO_IPV6
		}
		if e = bind(uintptr(fd), network, i); e != nil {
			syscall.Closesocket(fd)
			t.Fatal(e)
		}
		var value uint32
		size := int32(4)
		e = syscall.Getsockopt(fd, int32(level), 31, (*byte)(unsafe.Pointer(&value)), &size)
		syscall.Closesocket(fd)
		if e != nil {
			t.Fatal(e)
		}
		if family == syscall.AF_INET {
			bytes := *(*[4]byte)(unsafe.Pointer(&value))
			value = binary.BigEndian.Uint32(bytes[:])
		}
		if int(value) != i.Index {
			t.Fatalf("%s interface index %d, want %d", network, value, i.Index)
		}
	}
}
