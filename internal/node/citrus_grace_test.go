package node

import (
	C "github.com/metacubex/mihomo/constant"
	"mikan/internal/nodeapi"
	"net/netip"
	"testing"
	"time"
)

func TestCitrusGraceDestinationEnforced(t *testing.T) {
	now := time.Unix(100, 0)
	r := NewRegistry("e", 0, time.Minute, func() time.Time { return now })
	r.SetSlots([]nodeapi.Slot{{Name: "s", UUID: "u"}})
	r.SetPolicies("e", []nodeapi.Policy{{Slot: "s", Allowed: true, QuotaRemaining: -1, GraceUntil: 200, Inbounds: []string{"renew"}}})
	m := &C.Metadata{DstIP: netip.MustParseAddr("149.154.167.51"), DstPort: 443}
	if !r.graceAllows("u", m) {
		t.Fatal("Telegram rejected")
	}
	m.DstIP = netip.MustParseAddr("1.1.1.1")
	m.Host = "telegram.org"
	if r.graceAllows("u", m) {
		t.Fatal("forged hostname admitted")
	}
	m.Host = ""
	if r.graceAllows("u", m) {
		t.Fatal("unrelated IP admitted")
	}
	m.DstIP = netip.MustParseAddr("149.154.167.51")
	m.DstPort = 80
	if r.graceAllows("u", m) {
		t.Fatal("unapproved port admitted")
	}
	m.DstPort = 443
	now = time.Unix(200, 0)
	if r.graceAllows("u", m) {
		t.Fatal("expired grace admitted")
	}
}
