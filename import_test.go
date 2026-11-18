package main

import (
	"os"
	"strings"
	"testing"
)

const sampleCaddyfile = `# my old server
{
	email ops@example.com
	admin off
	servers {
		timeouts {
			read_body 30s
		}
	}
}

(common) {
	encode zstd gzip
	header X-Frame-Options SAMEORIGIN
}

app.example.com, www.app.example.com {
	import common
	reverse_proxy localhost:3000
	handle_path /api/* {
		reverse_proxy 127.0.0.1:4000
	}
	log {
		output file /var/log/caddy/app.log
	}
}

git.example.com {
	basic_auth {
		alice $2a$14$C6UzMDM.H6dfI/f/IKcEeO5pVnHMmA0iZ5q4ccv1mA9V0sCwVK0sK
	}
	reverse_proxy 192.168.1.10:3000 192.168.1.11:3000 {
		lb_policy least_conn
		health_uri /health
	}
}

http://printer.home.lan {
	reverse_proxy 192.168.1.50:80
}

example.org {
	redir https://example.com{uri} permanent
}

parked.example.net {
	respond "Nothing here" 404
}

static.example.com {
	root * /srv/www
	file_server
}

legacy.example.com {
	reverse_proxy localhost:8080 {
		header_up Host {upstream_hostport}
	}
}
`

func TestImportCaddyfile(t *testing.T) {
	res, err := importCaddyfile(sampleCaddyfile)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ImportItem{}
	for _, it := range res.Items {
		byName[it.Name] = it
		t.Logf("%-45s managed=%v %s %v", it.Name, it.Managed, it.Type, it.Notes)
	}
	want := map[string]bool{
		"app.example.com, www.app.example.com": true, "git.example.com": true, "printer.home.lan": true,
		"example.org": true, "parked.example.net": true, "static.example.com": false, "legacy.example.com": false,
	}
	for name, managed := range want {
		it, ok := byName[name]
		if !ok {
			t.Fatalf("missing item %q", name)
		}
		if it.Managed != managed {
			t.Errorf("%s: managed=%v, want %v", name, it.Managed, managed)
		}
	}
	if res.Email != "ops@example.com" {
		t.Errorf("email %q", res.Email)
	}
	if !strings.Contains(res.GlobalExtra, "servers") || strings.Contains(res.GlobalExtra, "admin") {
		t.Errorf("global extra: %q", res.GlobalExtra)
	}
	if !strings.Contains(res.ExtraConfig, "file_server") || !strings.Contains(res.ExtraConfig, "header_up") || !strings.Contains(res.ExtraConfig, "(common)") {
		t.Errorf("extra config missing kept blocks:\n%s", res.ExtraConfig)
	}
	var app, git, printer, redir, dead Host
	for _, h := range res.Hosts {
		switch h.Domains[0] {
		case "app.example.com":
			app = h
		case "git.example.com":
			git = h
		case "printer.home.lan":
			printer = h
		case "example.org":
			redir = h
		case "parked.example.net":
			dead = h
		}
	}
	if len(app.Domains) != 2 || app.Upstreams[0] != "localhost:3000" || !app.Gzip || len(app.Headers) != 1 || len(app.Paths) != 1 || !app.Paths[0].Strip || app.Paths[0].Path != "/api" {
		t.Errorf("app: %+v", app)
	}
	if git.Access.Mode != "custom" || len(git.Access.Users) != 1 || git.LB != "least_conn" || git.HealthPath != "/health" || len(git.Upstreams) != 2 {
		t.Errorf("git: %+v", git)
	}
	if printer.Proto != "http" {
		t.Errorf("printer proto %q", printer.Proto)
	}
	if redir.Type != "redirect" || redir.RedirectCode != 301 || !redir.KeepPath || redir.RedirectTo != "https://example.com" {
		t.Errorf("redirect: %+v", redir)
	}
	if dead.Type != "dead" || dead.DeadCode != "404" || dead.DeadMsg != "Nothing here" {
		t.Errorf("dead: %+v", dead)
	}
	if len(res.Localhost) != 3 {
		t.Errorf("localhost upstreams: %v", res.Localhost)
	}

	// the converted result must be a valid Caddyfile
	st := State{Hosts: res.Hosts, AccessLists: []AccessList{}, GlobalExtra: res.GlobalExtra, ExtraConfig: rewriteLocalInConfig(res.ExtraConfig)}
	for i := range st.Hosts {
		for j := range st.Hosts[i].Upstreams {
			st.Hosts[i].Upstreams[j] = rewriteLocal(st.Hosts[i].Upstreams[j])
		}
	}
	if err := validateState(&st); err != nil {
		t.Fatal(err)
	}
	text := generate(&st, Settings{Email: res.Email}, t.TempDir())
	if strings.Contains(text, "localhost:3000") || strings.Contains(text, "localhost:8080") || !strings.Contains(text, "host.docker.internal:8080") {
		t.Errorf("localhost rewrite failed:\n%s", text)
	}
	if os.Getenv("CADDY_BIN") == "" {
		t.Skip("set CADDY_BIN to validate against a real caddy")
	}
	os.MkdirAll("/tmp/srv-www-test", 0755)
	c := &Caddy{bin: os.Getenv("CADDY_BIN"), dataDir: t.TempDir()}
	c.setEnv("")
	if err := c.Validate(text); err != nil {
		t.Fatalf("generated config invalid: %v\n%s", err, text)
	}
}

