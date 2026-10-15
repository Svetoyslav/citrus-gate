package app

import (
	"encoding/json"
	"mikan/internal/panel/features"
	"net/http"
	"testing"
)

func TestCitrusSettingsRequiresSessionAndCSRF(t *testing.T) {
	h := newHarness(t)
	path := "/" + adminPath + "/api/v1/citrus/settings"
	resp, _ := h.do(http.MethodGet, path, nil, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("anonymous settings %d", resp.StatusCode)
	}
	if r, _ := h.login(password, ""); r.StatusCode != 200 {
		t.Fatal("login")
	}
	c := features.Config{ReferralDays: 5, DeviceStars: 30, GraceDays: 5, AutoName: "Выбор", AdminIDs: []int64{123}}
	resp, _ = h.do(http.MethodPut, path, c, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("CSRF missing %d", resp.StatusCode)
	}
	resp, body := h.do(http.MethodPut, path, c, map[string]string{"X-CSRF-Token": h.csrf})
	if resp.StatusCode != 200 {
		t.Fatalf("save %d %s", resp.StatusCode, body)
	}
	resp, body = h.do(http.MethodGet, path, nil, nil)
	var got features.Config
	if resp.StatusCode != 200 || json.Unmarshal(body, &got) != nil || got.ReferralDays != 5 || got.GraceDays != 5 {
		t.Fatalf("round trip %d %s", resp.StatusCode, body)
	}
	c.GraceDays = -1
	resp, _ = h.do(http.MethodPut, path, c, map[string]string{"X-CSRF-Token": h.csrf})
	if resp.StatusCode != 400 {
		t.Fatalf("negative grace accepted %d", resp.StatusCode)
	}
	c.GraceDays = 5
	c.GraceInbound = 999999
	resp, _ = h.do(http.MethodPut, path, c, map[string]string{"X-CSRF-Token": h.csrf})
	if resp.StatusCode != 400 {
		t.Fatalf("invalid grace accepted %d", resp.StatusCode)
	}
}
