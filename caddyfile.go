package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

type cfWriter struct {
	b   strings.Builder
	ind int
}

func (w *cfWriter) line(format string, a ...any) {
	if format == "" {
		w.b.WriteString("\n")
		return
	}
	w.b.WriteString(strings.Repeat("\t", w.ind))
	w.b.WriteString(fmt.Sprintf(format, a...))
	w.b.WriteString("\n")
}
func (w *cfWriter) open(format string, a ...any) { w.line(format+" {", a...); w.ind++ }
func (w *cfWriter) close()                       { w.ind--; w.line("}") }

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

func snippetName(l AccessList) string { return "access_" + l.ID }

func cidrs(rules []Rule, act string) []string {
	out := []string{}
	for _, r := range rules {
		if r.Act == act {
			out = append(out, r.CIDR)
		}
	}
	return out
}

// access rules, written inside a route block so they run in order
func writeAccess(w *cfWriter, users []User, rules []Rule, satisfy string, pass bool) {
	deny := cidrs(rules, "deny")
	allow := cidrs(rules, "allow")
	if len(deny) > 0 {
		w.line("@denied remote_ip %s", strings.Join(deny, " "))
		w.line("respond @denied 403")
	}
	if len(allow) > 0 && (len(users) == 0 || satisfy == "all") {
		w.line("@blocked not remote_ip %s", strings.Join(allow, " "))
		w.line("respond @blocked 403")
	}
	if len(users) > 0 {
		if len(allow) > 0 && satisfy == "any" {
			w.line("@needauth not remote_ip %s", strings.Join(allow, " "))
			w.open("basic_auth @needauth")
		} else {
			w.open("basic_auth")
		}
		for _, u := range users {
			w.line("%s %s", u.Name, u.Hash)
		}
		w.close()
		if pass {
			w.line("request_header X-Remote-User {http.auth.user.id}")
		}
	}
}

func writeProxy(w *cfWriter, ups []string, h Host) {
	if len(ups) > 1 {
		w.open("reverse_proxy %s", strings.Join(ups, " "))
		w.line("lb_policy %s", h.LB)
		if h.HealthPath != "" {
			w.line("health_uri %s", h.HealthPath)
			w.line("health_interval 15s")
		}
		w.close()
		return
	}
	w.line("reverse_proxy %s", ups[0])
}

func siteAddresses(h Host) string {
	addrs := []string{}
	for _, d := range h.Domains {
		if h.Proto == "http" {
			addrs = append(addrs, "http://"+d)
		} else if !h.ForceHTTPS {
			addrs = append(addrs, "http://"+d, "https://"+d)
		} else {
			addrs = append(addrs, d)
		}
	}
	return strings.Join(addrs, ", ")
}