func TestImportRejects(t *testing.T) {
	if _, err := importCaddyfile("example.com\nreverse_proxy x:1\n"); err == nil {
		t.Error("single-site file without braces should ask to wrap or run as-is")
	}
	if _, err := importCaddyfile("a.com {\n respond <<EOF\n hi\n EOF 200\n}\n"); err == nil {
		t.Error("heredoc should be refused")
	}
	if _, err := importCaddyfile("a.com {\n reverse_proxy x:1\n"); err == nil {
		t.Error("unclosed block should fail")
	}
	// injection attempt through a quoted header value stays out of managed fields
	res, err := importCaddyfile("a.com {\n header X-A \"v\\\"\nrespond 200\" \n reverse_proxy x:1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	st := State{Hosts: res.Hosts, ExtraConfig: res.ExtraConfig}
	if err := validateState(&st); err != nil {
		t.Fatal(err)
	}
}

func TestImportHostHeaderNoop(t *testing.T) {
	src, err := os.ReadFile("/tmp/user.Caddyfile")
	if err != nil {
		t.Skip("sample not present")
	}
	res, err := importCaddyfile(string(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		t.Logf("%-40s managed=%v %v", it.Name, it.Managed, it.Notes)
		if !it.Managed {
			t.Errorf("%s should be managed", it.Name)
		}
	}
	if res.ExtraConfig != "" {
		t.Errorf("nothing should be kept as written, got:\n%s", res.ExtraConfig)
	}
	st := State{Hosts: res.Hosts}
	if err := validateState(&st); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CADDY_BIN") != "" {
		c := &Caddy{bin: os.Getenv("CADDY_BIN"), dataDir: t.TempDir()}
		c.setEnv("")
		if err := c.Validate(generate(&st, Settings{}, t.TempDir())); err != nil {
			t.Fatal(err)
		}
	}
	// a header_up that does change something becomes a "to upstream" header
	r2, _ := importCaddyfile("a.com {\n reverse_proxy x:1 {\n  header_up X-Real-IP {remote_host}\n  header_down Strict-Transport-Security max-age=300\n }\n}\n")
	if !r2.Items[0].Managed || len(r2.Hosts[0].Headers) != 2 || r2.Hosts[0].Headers[0].Kind != "req" || r2.Hosts[0].Headers[1].Kind != "res" {
		t.Errorf("header_up/down mapping: %+v", r2.Hosts)
	}
	// placeholders that only exist inside reverse_proxy must keep the site as written
	r3, _ := importCaddyfile("a.com {\n reverse_proxy x:1 {\n  header_up Host {upstream_hostport}\n }\n}\n")
	if r3.Items[0].Managed {
		t.Error("{upstream_hostport} must not be converted")
	}
}
