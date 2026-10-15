package node

import (
	C "github.com/metacubex/mihomo/constant"
	"net/netip"
)

// Telegram publishes its current address ranges at https://core.telegram.org/resources/cidr.txt.
// Renewal access deliberately admits IP destinations only. Trusting a client-supplied
// hostname would let it pair telegram.org with an unrelated destination IP.
var telegramNets = []netip.Prefix{
	netip.MustParsePrefix("91.108.56.0/22"), netip.MustParsePrefix("91.108.4.0/22"),
	netip.MustParsePrefix("91.108.8.0/22"), netip.MustParsePrefix("91.108.16.0/22"),
	netip.MustParsePrefix("91.108.12.0/22"), netip.MustParsePrefix("149.154.160.0/20"),
	netip.MustParsePrefix("91.105.192.0/23"), netip.MustParsePrefix("91.108.20.0/22"),
	netip.MustParsePrefix("185.76.151.0/24"), netip.MustParsePrefix("2001:b28:f23d::/48"),
	netip.MustParsePrefix("2001:b28:f23f::/48"), netip.MustParsePrefix("2001:67c:4e8::/48"),
	netip.MustParsePrefix("2001:b28:f23c::/48"), netip.MustParsePrefix("2a0a:f280::/32"),
}

func (r *Registry) graceAllows(user string, m *C.Metadata) bool {
	s := r.lookup(user)
	if s == nil {
		return true
	} // normal admission handles unknown keys
	until := s.graceUntil.Load()
	if until == 0 {
		return true
	}
	if r.now().Unix() >= until || !m.DstIP.IsValid() || m.Host != "" || m.DstPort != 443 {
		return false
	}
	for _, p := range telegramNets {
		if p.Contains(m.DstIP.Unmap()) {
			return true
		}
	}
	return false
}
