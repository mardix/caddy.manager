package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ---------------- Caddyfile lexer / tree ----------------

type ctok struct {
	text  string
	quote byte // 0, '"' or '`'
	line  int
	start int
	end   int
}

type cnode struct {
	toks  []ctok
	kids  []*cnode
	block bool
	start int // byte offset of the first token
	end   int // byte offset after the last token / closing brace
}

func (n *cnode) name() string {
	if len(n.toks) == 0 {
		return ""
	}
	return n.toks[0].text
}

func (n *cnode) args() []string {
	out := []string{}
	for _, t := range n.toks[1:] {
		out = append(out, t.text)
	}
	return out
}

var errHeredoc = fmt.Errorf("heredoc")

func lexCaddyfile(src string) ([]ctok, error) {
	var toks []ctok
	line := 1
	i := 0
	for i < len(src) {
		c := src[i]
		if c == '\n' {
			line++
			i++
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if c == '#' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		start, startLine := i, line
		if c == '"' || c == '`' {
			q := c
			i++
			var b strings.Builder
			for i < len(src) && src[i] != q {
				if q == '"' && src[i] == '\\' && i+1 < len(src) && (src[i+1] == '"' || src[i+1] == '\\') {
					b.WriteByte(src[i+1])
					i += 2
					continue
				}
				if src[i] == '\n' {
					line++
				}
				b.WriteByte(src[i])
				i++
			}
			i++ // closing quote
			toks = append(toks, ctok{text: b.String(), quote: q, line: startLine, start: start, end: i})
			continue
		}
		for i < len(src) && src[i] != ' ' && src[i] != '\t' && src[i] != '\n' && src[i] != '\r' {
			i++
		}
		t := src[start:i]
		if strings.HasPrefix(t, "<<") {
			return nil, errHeredoc
		}
		toks = append(toks, ctok{text: t, line: startLine, start: start, end: i})
	}
	return toks, nil
}

func parseCaddyfile(src string) ([]*cnode, error) {
	toks, err := lexCaddyfile(src)
	if err != nil {
		return nil, err
	}
	// group tokens by the line they start on
	var lines [][]ctok
	for _, t := range toks {
		if len(lines) > 0 && lines[len(lines)-1][0].line == t.line {
			lines[len(lines)-1] = append(lines[len(lines)-1], t)
		} else {
			lines = append(lines, []ctok{t})
		}
	}
	root := &cnode{}
	stack := []*cnode{root}
	for _, ln := range lines {
		// a line may start with closing braces
		for len(ln) > 0 && ln[0].text == "}" && ln[0].quote == 0 {
			if len(stack) == 1 {
				return nil, fmt.Errorf("unexpected } on line %d", ln[0].line)
			}
			stack[len(stack)-1].end = ln[0].end
			stack = stack[:len(stack)-1]
			ln = ln[1:]
		}
		if len(ln) == 0 {
			continue
		}
		n := &cnode{start: ln[0].start, end: ln[len(ln)-1].end}
		last := ln[len(ln)-1]
		if last.text == "{" && last.quote == 0 {
			n.block = true
			ln = ln[:len(ln)-1]
		}
		n.toks = ln
		parent := stack[len(stack)-1]
		parent.kids = append(parent.kids, n)
		if n.block {
			stack = append(stack, n)
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("a { block is not closed")
	}
	return root.kids, nil
}

// serialize writes a node back as Caddyfile text (comments are not kept)
func serialize(n *cnode, ind int) string {
	var b strings.Builder
	parts := []string{}
	for _, t := range n.toks {
		switch {
		case t.quote == '`':
			parts = append(parts, "`"+t.text+"`")
		case t.quote == '"' || strings.ContainsAny(t.text, " \t") || t.text == "":
			parts = append(parts, quote(t.text))
		default:
			parts = append(parts, t.text)
		}
	}
	b.WriteString(strings.Repeat("\t", ind) + strings.Join(parts, " "))
	if n.block {
		b.WriteString(" {\n")
		for _, k := range n.kids {
			b.WriteString(serialize(k, ind+1))
		}
		b.WriteString(strings.Repeat("\t", ind) + "}")
	}
	b.WriteString("\n")
	return b.String()
}

// verbatim returns the original text of a node, from the start of its line, so comments and layout survive
func verbatim(src string, n *cnode) string {
	s := n.start
	for s > 0 && src[s-1] != '\n' {
		s--
	}
	return strings.TrimRight(src[s:n.end], " \t") + "\n"
}

// ---------------- conversion ----------------

type ImportItem struct {
	Name    string   `json:"name"`
	Managed bool     `json:"managed"`
	Type    string   `json:"type,omitempty"`
	Notes   []string `json:"notes"`
}

type ImportResult struct {
	Hosts       []Host       `json:"hosts"`
	Items       []ImportItem `json:"items"`
	Email       string       `json:"email,omitempty"`
	GlobalExtra string       `json:"globalExtra"`
	ExtraConfig string       `json:"extraConfig"`
	GlobalNotes []string     `json:"globalNotes"`
	Localhost   []string     `json:"localhost"`
}

var reBcrypt = regexp.MustCompile(`^\$2[aby]?\$\d\d\$[./A-Za-z0-9]{53}$`)

// directives that can stay inside a managed host as custom directives without changing how requests are handled
var rawOK = map[string]bool{"header": true, "request_header": true, "request_body": true, "log_skip": true, "skip_log": true}

func isLocal(u string) bool {
	h := upstreamAddr(u)
	return strings.HasPrefix(h, "localhost:") || strings.HasPrefix(h, "127.") || strings.HasPrefix(h, "[::1]:")
}

func pathFromMatcher(m string) (string, bool) {
	p := strings.TrimSuffix(strings.TrimSuffix(m, "*"), "/")
	if p == "" || !rePath.MatchString(p) {
		return "", false
	}
	return p, true
}

// convertSite turns one site block into a Host, or says why it must be kept as written.
func convertSite(n *cnode, snippets map[string]*cnode) (Host, []string, string) {
	h := Host{Type: "proxy", Enabled: true, Proto: "https", Cert: "auto", ForceHTTPS: true, LB: "round_robin",
		Paths: []PathRoute{}, Headers: []Header{}, Access: Access{Mode: "none", Satisfy: "any", Users: []User{}, Rules: []Rule{}}}
	notes := []string{}

	// addresses
	seen := map[string]map[string]bool{}
	var order []string
	for _, t := range n.toks {
		for _, a := range strings.Split(t.text, ",") {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			scheme := ""
			if strings.HasPrefix(a, "http://") {
				scheme, a = "http", a[7:]
			} else if strings.HasPrefix(a, "https://") {
				scheme, a = "https", a[8:]
			}
			if strings.Contains(a, "/") || strings.HasPrefix(a, ":") || !reDomain.MatchString(a) {
				return h, nil, fmt.Sprintf("its address %q can't be a managed host", t.text)
			}
			a = strings.ToLower(a)
			if seen[a] == nil {
				seen[a] = map[string]bool{}
				order = append(order, a)
			}
			seen[a][scheme] = true
		}
	}
	if len(order) == 0 {
		return h, nil, "it has no address"
	}
	allHTTP, anyBoth := true, false
	for _, d := range order {
		s := seen[d]
		if !(s["http"] && len(s) == 1) {
			allHTTP = false
		}
		if s["http"] && (s["https"] || s[""]) {
			anyBoth = true
		}
	}
	h.Domains = order
	if allHTTP {
		h.Proto = "http"
	} else if anyBoth {
		h.ForceHTTPS = false
	}

	// expand imports of snippets defined in the same file
	var kids []*cnode
	for _, k := range n.kids {
		if k.name() == "import" && len(k.toks) == 2 {
			if sn, ok := snippets[k.toks[1].text]; ok {
				kids = append(kids, sn.kids...)
				continue
			}
			return h, nil, fmt.Sprintf("it imports %q, which isn't a snippet in this file", k.toks[1].text)
		}
		kids = append(kids, k)
	}

	mains := 0
	var raw []string
	setMainProxy := func(k *cnode, ups []string) string {
		if len(ups) == 0 {
			return "reverse_proxy has no upstream"
		}
		for _, u := range ups {
			if !reUpstream.MatchString(u) {
				return fmt.Sprintf("upstream %q isn't a plain address", u)
			}
		}
		for _, sk := range k.kids {
			a := sk.args()
			switch sk.name() {
			case "lb_policy":
				if len(a) != 1 || !map[string]bool{"round_robin": true, "least_conn": true, "ip_hash": true, "first": true}[a[0]] {
					return "it uses a load-balancing option this app doesn't manage"
				}
				h.LB = a[0]
			case "health_uri":
				if len(a) != 1 || !reHealth.MatchString(a[0]) {
					return "its health check can't be managed"
				}
				h.HealthPath = a[0]
			case "health_interval":
			default:
				return fmt.Sprintf("reverse_proxy uses %q", sk.name())
			}
		}
		h.Upstreams = ups
		mains++
		return ""
	}
	addPath := func(m string, k *cnode, strip bool) string {
		p, ok := pathFromMatcher(m)
		if !ok || len(k.toks) != 2 || k.block {
			return fmt.Sprintf("its route for %s can't be managed", m)
		}
		if !reUpstream.MatchString(k.toks[1].text) {
			return fmt.Sprintf("upstream %q isn't a plain address", k.toks[1].text)
		}
		h.Paths = append(h.Paths, PathRoute{Path: p, To: k.toks[1].text, Strip: strip})
		if strings.HasSuffix(m, "*") && !strings.HasSuffix(m, "/*") {
			notes = append(notes, fmt.Sprintf("%s now matches %s/* (and no longer %s-anything)", m, p, p))
		}
		return ""
	}

	for _, k := range kids {
		a := k.args()
		name := k.name()
		switch {
		case strings.HasPrefix(name, "@"):
			return h, nil, "it uses named matchers (" + name + ")"
		case name == "log":
			// the manager writes its own access log for every host
		case name == "reverse_proxy":
			if len(a) > 0 && strings.HasPrefix(a[0], "/") {
				if why := addPath(a[0], &cnode{toks: k.toks[1:], block: k.block, kids: k.kids}, false); why != "" {
					return h, nil, why
				}
				continue
			}
			if len(a) > 0 && (strings.HasPrefix(a[0], "@") || a[0] == "*") {
				if a[0] != "*" {
					return h, nil, "reverse_proxy uses a matcher"
				}
				a = a[1:]
			}
			if mains > 0 {
				return h, nil, "it has more than one main handler"
			}
			if why := setMainProxy(k, a); why != "" {
				return h, nil, why
			}
		case name == "handle" || name == "handle_path":
			if len(k.kids) != 1 || k.kids[0].name() != "reverse_proxy" {
				return h, nil, fmt.Sprintf("its %s block does more than proxy", name)
			}
			inner := k.kids[0]
			if len(a) == 0 && name == "handle" {
				if mains > 0 {
					return h, nil, "it has more than one main handler"
				}
				if why := setMainProxy(inner, inner.args()); why != "" {
					return h, nil, why
				}
				continue
			}
			if len(a) != 1 || !strings.HasPrefix(a[0], "/") || len(inner.kids) > 0 {
				return h, nil, fmt.Sprintf("its %s block can't be managed", name)
			}
			if why := addPath(a[0], inner, name == "handle_path"); why != "" {
				return h, nil, why
			}
		case name == "redir":
			if len(a) == 0 || !strings.HasPrefix(a[0], "http") || len(a) > 2 {
				return h, nil, "its redirect uses a matcher or a relative target"
			}
			code := 302
			if len(a) == 2 {
				switch a[1] {
				case "permanent":
					code = 301
				case "temporary":
					code = 302
				default:
					c, err := strconv.Atoi(a[1])
					if err != nil || (c != 301 && c != 302 && c != 307 && c != 308) {
						return h, nil, "its redirect type can't be managed"
					}
					code = c
				}
			}
			target := a[0]
			if strings.HasSuffix(target, "{uri}") {
				h.KeepPath = true
				target = strings.TrimSuffix(target, "{uri}")
			}
			if !reURL.MatchString(target) || strings.Contains(target, "{") {
				return h, nil, "its redirect target uses placeholders"
			}
			h.Type, h.RedirectTo, h.RedirectCode = "redirect", target, code
			mains++
		case name == "respond":
			if len(a) > 0 && (strings.HasPrefix(a[0], "/") || strings.HasPrefix(a[0], "@")) {
				return h, nil, "respond uses a matcher"
			}
			body, status := "", ""
			switch len(a) {
			case 1:
				if _, err := strconv.Atoi(a[0]); err == nil {
					status = a[0]
				} else {
					body, status = a[0], "200"
				}
			case 2:
				body, status = a[0], a[1]
			default:
				return h, nil, "its respond can't be managed"
			}
			if status != "404" && status != "410" {
				return h, nil, "it responds with status " + status
			}
			if !noBad(body) {
				return h, nil, "its response text has quotes or new lines"
			}
			h.Type, h.DeadCode, h.DeadMsg = "dead", status, body
			mains++
		case name == "abort" && len(a) == 0:
			h.Type, h.DeadCode = "dead", "abort"
			mains++
		case name == "encode":
			h.Gzip = true
		case name == "tls":
			switch {
			case len(a) == 1 && a[0] == "internal" && !k.block:
				h.Cert = "internal"
			case len(a) == 0 && len(k.kids) == 1 && k.kids[0].name() == "dns" && len(k.kids[0].toks) >= 2 && k.kids[0].toks[1].text == "cloudflare":
				h.Cert = "dns"
				notes = append(notes, "uses the DNS challenge: add your Cloudflare token in Settings")
			case len(a) == 1 && strings.Contains(a[0], "@") && !k.block:
				notes = append(notes, "per-site ACME email "+a[0]+" dropped (set one email in Settings)")
			default:
				return h, nil, "its tls settings can't be managed (e.g. certificate files)"
			}
		case name == "basic_auth" || name == "basicauth":
			if len(a) > 1 || (len(a) == 1 && a[0] != "bcrypt") || len(k.kids) == 0 {
				return h, nil, "its basic_auth uses a matcher or another hash type"
			}
			for _, u := range k.kids {
				if len(u.toks) != 2 || !reUser.MatchString(u.toks[0].text) || !reBcrypt.MatchString(u.toks[1].text) {
					return h, nil, "a basic_auth user can't be managed"
				}
				h.Access.Users = append(h.Access.Users, User{Name: u.toks[0].text, Hash: u.toks[1].text})
			}
			h.Access.Mode = "custom"
		case (name == "header" || name == "request_header") && len(k.toks) == 3 && !k.block && reHdrName.MatchString(a[0]) && noBad(a[1]):
			kind := "res"
			if name == "request_header" {
				kind = "req"
			}
			h.Headers = append(h.Headers, Header{Kind: kind, Name: a[0], Value: a[1]})
		case rawOK[name]:
			raw = append(raw, serialize(k, 0))
			notes = append(notes, fmt.Sprintf("%q kept as a custom directive", name))
		default:
			return h, nil, fmt.Sprintf("it uses %q", name)
		}
	}
	if mains != 1 {
		if mains == 0 {
			return h, nil, "it doesn't proxy, redirect or return an error"
		}
		return h, nil, "it has more than one main handler"
	}
	if h.Type != "proxy" && (len(h.Paths) > 0 || h.Access.Mode != "none" || len(h.Headers) > 0 || len(raw) > 0) {
		return h, nil, "it combines a redirect or error page with other settings"
	}
	if h.Type == "proxy" && len(h.Upstreams) == 0 {
		return h, nil, "it has paths but no main upstream"
	}
	h.Raw = strings.TrimSpace(strings.Join(raw, ""))
	return h, notes, ""
}

// importCaddyfile converts a whole file. Sites that can't be represented faithfully are kept verbatim in ExtraConfig.
func importCaddyfile(src string) (ImportResult, error) {
	res := ImportResult{Hosts: []Host{}, Items: []ImportItem{}, GlobalNotes: []string{}, Localhost: []string{}}
	nodes, err := parseCaddyfile(src)
	if err == errHeredoc {
		return res, fmt.Errorf("this file uses heredocs (<<), which the importer doesn't support; run it as-is instead")
	}
	if err != nil {
		return res, err
	}
	if len(nodes) > 0 && !nodes[0].block {
		return res, fmt.Errorf("this looks like a single-site Caddyfile without braces; wrap the site in { } or run it as-is")
	}
	snippets := map[string]*cnode{}
	var extra []string
	var global []string
	for i, n := range nodes {
		if i == 0 && len(n.toks) == 0 && n.block { // global options
			for _, g := range n.kids {
				switch g.name() {
				case "admin":
					res.GlobalNotes = append(res.GlobalNotes, "admin setting dropped (the manager needs its own)")
				case "email":
					if len(g.toks) == 2 {
						res.Email = g.toks[1].text
					}
				default:
					global = append(global, serialize(g, 0))
				}
			}
			continue
		}
		if len(n.toks) == 1 && strings.HasPrefix(n.name(), "(") && strings.HasSuffix(n.name(), ")") {
			snippets[strings.Trim(n.name(), "()")] = n
			extra = append(extra, verbatim(src, n))
			continue
		}
		label := []string{}
		for _, t := range n.toks {
			label = append(label, t.text)
		}
		name := strings.Join(label, " ")
		h, notes, why := convertSite(n, snippets)
		if why != "" {
			res.Items = append(res.Items, ImportItem{Name: name, Managed: false, Notes: []string{"Kept exactly as written because " + why + "."}})
			extra = append(extra, verbatim(src, n))
			for _, k := range n.kids {
				walkProxies(k, func(u string) {
					if isLocal(u) {
						res.Localhost = appendUniq(res.Localhost, u)
					}
				})
			}
			continue
		}
		h.ID = newID()
		res.Hosts = append(res.Hosts, h)
		res.Items = append(res.Items, ImportItem{Name: strings.Join(h.Domains, ", "), Managed: true, Type: h.Type, Notes: notes})
		for _, u := range h.Upstreams {
			if isLocal(u) {
				res.Localhost = appendUniq(res.Localhost, u)
			}
		}
		for _, p := range h.Paths {
			if isLocal(p.To) {
				res.Localhost = appendUniq(res.Localhost, p.To)
			}
		}
	}
	// snippets only matter if a kept-as-written site uses them
	keptSites := false
	for _, it := range res.Items {
		if !it.Managed {
			keptSites = true
		}
	}
	if !keptSites {
		var onlySites []string
		for _, e := range extra {
			if !strings.HasPrefix(strings.TrimSpace(e), "(") {
				onlySites = append(onlySites, e)
			}
		}
		extra = onlySites
	}
	res.GlobalExtra = strings.TrimSpace(strings.Join(global, ""))
	res.ExtraConfig = strings.TrimSpace(strings.Join(extra, "\n"))
	if res.GlobalExtra != "" {
		res.GlobalNotes = append(res.GlobalNotes, "Other global options kept as written (Settings → Extra Caddyfile config).")
	}
	return res, nil
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func walkProxies(n *cnode, f func(string)) {
	if n.name() == "reverse_proxy" {
		for _, a := range n.args() {
			if !strings.HasPrefix(a, "/") && !strings.HasPrefix(a, "@") && a != "*" {
				f(a)
			}
		}
		for _, k := range n.kids {
			if k.name() == "to" {
				for _, a := range k.args() {
					f(a)
				}
			}
		}
	}
	for _, k := range n.kids {
		walkProxies(k, f)
	}
}

var reLocalUp = regexp.MustCompile(`(^|[\s/])(localhost|127\.0\.0\.1|\[::1\])(:\d+)`)

// rewriteLocal points loopback upstreams at the Docker host.
func rewriteLocal(u string) string {
	return reLocalUp.ReplaceAllString(u, "${1}host.docker.internal${3}")
}

func rewriteLocalInConfig(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "reverse_proxy") || strings.HasPrefix(t, "to ") {
			lines[i] = rewriteLocal(l)
		}
	}
	return strings.Join(lines, "\n")
}
