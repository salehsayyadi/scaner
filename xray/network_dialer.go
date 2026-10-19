package xray

import (
	"context"
	"fmt"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport/internet"
	"net"
	"piyazche/netroute"
)

// Installed once, before any workers start. Choice belongs to each outbound's
// immutable sockopt, not a global "current interface" shared by running scans.
func init() { internet.UseAlternativeSystemDialer(&boundSystemDialer{}) }

type boundSystemDialer struct{ fallback internet.DefaultSystemDialer }

func (d *boundSystemDialer) DestIpAddress() net.IP { return nil }
func (d *boundSystemDialer) Dial(ctx context.Context, src xnet.Address, dest xnet.Destination, opt *internet.SocketConfig) (net.Conn, error) {
	if opt == nil || opt.Interface == "" {
		return d.fallback.Dial(ctx, src, dest, opt)
	}
	switch dest.Network {
	case xnet.Network_TCP, xnet.Network_UDP:
	default:
		return nil, fmt.Errorf("selected network: unsupported transport %s", dest.Network)
	}
	// Unlike upstream v1.8.24's sockopt handler, binding failures are returned,
	// never merely logged while a socket continues on the default/VPN route.
	return netroute.DialContext(ctx, opt.Interface, dest.Network.SystemString(), dest.NetAddr())
}
