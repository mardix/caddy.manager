package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

type aiReq struct {
	Mode     string `json:"mode"` // logs | explain | ask
	HostID   string `json:"hostId"`
	Range    string `json:"range"`
	Question string `json:"question"`
	Error    string `json:"error"`
}

func topN(m map[string]int, n int) []string {
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	out := []string{}
	for i := 0; i < len(list) && i < n; i++ {
		out = append(out, fmt.Sprintf("%s (%d)", list[i].k, list[i].v))
	}
	return out
}

func describeHost(h Host, lists []AccessList) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- %s [%s, %s", strings.Join(h.Domains, ", "), h.Type, map[bool]string{true: "enabled", false: "disabled"}[h.Enabled])
	if h.Type == "proxy" {
		fmt.Fprintf(&b, ", forwards to %s", strings.Join(h.Upstreams, " "))
		for _, p := range h.Paths {
			fmt.Fprintf(&b, ", %s -> %s (strip=%v)", p.Path, p.To, p.Strip)
		}
	}
	if h.Type == "redirect" {
		fmt.Fprintf(&b, ", %d to %s", h.RedirectCode, h.RedirectTo)
	}
	fmt.Fprintf(&b, ", %s, cert=%s", h.Proto, h.Cert)
	switch h.Access.Mode {
	case "list":
		for _, l := range lists {
			if l.ID == h.Access.ListID {
				fmt.Fprintf(&b, ", access list %q", l.Name)
			}
		}
	case "custom":
		fmt.Fprintf(&b, ", custom access (%d logins, %d IP rules)", len(h.Access.Users), len(h.Access.Rules))
	}
	b.WriteString("]")
	return b.String()
}

func (a *App) logContext(r *http.Request, host *Host, entries []Entry, from, step int64, n int, st State) (string, int) {
	s := computeStats(entries, from, step, n, st)
	var b strings.Builder
	scope := "all hosts"
	if host != nil {
		scope = "host " + strings.Join(host.Domains, ", ")
	}
	fmt.Fprintf(&b, "Scope: %s. Window starts %s, now is %s.\n", scope, time.UnixMilli(from).Format(time.RFC3339), time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "Totals: %d requests, 5xx=%d, 4xx=%d, 403=%d, median %dms, %d bytes.\n", s.Total, s.Err5, s.Err4, s.Blocked, s.Median, s.Bytes)
	fmt.Fprintf(&b, "Status classes: 2xx=%d 3xx=%d 4xx=%d 5xx=%d\n", s.Codes["2"], s.Codes["3"], s.Codes["4"], s.Codes["5"])
	b.WriteString("Requests per bucket (start time: ok/5xx): ")
	for i, bk := range s.Buckets {
		if bk["ok"]+bk["err"] == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s %d/%d; ", time.UnixMilli(from+int64(i)*step).Format("Jan 2 15:04"), bk["ok"], bk["err"])
	}
	b.WriteString("\n")
	ips, paths, errPaths, slow := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	for _, e := range entries {
		ips[e.IP]++
		p := e.URI
		if i := strings.IndexAny(p, "?#"); i >= 0 {
			p = p[:i]
		}
		key := e.Host + p
		paths[key]++
		if e.Status >= 400 {
			errPaths[fmt.Sprintf("%d %s", e.Status, key)]++
		}
		if e.Ms > 1000 {
			slow[key]++
		}
	}
	if host == nil {
		hosts := map[string]int{}
		for _, e := range entries {
			hosts[e.Host]++
		}
		fmt.Fprintf(&b, "Top hosts: %s\n", strings.Join(topN(hosts, 10), ", "))
	}
	fmt.Fprintf(&b, "Top client IPs: %s\n", strings.Join(topN(ips, 10), ", "))
	fmt.Fprintf(&b, "Top paths: %s\n", strings.Join(topN(paths, 10), ", "))
	fmt.Fprintf(&b, "Top error responses: %s\n", strings.Join(topN(errPaths, 15), ", "))
	fmt.Fprintf(&b, "Paths with requests over 1s: %s\n", strings.Join(topN(slow, 10), ", "))
	// sample lines: recent errors first, then recent others
	var errs, rest []Entry
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Status >= 400 && len(errs) < 80 {
			errs = append(errs, e)
		} else if e.Status < 400 && len(rest) < 60 {
			rest = append(rest, e)
		}
		if len(errs) >= 80 && len(rest) >= 60 {
			break
		}
	}
	b.WriteString("\nRecent error lines (newest first):\n")
	for _, e := range errs {
		fmt.Fprintf(&b, "%s %d %s %s%s %s %dms\n", time.UnixMilli(e.TS).Format("01-02 15:04:05"), e.Status, e.Method, e.Host, e.URI, e.IP, e.Ms)
	}
	b.WriteString("\nRecent other lines (newest first):\n")
	for _, e := range rest {
		fmt.Fprintf(&b, "%s %d %s %s%s %s %dms\n", time.UnixMilli(e.TS).Format("01-02 15:04:05"), e.Status, e.Method, e.Host, e.URI, e.IP, e.Ms)
	}
	// caddy events (upstream errors, cert problems) in the window
	evs := a.caddy.Events()
	b.WriteString("\nCaddy events in window (errors, certificates, reloads):\n")
	count := 0
	for i := len(evs) - 1; i >= 0 && count < 30; i-- {
		e := evs[i]
		if int64(e.TS*1000) < from {
			break
		}
		if host != nil && e.Ident != "" && !hostMatches(host.Domains, e.Ident) {
			continue
		}
		fmt.Fprintf(&b, "%s %s %s %s %s\n", time.Unix(int64(e.TS), 0).Format("01-02 15:04:05"), e.Level, e.Msg, e.Ident, e.Err)
		count++
	}
	return b.String(), len(entries)
}

