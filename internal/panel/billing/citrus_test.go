package billing

import (
	"context"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/features"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"sync"
	"testing"
	"time"
)

func TestCitrusDevicePaymentReplayRenewAndRefund(t *testing.T) {
	e, u, _ := packageEnv(t)
	ctx := context.Background()
	must(t, settings.Set(ctx, e.s.d.Settings, features.Key, features.Config{DeviceStars: 25}))
	if _, err := e.s.DeviceInvoice(ctx, 999, u.ID, Stars); err != ErrNotYours {
		t.Fatalf("foreign purchase: %v", err)
	}
	p, err := e.s.DeviceInvoice(ctx, 555, u.ID, Stars)
	must(t, err)
	must(t, e.s.PreCheckout(ctx, 555, p.Payload, "XTR", 25))
	must(t, e.s.StarsPaid(ctx, 555, p.Payload, "cg-device", "XTR", 25))
	must(t, e.s.StarsPaid(ctx, 555, p.Payload, "cg-device", "XTR", 25))
	must(t, e.s.Apply(ctx, p.ID))
	after := e.user(u.ID)
	if after.DeviceLimit.Int64 != u.DeviceLimit.Int64+1 || after.ExpiresAt != u.ExpiresAt {
		t.Fatalf("bad device benefit: %+v", after)
	}
	e.buy(u.ID, "cg-renew")
	after = e.user(u.ID)
	if after.DeviceLimit.Int64 != e.sale.DeviceLimit.Int64+1 {
		t.Fatal("renewal lost bonus")
	}
	_, err = e.s.Refund(ctx, p.ID)
	must(t, err)
	after = e.user(u.ID)
	if after.DeviceLimit != e.sale.DeviceLimit {
		t.Fatal("refund did not remove device")
	}
}
func TestCitrusReferralFirstPaymentOnlyAndRefund(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	must(t, settings.Set(ctx, e.s.d.Settings, features.Key, features.Config{ReferralDays: 7}))
	inviter, err := e.s.d.Users.Create(ctx, domain.CreateInput{Name: "inviter", TariffID: e.sale.ID})
	must(t, err)
	must(t, e.st.Q.LinkTg(ctx, db.LinkTgParams{UserID: inviter.ID, TgID: 777, CreatedAt: e.now.Unix()}))
	code, err := e.s.ReferralCode(ctx, inviter.ID)
	must(t, err)
	must(t, e.s.RegisterReferral(ctx, 555, code))
	must(t, e.s.RegisterReferral(ctx, 777, code))
	p := e.invoice(555, 0, Stars)
	must(t, e.s.StarsPaid(ctx, 555, p.Payload, "cg-ref-first", p.Currency, p.Amount))
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- e.s.Apply(ctx, p.ID) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		must(t, err)
	}
	after := e.user(inviter.ID)
	if after.ExpiresAt.Int64-inviter.ExpiresAt.Int64 != int64((7*24*time.Hour)/time.Second) {
		t.Fatal("referral applied wrong number of times")
	}
	paid := e.payment(p.ID)
	e.buy(paid.UserID.Int64, "cg-ref-renew")
	if e.user(inviter.ID).ExpiresAt != after.ExpiresAt {
		t.Fatal("renewal rewarded twice")
	}
	_, err = e.s.Refund(ctx, p.ID)
	must(t, err)
	if e.user(inviter.ID).ExpiresAt != inviter.ExpiresAt {
		t.Fatal("refund retained referral bonus")
	}
	var self int
	must(t, e.st.DB.QueryRowContext(ctx, "SELECT count(*) FROM cg_referrals WHERE invitee=777").Scan(&self))
	if self != 0 {
		t.Fatal("self referral registered")
	}
}
