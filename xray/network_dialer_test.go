package xray

import (
	"context"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport/internet"
	"testing"
)

func TestStrictXrayDialerDoesNotFallBackAfterAdapterDisappears(t *testing.T) {
	d := &boundSystemDialer{}
	for _, network := range []xnet.Network{xnet.Network_TCP, xnet.Network_UDP} {
		dest := xnet.Destination{Network: network, Address: xnet.ParseAddress("127.0.0.1"), Port: 80}
		c, e := d.Dial(context.Background(), nil, dest, &internet.SocketConfig{Interface: "scaner-no-such-network-564"})
		if c != nil {
			c.Close()
			t.Fatal("wrong route used")
		}
		if e == nil {
			t.Fatal("missing interface did not fail")
		}
	}
}
