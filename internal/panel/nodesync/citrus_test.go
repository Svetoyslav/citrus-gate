package nodesync

import (
	"context"
	"mikan/internal/nodeapi"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/features"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/proto"
	"slices"
	"testing"
)

func TestCitrusGraceDurationAndSelectedServer(t *testing.T) {
	local, _, st, users, now := setup(t)
	remote, _ := addRemote(t, local, st)
	ctx := context.Background()
	tariffs, err := st.Q.ListTariffs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(ctx, domain.CreateInput{Name: "renewal", TariffID: tariffs[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	expired := now.Unix() - 2*86400
	if _, err = st.DB.ExecContext(ctx, "UPDATE users SET expires_at=$1 WHERE id=$2", expired, u.ID); err != nil {
		t.Fatal(err)
	}
	ins, err := st.Q.ListInbounds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var chosen, own db.Inbound
	for _, in := range ins {
		template, e := proto.Parse(in.Config)
		if e == nil && !proto.Shared(template.Type()) && in.Enabled != 0 {
			if in.NodeID == remote.id {
				chosen = in
			}
			if in.NodeID == local.id {
				own = in
			}
		}
	}
	if chosen.ID == 0 || own.ID == 0 {
		t.Fatal("missing personal protocol")
	}
	set := settings.New(st.Q)
	// Old saved settings retain one day, explicit zero remains disabled after upgrade.
	if err = settings.Set(ctx, set, features.Key, map[string]any{"grace_inbound": chosen.ID}); err != nil {
		t.Fatal(err)
	}
	c, err := features.Load(ctx, set)
	if err != nil || c.GraceDays != 1 {
		t.Fatalf("legacy default: %+v %v", c, err)
	}
	policy := func(s *Syncer) nodeapi.Policy {
		t.Helper()
		_, ps, owners, e := s.policies(ctx)
		if e != nil {
			t.Fatal(e)
		}
		for _, p := range ps {
			if owners[p.Slot] == u.ID {
				return p
			}
		}
		t.Fatal("missing policy")
		return nodeapi.Policy{}
	}
	save := func(c features.Config) {
		t.Helper()
		if e := settings.Set(ctx, set, features.Key, c); e != nil {
			t.Fatal(e)
		}
		local.m.PoliciesChanged()
	}
	c.GraceDays = 5
	save(c)
	p := policy(remote)
	if !p.Allowed || p.GraceUntil != expired+5*86400 || !slices.Equal(p.Inbounds, []string{chosen.Name}) || policy(local).Allowed {
		t.Fatalf("wrong selected-server policy: %+v", p)
	}
	c.GraceInbound = own.ID
	save(c)
	if !policy(local).Allowed || policy(remote).Allowed {
		t.Fatal("server change did not move access")
	}
	c.GraceDays = 1
	save(c)
	if policy(local).Allowed || policy(remote).Allowed {
		t.Fatal("shortened window still allows access")
	}
	c.GraceDays = 0
	save(c)
	loaded, err := features.Load(ctx, set)
	if err != nil || loaded.GraceDays != 0 || policy(local).Allowed {
		t.Fatal("zero must disable access")
	}
	c.GraceDays = 5
	c.GraceInbound = chosen.ID
	save(c)
	if _, err = st.DB.ExecContext(ctx, "UPDATE nodes SET enabled=0 WHERE id=$1", remote.id); err != nil {
		t.Fatal(err)
	}
	local.m.PoliciesChanged()
	if policy(remote).Allowed || policy(local).Allowed {
		t.Fatal("disabled selected server admitted renewal access")
	}
}