var reJSONBlock = regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")

func (a *App) ai(w http.ResponseWriter, r *http.Request) {
	var req aiReq
	if err := readJSON(r, &req); err != nil {
		jerr(w, 400, "bad request")
		return
	}
	st, set := a.snapshot()
	if !set.AI.Enabled {
		jerr(w, 400, "Turn on the AI assistant in Settings first")
		return
	}
	var host *Host
	for i := range st.Hosts {
		if st.Hosts[i].ID == req.HostID {
			host = &st.Hosts[i]
		}
	}
	system := aiSystemBase
	var user string
	basis := ""
	switch req.Mode {
	case "logs":
		if strings.TrimSpace(req.Question) == "" {
			jerr(w, 400, "Type a question")
			return
		}
		q := r.URL.Query()
		q.Set("range", req.Range)
		q.Set("host", req.HostID)
		r.URL.RawQuery = q.Encode()
		entries, from, step, n, h := a.filterEntries(r)
		ctx, count := a.logContext(r, h, entries, from, step, n, st)
		system += "\nYou answer questions about access logs. Quote concrete numbers, times, paths and IPs from the data. If something should be fixed, say what to change in the UI (e.g. turn on 'Block common exploits', add an IP deny rule, add a second upstream)."
		user = "Log data:\n" + ctx + "\n\nQuestion: " + req.Question
		basis = fmt.Sprintf("Looked at %d log lines", count)
	case "explain":
		if strings.TrimSpace(req.Error) == "" {
			jerr(w, 400, "Nothing to explain")
			return
		}
		var b strings.Builder
		b.WriteString("Error shown to the user:\n" + req.Error + "\n\n")
		checks := []string{}
		if host != nil {
			b.WriteString("Host config (Caddyfile block):\n" + renderHost(*host, &st, a.dataDir) + "\n")
			for _, d := range host.Domains {
				hn := d
				if x, _, err := net.SplitHostPort(d); err == nil {
					hn = x
				}
				if strings.HasPrefix(hn, "*.") {
					continue
				}
				addrs, err := net.LookupHost(hn)
				if err != nil {
					checks = append(checks, fmt.Sprintf("DNS lookup %s: failed (%v)", hn, err))
				} else {
					checks = append(checks, fmt.Sprintf("DNS lookup %s: %s", hn, strings.Join(addrs, ", ")))
				}
			}
			for _, u := range host.Upstreams {
				hc := checkUpstream(u)
				checks = append(checks, fmt.Sprintf("TCP connect to upstream %s: %s %dms %s", u, hc.Status, hc.Ms, hc.Error))
			}
		}
		running, exitErr, _ := a.caddy.Status()
		checks = append(checks, fmt.Sprintf("Caddy process running: %v %s", running, exitErr))
		b.WriteString("Checks just run:\n" + strings.Join(checks, "\n") + "\n\nRecent Caddy events:\n")
		evs := a.caddy.Events()
		for i, c := len(evs)-1, 0; i >= 0 && c < 25; i-- {
			e := evs[i]
			if e.Level == "error" || e.Level == "warn" || host == nil || hostMatches(host.Domains, e.Ident) {
				fmt.Fprintf(&b, "%s %s %s %s %s\n", time.Unix(int64(e.TS), 0).Format("01-02 15:04:05"), e.Level, e.Msg, e.Ident, e.Err)
				c++
			}
		}
		system += "\nExplain what went wrong in 2-4 sentences, then give the fix as a short numbered list. Mention what you checked when relevant. This app runs Caddy in Docker; upstreams on the Docker host are reached via host.docker.internal, not localhost. Wildcard certificates need the DNS challenge with a Cloudflare token set in Settings."
		user = b.String()
		basis = fmt.Sprintf("Used the error text and %d checks", len(checks))
	case "ask":
		if strings.TrimSpace(req.Question) == "" {
			jerr(w, 400, "Type a question")
			return
		}
		var b strings.Builder
		b.WriteString("Current hosts:\n")
		for _, h := range st.Hosts {
			b.WriteString(describeHost(h, st.AccessLists) + "\n")
		}
		if len(st.Hosts) == 0 {
			b.WriteString("(none)\n")
		}
		b.WriteString("Access lists: ")
		names := []string{}
		for _, l := range st.AccessLists {
			names = append(names, fmt.Sprintf("%q", l.Name))
		}
		b.WriteString(strings.Join(names, ", ") + "\n")
		system += `
The user may ask a question or ask for a change to ONE host.
For a change: write one or two sentences describing it, then a JSON block exactly like:
` + "```json" + `
{"domain": "<existing or new domain>", "changes": { ...only the fields to set... }}
` + "```" + `
Allowed fields in "changes": type ("proxy"|"redirect"|"dead"), domains (array), upstreams (array of host:port), paths (array of {"path":"/api","to":"host:port","strip":true}), proto ("https"|"http"), cert ("auto"|"dns"|"internal"), forceHttps, hsts, exploits, gzip, cache, maintenance (booleans), redirectTo, redirectCode (301|302|307|308), keepPath, deadCode ("404"|"410"|"abort"), deadMsg, accessList (name of an existing access list, or "" for public).
"paths" replaces the whole list, so include existing paths you want to keep. Never set passwords. If information is missing (like the upstream port), ask instead of guessing. For a plain question, answer without JSON.`
		user = b.String() + "\nRequest: " + req.Question
		basis = fmt.Sprintf("Used your %d hosts", len(st.Hosts))
	default:
		jerr(w, 400, "unknown mode")
		return
	}
	user = redact(user, set.AI.MaskIPs)
	text, err := callAI(set.AI, system, user, 1200)
	if err != nil {
		jerr(w, 502, err.Error())
		return
	}
	resp := map[string]any{"text": text, "basis": basis}
	if req.Mode == "ask" {
		if m := reJSONBlock.FindStringSubmatch(text); m != nil {
			resp["text"] = strings.TrimSpace(reJSONBlock.ReplaceAllString(text, ""))
			if prop, desc := buildProposal(m[1], st); prop != nil {
				resp["proposal"] = prop
				resp["changes"] = desc
			}
		}
	}
	jok(w, resp)
}

