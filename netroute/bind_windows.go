//go:build windows

package netroute

import (
	"encoding/binary"
	"net"
	"strings"
	"syscall"
	"unsafe"
)

func Supported() bool { return true }
func bind(fd uintptr, network string, i *net.Interface) error {
	if strings.HasSuffix(network, "4") {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(i.Index))
		index := *(*uint32)(unsafe.Pointer(&b[0]))
		return syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, 31, int(index))
	}
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IPV6, 31, i.Index)
}
func probe(i *net.Interface) error {
	fd, e := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if e != nil {
		return e
	}
	defer syscall.Closesocket(fd)
	return bind(uintptr(fd), "tcp4", i)
}
