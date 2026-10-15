package subs

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestCitrusExternalRejectsPrivateTargets(t *testing.T) {
	for _, s := range []string{"http://example.com/s", "https://localhost/s", "https://127.0.0.1/s", "https://[::1]/s", "https://169.254.169.254/metadata", "https://100.64.0.1/s", "https://user:pass@example.com/s", "https://example.com:8080/s"} {
		if CheckExternalURL(s) == nil {
			t.Fatalf("admitted %s", s)
		}
	}
	if e := CheckExternalURL("https://example.com/sub?token=secret"); e != nil {
		t.Fatal(e)
	}
}
func TestCitrusExternalImportsAndStripsLocalResources(t *testing.T) {
	ps, e := parseExternal("proxies:\n  - name: Remote\n    type: trojan\n    server: example.com\n    port: 443\n    password: secret\n    dialer-proxy: DIRECT\n    ca-file: /private/key\n")
	if e != nil {
		t.Fatal(e)
	}
	if len(ps) != 1 || ps[0]["dialer-proxy"] != nil || ps[0]["ca-file"] != nil {
		t.Fatalf("unsafe proxy: %v", ps)
	}
	uri := "vless://aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa@example.com:443?security=tls&type=tcp#test"
	ps, e = parseExternal(base64.StdEncoding.EncodeToString([]byte(uri)))
	if e != nil || len(ps) != 1 {
		t.Fatalf("base64 import: %v", e)
	}
	if externalURI(ps[0], "My service") == "" {
		t.Fatal("no external share link")
	}
}
func TestCitrusXrayAutoContainsWorkingBalancer(t *testing.T) {
	p := profile(t, "")
	b, e := XrayAuto(p, "Быстрый сервер")
	if e != nil {
		t.Fatal(e)
	}
	var profiles []map[string]any
	if json.Unmarshal(b, &profiles) != nil || len(profiles) < 2 {
		t.Fatal("invalid subscription")
	}
	auto := profiles[0]
	if auto["remarks"] != "Быстрый сервер" || auto["observatory"] == nil {
		t.Fatal("auto profile missing")
	}
	bal := auto["routing"].(map[string]any)["balancers"].([]any)[0].(map[string]any)
	if bal["strategy"].(map[string]any)["type"] != "leastPing" || !strings.HasPrefix(bal["fallbackTag"].(string), "cg-") {
		t.Fatal("unsafe or missing fallback")
	}
	for _, manual := range profiles[1:] {
		if manual["observatory"] != nil {
			t.Fatal("manual profile got observatory")
		}
	}
	if Format("Happ/3", "", "xray") != "xray" || Format("Happ/3", "", "uri") != "uri" {
		t.Fatal("explicit format override lost")
	}
}
