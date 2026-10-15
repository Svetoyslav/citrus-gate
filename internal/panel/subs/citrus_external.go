package subs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/metacubex/mihomo/common/convert"
	"go.yaml.in/yaml/v3"
	"io"
	"log/slog"
	"mikan/internal/panel/store"
	"mikan/internal/panel/store/db"
	"mikan/internal/proto"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type ExternalProxy struct {
	Name string
	Node int64
	URI  string
	YAML map[string]any
}

func GraceProtocol(in db.Inbound) bool {
	t, e := parseTemplate(in.Config)
	return e == nil && !proto.Shared(t.Type())
}
func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, s := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "2001:db8::/32"} {
		if netip.MustParsePrefix(s).Contains(ip) {
			return false
		}
	}
	return true
}
func CheckExternalURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(raw) > 4096 {
		return errors.New("invalid_url")
	}
	if u.Port() != "" && u.Port() != "443" {
		return errors.New("https_443_required")
	}
	if ip, e := netip.ParseAddr(u.Hostname()); e == nil && !publicIP(ip) {
		return errors.New("private_address")
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return errors.New("private_address")
	}
	return nil
}
func externalClient() *http.Client {
	tr := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(addr)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, errors.New("dns_failed")
		}
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, errors.New("private_address")
			}
		}
		d := net.Dialer{Timeout: 10 * time.Second}
		var last error
		for _, ip := range ips {
			c, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return c, nil
			}
			last = e
		}
		return nil, last
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("redirect_limit")
		}
		return CheckExternalURL(req.URL.String())
	}}
}
func fetchExternal(ctx context.Context, raw string) (string, error) {
	if e := CheckExternalURL(raw); e != nil {
		return "", e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return "", errors.New("invalid_url")
	}
	req.Header.Set("User-Agent", "ClashMeta/1.19.32 CitrusGate/1.0")
	c := externalClient()
	defer c.CloseIdleConnections()
	res, e := c.Do(req)
	if e != nil {
		return "", errors.New("fetch_failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("http_%d", res.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if e != nil || len(b) > 2<<20 {
		return "", errors.New("body_too_large")
	}
	ps, e := parseExternal(string(b))
	if e != nil {
		return "", e
	}
	clean, e := yaml.Marshal(map[string]any{"proxies": ps})
	return string(clean), e
}
func parseExternal(body string) ([]map[string]any, error) {
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	_ = yaml.Unmarshal([]byte(body), &doc)
	ps := doc.Proxies
	if len(ps) == 0 {
		var e error
		ps, e = convert.ConvertsV2Ray([]byte(body))
		if e != nil {
			return nil, errors.New("invalid_subscription")
		}
	}
	if len(ps) == 0 || len(ps) > 2000 {
		return nil, errors.New("invalid_proxy_count")
	}
	out := []map[string]any{}
	for _, p := range ps {
		typ := xstr(p["type"])
		if !slices.Contains([]string{"vless", "vmess", "trojan", "ss", "hysteria2", "tuic", "anytls", "snell", "mieru"}, typ) {
			continue
		}
		if xstr(p["server"]) == "" || xnum(p["port"]) < 1 || xnum(p["port"]) > 65535 {
			continue
		}
		// A provider may supply endpoint options, never client-side remote resources,
		// scripts, chains, providers or paths to local certificate/key files.
		for _, k := range []string{"dialer-proxy", "certificate", "private-key", "private-key-path", "certificate-path", "ca", "ca-str", "ca-file", "interface-name", "routing-mark"} {
			delete(p, k)
		}
		if xstr(p["name"]) == "" {
			p["name"] = fmt.Sprintf("Server %d", len(out)+1)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, errors.New("no_supported_proxies")
	}
	return out, nil
}
func RefreshExternal(ctx context.Context, st *store.Store, id int64, now time.Time) error {
	sources, e := st.Q.CitrusSources(ctx)
	if e != nil {
		return e
	}
	for _, s := range sources {
		if s.ID != id {
			continue
		}
		body, err := fetchExternal(ctx, s.URL)
		code := ""
		if err != nil {
			code = err.Error()
		}
		if err == nil {
			_, e = st.DB.ExecContext(ctx, "UPDATE cg_external_sources SET body=$2,fetched_at=$3,attempted_at=$3,error='' WHERE id=$1 AND url=$4", id, body, now.Unix(), s.URL)
		} else {
			_, e = st.DB.ExecContext(ctx, "UPDATE cg_external_sources SET attempted_at=$2,error=$3 WHERE id=$1 AND url=$4", id, now.Unix(), code, s.URL)
		}
		if e != nil {
			return e
		}
		return err
	}
	return errors.New("source_not_found")
}
func RunExternal(ctx context.Context, st *store.Store, now func() time.Time, log *slog.Logger) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		ss, e := st.Q.CitrusSources(ctx)
		if e == nil {
			for _, s := range ss {
				if s.Enabled && now().Unix()-s.AttemptedAt >= s.Interval*60 {
					if err := RefreshExternal(ctx, st, s.ID, now()); err != nil {
						log.Warn("external subscription refresh", "source", s.ID, "code", err.Error())
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (h *Handler) addExternal(ctx context.Context, p *Profile, u db.User, app App) error {
	sources, e := h.st.Q.CitrusSources(ctx)
	if e != nil {
		return e
	}
	for _, s := range sources {
		if !s.Enabled || s.FetchedAt == 0 || h.now().Unix()-s.FetchedAt > 86400 {
			continue
		}
		var ids []int64
		if json.Unmarshal([]byte(s.TariffIDs), &ids) != nil {
			continue
		}
		if len(ids) > 0 && (!u.TariffID.Valid || !slices.Contains(ids, u.TariffID.Int64)) {
			continue
		}
		ps, e := parseExternal(s.Body)
		if e != nil {
			continue
		}
		for _, y := range ps {
			typ := xstr(y["type"])
			needType := typ
			if typ == "ss" {
				needType = "shadowsocks"
			}
			if !app.Supports(proto.Needs{Type: needType, Transport: xstr(y["network"])}) {
				continue
			}
			name := s.Name + " · " + xstr(y["name"])
			uri := externalURI(y, name)
			if app.Family != FamilyMihomo && app.Family != FamilyStash && uri == "" {
				continue
			}
			p.External = append(p.External, ExternalProxy{Name: name, Node: -s.ID, URI: uri, YAML: y})
		}
	}
	return nil
}
func xstr(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func xnum(v any) int { n, _ := strconv.Atoi(xstr(v)); return n }
func xmap(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