func writeHost(w *cfWriter, h Host, st *State, dataDir string) {
	w.open("%s", siteAddresses(h))
	if h.Proto == "https" {
		switch h.Cert {
		case "dns":
			w.open("tls")
			w.line("dns cloudflare {env.CF_API_TOKEN}")
			w.close()
		case "internal":
			w.line("tls internal")
		case "custom":
			w.line("tls %s %s", filepath.Join(dataDir, "certs", h.ID+".crt"), filepath.Join(dataDir, "certs", h.ID+".key"))
		}
		if h.HSTS {
			w.line(`header Strict-Transport-Security "max-age=31536000"`)
		}
	}
	w.open("log")
	w.open("output file %s", filepath.Join(dataDir, "logs", "access.log"))
	w.line("roll_size 50MiB")
	w.line("roll_keep 3")
	w.close()
	w.line("format json")
	w.close()

	switch h.Type {
	case "redirect":
		target := h.RedirectTo
		if h.KeepPath {
			target = strings.TrimRight(target, "/") + "{uri}"
		}
		w.line("redir %s %d", quote(target), h.RedirectCode)
	case "dead":
		if h.DeadCode == "abort" {
			w.line("abort")
		} else {
			msg := h.DeadMsg
			if msg == "" {
				msg = "Not found"
			}
			w.line("respond %s %s", quote(msg), h.DeadCode)
		}
	default:
		w.open("route")
		if h.Exploits {
			w.line("@exploits path_regexp `(?i)(/\\.env|/\\.git|/wp-login\\.php|/wp-admin|/xmlrpc\\.php|/phpmyadmin|\\.\\./)`")
			w.line("respond @exploits 403")
		}
		switch h.Access.Mode {
		case "list":
			for _, l := range st.AccessLists {
				if l.ID == h.Access.ListID {
					w.line("import %s", snippetName(l))
				}
			}
		case "custom":
			writeAccess(w, h.Access.Users, h.Access.Rules, h.Access.Satisfy, false)
		}
		if h.Maintenance {
			w.line(`respond "Back soon — this site is under maintenance." 503`)
		}
		if h.Gzip {
			w.line("encode zstd gzip")
		}
		if h.Cache {
			w.line("@static path *.css *.js *.png *.jpg *.jpeg *.gif *.svg *.webp *.ico *.woff *.woff2")
			w.line(`header @static Cache-Control "public, max-age=604800"`)
		}
		for _, hd := range h.Headers {
			if hd.Kind == "req" {
				w.line("request_header %s %s", hd.Name, quote(hd.Value))
			} else {
				w.line("header %s %s", hd.Name, quote(hd.Value))
			}
		}
		if len(h.Paths) > 0 {
			for _, p := range h.Paths {
				dir := "handle"
				if p.Strip {
					dir = "handle_path"
				}
				w.open("%s %s/*", dir, strings.TrimRight(p.Path, "/"))
				w.line("reverse_proxy %s", p.To)
				w.close()
			}
			w.open("handle")
			writeProxy(w, h.Upstreams, h)
			w.close()
		} else {
			writeProxy(w, h.Upstreams, h)
		}
		w.close()
	}
	if strings.TrimSpace(h.Raw) != "" {
		for _, l := range strings.Split(strings.TrimSpace(h.Raw), "\n") {
			w.line("%s", strings.TrimRight(l, " \t\r"))
		}
	}
	w.close()
}

func generate(st *State, set Settings, dataDir string) string {
	if st.Manual {
		return st.ManualCaddyfile
	}
	w := &cfWriter{}
	w.line("# Managed by caddy-manager. Edits here are replaced on the next apply;")
	w.line("# upload a Caddyfile from the Hosts page to run your own instead.")
	w.line("{")
	w.ind++
	w.line("admin localhost:2019")
	if set.Email != "" {
		w.line("email %s", set.Email)
	}
	if g := strings.TrimSpace(st.GlobalExtra); g != "" {
		w.line("")
		w.line("# Extra global options")
		for _, l := range strings.Split(g, "\n") {
			w.line("%s", strings.TrimRight(l, " \t\r"))
		}
	}
	w.close()
	if x := strings.TrimSpace(st.ExtraConfig); x != "" {
		w.line("")
		w.line("# ---- Kept exactly as written (Settings → Extra Caddyfile config) ----")
		w.b.WriteString(x + "\n")
		w.line("# ---- end of kept config ----")
	}
	used := map[string]bool{}
	for _, h := range st.Hosts {
		if h.Enabled && h.Type == "proxy" && h.Access.Mode == "list" {
			used[h.Access.ListID] = true
		}
	}
	for _, l := range st.AccessLists {
		if !used[l.ID] {
			continue
		}
		w.line("")
		w.line("# Access list: %s", strings.ReplaceAll(l.Name, "\n", " "))
		w.open("(%s)", snippetName(l))
		writeAccess(w, l.Users, l.Rules, l.Satisfy, l.Pass)
		w.close()
	}
	for _, h := range st.Hosts {
		if !h.Enabled {
			continue
		}
		w.line("")
		writeHost(w, h, st, dataDir)
	}
	return w.b.String()
}

// snippet for a single host / list, for the live preview in the editor
func renderHost(h Host, st *State, dataDir string) string {
	w := &cfWriter{}
	writeHost(w, h, st, dataDir)
	return w.b.String()
}

func renderList(l AccessList) string {
	w := &cfWriter{}
	w.open("(%s)", snippetName(l))
	writeAccess(w, l.Users, l.Rules, l.Satisfy, l.Pass)
	w.close()
	w.line("")
	w.line("# on a host:  import %s", snippetName(l))
	return w.b.String()
}
