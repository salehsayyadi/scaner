//go:build (!windows && !linux) || android

package netroute

import (
	"fmt"
	"net"
)

func Supported() bool { return false }
func bind(uintptr, string, *net.Interface) error {
	return fmt.Errorf("interface binding is unsupported on this platform")
}
func probe(*net.Interface) error {
	return fmt.Errorf("interface binding is unsupported on this platform")
}
