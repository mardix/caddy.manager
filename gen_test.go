package main

import (
	"os"
	"testing"
)

func TestGenerateValidates(t *testing.T) {
	st := State{
		AccessLists: []AccessList{{ID: "aaaaaaaaaaaa", Name: "Family", Users: []User{{Name: "alice", Hash: "$2a$12$C6UzMDM.H6dfI/f/IKcEeO5pVnHMmA0iZ5q4ccv1mA9V0sCwVK0sK"}}, Rules: []Rule{{Act: "allow", CIDR: "192.168.1.0/24"}}, Satisfy: "any", Pass: true}},
		Hosts: []Host{
			{ID: "h1", Type: "proxy", Enabled: true, Domains: []string{"app.localhost", "www.app.localhost"}, Upstreams: []string{"127.0.0.1:9001"}, Proto: "https", Cert: "internal", ForceHTTPS: true, HSTS: true, Exploits: true, Gzip: true, Cache: true,
				Paths:   []PathRoute{{Path: "/api", To: "127.0.0.1:9002", Strip: true}, {Path: "/files/", To: "127.0.0.1:9003"}},
				Access:  Access{Mode: "list", ListID: "aaaaaaaaaaaa"},
				Headers: []Header{{Kind: "res", Name: "X-Frame-Options", Value: "SAMEORIGIN"}, {Kind: "req", Name: "X-Real-IP", Value: "{remote_host}"}}},
			{ID: "h2", Type: "proxy", Enabled: true, Domains: []string{"lb.localhost"}, Upstreams: []string{"127.0.0.1:9001", "http://127.0.0.1:9002"}, LB: "least_conn", HealthPath: "/", Proto: "http",
				Access: Access{Mode: "custom", Users: []User{{Name: "bob", Hash: "$2a$12$C6UzMDM.H6dfI/f/IKcEeO5pVnHMmA0iZ5q4ccv1mA9V0sCwVK0sK"}}, Rules: []Rule{{Act: "deny", CIDR: "10.9.9.9"}, {Act: "allow", CIDR: "private_ranges"}}, Satisfy: "all"}, Maintenance: true},
			{ID: "h3", Type: "redirect", Enabled: true, Domains: []string{"old.localhost"}, RedirectTo: "https://new.example.com", RedirectCode: 302, KeepPath: true, Proto: "http"},
			{ID: "h4", Type: "dead", Enabled: true, Domains: []string{"parked.localhost"}, DeadCode: "410", DeadMsg: "Gone", Proto: "https", Cert: "internal", ForceHTTPS: false},
			{ID: "h5", Type: "dead", Enabled: true, Domains: []string{"abort.localhost"}, DeadCode: "abort", Proto: "http"},
			{ID: "h6", Type: "proxy", Enabled: false, Domains: []string{"off.localhost"}, Upstreams: []string{"x:1"}},
		},
	}
	if err := validateState(&st); err != nil {
		t.Fatal(err)
	}
	text := generate(&st, Settings{Email: "me@example.com"}, "/tmp/cmdata")
	t.Log("\n" + text)
	if os.Getenv("CADDY_BIN") == "" {
		t.Skip("set CADDY_BIN to validate against a real caddy")
	}
	c := &Caddy{bin: os.Getenv("CADDY_BIN"), dataDir: t.TempDir()}
	c.setEnv("")
	if err := c.Validate(text); err != nil {
		t.Fatalf("invalid: %v", err)
	}
}

func TestValidationRejectsInjection(t *testing.T) {
	bad := []Host{
		{Type: "proxy", Enabled: true, Domains: []string{"a.com {\n}"}, Upstreams: []string{"x:1"}},
		{Type: "proxy", Enabled: true, Domains: []string{"a.com"}, Upstreams: []string{"x:1\nrespond 200"}},
		{Type: "redirect", Enabled: true, Domains: []string{"a.com"}, RedirectTo: "https://x.com\" 301\nabort"},
		{Type: "proxy", Enabled: true, Domains: []string{"a.com"}, Upstreams: []string{"x:1"}, Headers: []Header{{Kind: "res", Name: "X", Value: "a\"b"}}},
		{Type: "proxy", Enabled: true, Domains: []string{"a.com"}, Upstreams: []string{"x:1"}, Paths: []PathRoute{{Path: "/a {", To: "x:1"}}},
		{Type: "proxy", Enabled: true, Domains: []string{"a.com"}, Upstreams: []string{"x:1"}, Access: Access{Mode: "custom", Users: []User{{Name: "a b", Password: "x"}}}},
	}
	for i, h := range bad {
		st := State{Hosts: []Host{h}}
		if err := validateState(&st); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
}

func TestRedact(t *testing.T) {
	in := "a 203.0.113.9 b 203.0.113.9 c 10.0.0.5 d 127.0.0.1 alice $2a$12$C6UzMDM.H6dfI/f/IKcEeO5pVnHMmA0iZ5q4ccv1mA9V0sCwVK0sK"
	out := redact(in, true)
	want := "a ip-1 b ip-1 c ip-2 d 127.0.0.1 alice [hash removed]"
	if out != want {
		t.Fatalf("got %q", out)
	}
	if redact("x 1.2.3.4", false) != "x 1.2.3.4" {
		t.Fatal("should not mask when off")
	}
}
