package citrusbot

import (
	"context"
	"log/slog"
	"mikan/internal/panel/billing"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/features"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/storetest"
	"mikan/internal/panel/tgate"
	"testing"
	"time"
)

type noChanges struct{}

func (noChanges) PoliciesChanged() {}
func (noChanges) SlotsChanged()    {}
func TestCitrusAdminConfirmationBoundToAccountAndSingleUse(t *testing.T) {
	ctx := context.Background()
	st, e := storetest.Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	now := time.Now()
	clock := func() time.Time { return now }
	if e := domain.Seed(ctx, st, now); e != nil {
		t.Fatal(e)
	}
	set := settings.New(st.Q)
	if e := settings.Set(ctx, set, features.Key, features.Config{AdminIDs: []int64{111}}); e != nil {
		t.Fatal(e)
	}
	users := domain.NewUsers(st, domain.NewPool(st, clock), noChanges{}, clock)
	ts, _ := st.Q.ListTariffs(ctx)
	u, e := users.Create(ctx, domain.CreateInput{Name: "customer", TariffID: ts[0].ID})
	if e != nil {
		t.Fatal(e)
	}
	g := tgate.New(tgate.Deps{Store: st, Settings: set, Now: clock, Log: slog.Default()})
	h := New(st, set, g, (*billing.Service)(nil), users, nil, nil, clock, slog.Default())
	h.pending["nonce"] = confirmation{111, "askextend", u.ID, now.Add(time.Minute)}
	send := func(id int64) {
		t.Helper()
		up := tgate.Update{CallbackQuery: &tgate.CallbackQuery{From: tgate.User{ID: id}, Message: &tgate.Message{Chat: tgate.Chat{ID: id, Type: "private"}}, Data: "cg:confirm:nonce"}}
		if e := h.Update(ctx, up); e != nil {
			t.Fatal(e)
		}
	}
	send(222)
	before, _ := st.Q.GetUser(ctx, u.ID)
	if before.ExpiresAt != u.ExpiresAt {
		t.Fatal("non-admin modified user")
	}
	send(111)
	after, _ := st.Q.GetUser(ctx, u.ID)
	if after.ExpiresAt.Int64-before.ExpiresAt.Int64 != 7*86400 {
		t.Fatal("admin extension failed")
	}
	send(111)
	again, _ := st.Q.GetUser(ctx, u.ID)
	if again.ExpiresAt != after.ExpiresAt {
		t.Fatal("confirmation reused")
	}
}
