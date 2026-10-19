//go:build linux && !android

package netroute

import (
	"golang.org/x/sys/unix"
	"net"
)

func Supported() bool                                   { return true }
func bind(fd uintptr, _ string, i *net.Interface) error { return unix.BindToDevice(int(fd), i.Name) }
func probe(i *net.Interface) error {
	fd, e := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	return bind(uintptr(fd), "tcp4", i)
}
