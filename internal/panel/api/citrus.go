package api

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"mikan/internal/panel/audit"
	"mikan/internal/panel/features"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/subs"
	"net/http"
	"strings"
)

type CitrusFeatureSettings features.Config
type citrusConfigOutput struct{ Body CitrusFeatureSettings }
type citrusConfigInput struct{ Body CitrusFeatureSettings }
type citrusGraceOption struct {
	ServerID    int64  `json:"server_id"`
	ServerName  string `json:"server_name"`
	InboundID   int64  `json:"inbound_id"`
	InboundName string `json:"inbound_name"`
}
type citrusGraceOptionsOutput struct{ Body []citrusGraceOption }
type citrusSourcesOutput struct{ Body []db.CitrusSource }
type citrusSourceInput struct {
	Body struct {
		ID        int64   `json:"id" minimum:"0"`
		Name      string  `json:"name" minLength:"1" maxLength:"64"`
		URL       string  `json:"url" maxLength:"4096"`
		Enabled   bool    `json:"enabled"`
		TariffIDs []int64 `json:"tariff_ids"`
		Interval  int64   `json:"interval_minutes" minimum:"5" maximum:"10080"`
	}
}

func (h *handlers) registerCitrus() {
	op := sessionOps("citrus")
	huma.Register(h.api, op("citrus-grace-options", http.MethodGet, "/api/v1/citrus/grace-options", "Серверы и подключения для продления", 0), func(ctx context.Context, _ *struct{}) (*citrusGraceOptionsOutput, error) {
		nodes, err := h.d.Store.Q.ListNodes(ctx)
		if err != nil {
			return nil, err
		}
		inbounds, err := h.d.Store.Q.ListInbounds(ctx)
		if err != nil {
			return nil, err
		}
		out := []citrusGraceOption{}
		for _, n := range nodes {
			if n.Enabled == 0 {
				continue
			}
			name := n.Name
			if strings.TrimSpace(name) == "" {
				name = n.PublicHost
				if name == "" {
					name = "Сервер панели"
				}
			}
			for _, in := range inbounds {
				if in.NodeID == n.ID && in.Enabled != 0 && subs.GraceProtocol(in) {
					out = append(out, citrusGraceOption{n.ID, name, in.ID, in.Name})
				}
			}
		}
		return &citrusGraceOptionsOutput{out}, nil
	})
	huma.Register(h.api, op("citrus-config", http.MethodGet, "/api/v1/citrus/settings", "Настройки Citrus Gate", 0), func(ctx context.Context, _ *struct{}) (*citrusConfigOutput, error) {
		c, e := features.Load(ctx, h.d.Settings)
		return &citrusConfigOutput{CitrusFeatureSettings(c)}, e
	})
	huma.Register(h.api, op("citrus-save", http.MethodPut, "/api/v1/citrus/settings", "Сохранить настройки Citrus Gate", 0), func(ctx context.Context, in *citrusConfigInput) (*citrusConfigOutput, error) {
		c := features.Config(in.Body)
		if e := c.Validate(); e != nil {
			return nil, huma.Error400BadRequest(e.Error())
		}
		if e := subs.ValidName(c.AutoName); e != nil {
			return nil, huma.Error400BadRequest("invalid_auto_name")
		}
		if c.GraceInbound > 0 {
			i, e := h.d.Store.Q.GetInbound(ctx, c.GraceInbound)
			if e != nil || i.Enabled == 0 || !subs.GraceProtocol(i) {
				return nil, huma.Error400BadRequest("grace_requires_enabled_personal_protocol")
			}
			n, e := h.d.Store.Q.GetNode(ctx, i.NodeID)
			if e != nil || n.Enabled == 0 {
				return nil, huma.Error400BadRequest("grace_requires_enabled_server")
			}
		}
		if e := settings.Set(ctx, h.d.Settings, features.Key, c); e != nil {
			return nil, e
		}
		h.d.Changes.PoliciesChanged()
		_ = audit.Write(ctx, h.d.Store.Q, h.d.Now(), audit.Entry{Action: "citrus.settings", TargetType: "settings"})
		return &citrusConfigOutput{CitrusFeatureSettings(c)}, nil
	})
	huma.Register(h.api, op("citrus-sources", http.MethodGet, "/api/v1/citrus/sources", "Внешние подписки", 0), func(ctx context.Context, _ *struct{}) (*citrusSourcesOutput, error) {
		s, e := h.d.Store.Q.CitrusSources(ctx)
		return &citrusSourcesOutput{s}, e
	})
	huma.Register(h.api, op("citrus-source-save", http.MethodPut, "/api/v1/citrus/sources", "Добавить или изменить внешнюю подписку", 0), func(ctx context.Context, in *citrusSourceInput) (*citrusSourcesOutput, error) {
		b := in.Body
		if strings.TrimSpace(b.Name) == "" || len(b.TariffIDs) > 1000 {
			return nil, huma.Error400BadRequest("invalid_source")
		}
		if e := subs.CheckExternalURL(b.URL); e != nil {
			return nil, huma.Error400BadRequest("public_https_url_required")
		}
		for _, id := range b.TariffIDs {
			if _, e := h.d.Store.Q.GetTariff(ctx, id); e != nil {
				return nil, huma.Error400BadRequest("tariff_not_found")
			}
		}
		if b.TariffIDs == nil {
			b.TariffIDs = []int64{}
		}
		ids, _ := json.Marshal(b.TariffIDs)
		var e error
		if b.ID == 0 {
			_, e = h.d.Store.DB.ExecContext(ctx, "INSERT INTO cg_external_sources(name,url,enabled,tariff_ids,interval_minutes) VALUES($1,$2,$3,$4,$5)", b.Name, b.URL, b.Enabled, string(ids), b.Interval)
		} else {
			_, e = h.d.Store.DB.ExecContext(ctx, "UPDATE cg_external_sources SET name=$2,url=$3,enabled=$4,tariff_ids=$5,interval_minutes=$6,attempted_at=0,body=CASE WHEN url=$3 THEN body ELSE '' END,fetched_at=CASE WHEN url=$3 THEN fetched_at ELSE 0 END WHERE id=$1", b.ID, b.Name, b.URL, b.Enabled, string(ids), b.Interval)
		}
		if e != nil {
			return nil, e
		}
		s, e := h.d.Store.Q.CitrusSources(ctx)
		return &citrusSourcesOutput{s}, e
	})
	type sourceID struct {
		ID int64 `path:"id" minimum:"1"`
	}
	huma.Register(h.api, op("citrus-source-delete", http.MethodDelete, "/api/v1/citrus/sources/{id}", "Удалить внешнюю подписку", 204), func(ctx context.Context, in *sourceID) (*struct{}, error) {
		_, e := h.d.Store.DB.ExecContext(ctx, "DELETE FROM cg_external_sources WHERE id=$1", in.ID)
		return nil, e
	})
	huma.Register(h.api, op("citrus-source-refresh", http.MethodPost, "/api/v1/citrus/sources/{id}/refresh", "Обновить внешнюю подписку", 0), func(ctx context.Context, in *sourceID) (*citrusSourcesOutput, error) {
		if e := subs.RefreshExternal(ctx, h.d.Store, in.ID, h.d.Now()); e != nil {
			return nil, huma.Error502BadGateway("external_refresh_failed")
		}
		s, e := h.d.Store.Q.CitrusSources(ctx)
		return &citrusSourcesOutput{s}, e
	})
}