// buildProposal merges the model's suggested fields into a copy of the matching host (or a new one).
func buildProposal(raw string, st State) (*Host, []string) {
	var p struct {
		Domain  string         `json:"domain"`
		Changes map[string]any `json:"changes"`
	}
	if json.Unmarshal([]byte(raw), &p) != nil || p.Changes == nil {
		return nil, nil
	}
	dom := strings.ToLower(strings.TrimSpace(p.Domain))
	var h Host
	found := false
	for _, x := range st.Hosts {
		for _, d := range x.Domains {
			if d == dom {
				b, _ := json.Marshal(x)
				json.Unmarshal(b, &h)
				found = true
			}
		}
	}
	if !found {
		h = Host{Type: "proxy", Enabled: true, Domains: []string{dom}, Proto: "https", Cert: "auto", ForceHTTPS: true, Exploits: true, Gzip: true}
	}
	desc := []string{}
	if !found {
		desc = append(desc, "New host "+dom)
	}
	// apply allowed fields via JSON round-trip
	allowed := map[string]bool{"type": true, "domains": true, "upstreams": true, "paths": true, "proto": true, "cert": true, "forceHttps": true, "hsts": true, "exploits": true, "gzip": true, "cache": true, "maintenance": true, "redirectTo": true, "redirectCode": true, "keepPath": true, "deadCode": true, "deadMsg": true}
	cur, _ := json.Marshal(h)
	var m map[string]any
	json.Unmarshal(cur, &m)
	keys := []string{}
	for k := range p.Changes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := p.Changes[k]
		if k == "accessList" {
			name, _ := v.(string)
			if name == "" {
				h.Access.Mode = "none"
				desc = append(desc, "Access: public")
				continue
			}
			for _, l := range st.AccessLists {
				if strings.EqualFold(l.Name, name) {
					h.Access = Access{Mode: "list", ListID: l.ID, Satisfy: "any", Users: []User{}, Rules: []Rule{}}
					desc = append(desc, "Access list: "+l.Name)
				}
			}
			continue
		}
		if !allowed[k] {
			continue
		}
		m[k] = v
		vb, _ := json.Marshal(v)
		desc = append(desc, fmt.Sprintf("%s: %s", k, string(vb)))
	}
	access := h.Access
	b, _ := json.Marshal(m)
	var out Host
	if json.Unmarshal(b, &out) != nil {
		return nil, nil
	}
	out.Access = access
	if out.ID == "" {
		out.ID = ""
	}
	normalizeHost(&out)
	if !found {
		out.ID = "" // new host: editor assigns
	}
	for i := range out.Access.Users {
		out.Access.Users[i].Hash = ""
	}
	return &out, desc
}
