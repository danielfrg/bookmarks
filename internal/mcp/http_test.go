package mcp

import (
	"context"
	"net/http"
	"testing"
)

func TestParseAllowedHosts(t *testing.T) {
	got, err := ParseAllowedHosts(" A.example, [::1] ,10.0.0.1")
	if err != nil || len(got) != 3 || got[0] != "a.example" || got[1] != "::1" {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := ParseAllowedHosts("  "); got != nil || err != nil {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []string{"http://x.example", "x.example:80", "a,,b", "-x.example", "x/y"} {
		if _, err := ParseAllowedHosts(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestProtectRejectsUnknownHost(t *testing.T) {
	e := newEnv(t)
	if r := post(t, e.srv.URL, "evil.example", ""); r.StatusCode != 403 {
		t.Fatalf("status %d", r.StatusCode)
	}
	if r := post(t, e.srv.URL, "", ""); r.StatusCode != 200 {
		t.Fatalf("status %d", r.StatusCode)
	}
	resp, err := http.Get(e.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Fatalf("GET status %d", resp.StatusCode)
	}
}

func TestBearerMustVerify(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, good, err := e.svc.CreateToken(ctx, "mcp")
	if err != nil {
		t.Fatal(err)
	}
	info, gone, err := e.svc.CreateToken(ctx, "gone")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteToken(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	for _, auth := range []string{"Bearer bad", "Token bad", "Bearer ", "Bearer " + gone} {
		if r := post(t, e.srv.URL, "", auth); r.StatusCode != 401 {
			t.Errorf("%q: status %d", auth, r.StatusCode)
		}
	}
	for _, auth := range []string{"Bearer " + good, "token " + good, "", "Basic Zm9v"} {
		if r := post(t, e.srv.URL, "", auth); r.StatusCode != 200 {
			t.Errorf("%q: status %d", auth, r.StatusCode)
		}
	}
	e.st.Close()
	if r := post(t, e.srv.URL, "", "Bearer "+good); r.StatusCode != 503 {
		t.Fatalf("store down: status %d", r.StatusCode)
	}
}
