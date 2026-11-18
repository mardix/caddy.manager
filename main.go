package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

type App struct {
	store    *Store
	caddy    *Caddy
	mon      *Monitor
	alog     *AccessLog
	password string
	secret   []byte
	dataDir  string

	pmu     sync.Mutex
	pending map[string]pendingChange
	applyMu sync.Mutex
}

type pendingChange struct {
	State     State
	Caddyfile string
	Label     string
	Created   time.Time
	SetEmail  string // ACME email picked up from an imported Caddyfile
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	log.SetFlags(log.LstdFlags)
	pw := os.Getenv("ADMIN_PASSWORD")
	if len(pw) < 8 {
		log.Fatal("ADMIN_PASSWORD must be set (at least 8 characters)")
	}
	dataDir := env("DATA_DIR", "/data")
	for _, d := range []string{"logs", "backups", "certs"} {
		if err := os.MkdirAll(filepath.Join(dataDir, d), 0700); err != nil {
			log.Fatal(err)
		}
	}
	app := &App{
		store:    &Store{dir: dataDir},
		mon:      &Monitor{health: map[string]Health{}},
		alog:     &AccessLog{path: filepath.Join(dataDir, "logs", "access.log")},
		password: pw,
		dataDir:  dataDir,
		pending:  map[string]pendingChange{},
	}
	if err := app.store.load(); err != nil {
		log.Fatal(err)
	}
	app.secret = loadSecret(dataDir)
	app.caddy = &Caddy{bin: env("CADDY_BIN", "caddy"), dataDir: dataDir}
	app.caddy.setEnv(app.store.settings.DNSToken)
	app.caddy.loadEvents()

	cf := app.caddy.caddyfilePath()
	if _, err := os.Stat(cf); err != nil {
		os.WriteFile(cf, []byte(generate(&app.store.state, app.store.settings, dataDir)), 0600)
	}
	if err := app.caddy.Start(); err != nil {
		log.Printf("could not start caddy: %v", err)
	}
	app.caddy.note("info", "Manager started")

	go func() {
		for {
			app.store.mu.Lock()
			st := app.store.state
			app.store.mu.Unlock()
			app.mon.CheckAll(st)
			time.Sleep(30 * time.Second)
		}
	}()

	mux := http.NewServeMux()
	app.routes(mux)
	srv := &http.Server{Addr: env("LISTEN", ":8080"), Handler: securityHeaders(mux), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Print("shutting down")
		app.caddy.Stop()
		srv.Close()
	}()
	log.Printf("caddy-manager listening on %s (data: %s)", srv.Addr, dataDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func loadSecret(dir string) []byte {
	p := filepath.Join(dir, ".secret")
	if b, err := os.ReadFile(p); err == nil && len(b) >= 32 {
		return b
	}
	b := make([]byte, 32)
	rand.Read(b)
	os.WriteFile(p, b, 0600)
	return b
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		h.ServeHTTP(w, r)
	})
}

// ---------------- auth ----------------

func (a *App) sign(exp int64) string {
	m := hmac.New(sha256.New, a.secret)
	fmt.Fprintf(m, "%d|%s", exp, a.password) // changing the password logs everyone out
	return hex.EncodeToString(m.Sum(nil))
}

func (a *App) authed(r *http.Request) bool {
	c, err := r.Cookie("cm_session")
	if err != nil {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(parts[1]), []byte(a.sign(exp)))
}

var loginMu sync.Mutex

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(a.password)) != 1 {
		loginMu.Lock() // serialize failures to slow down guessing
		time.Sleep(time.Second)
		loginMu.Unlock()
		jerr(w, 401, "Wrong password")
		return
	}
	exp := time.Now().Add(30 * 24 * time.Hour).Unix()
	http.SetCookie(w, &http.Cookie{
		Name: "cm_session", Value: fmt.Sprintf("%d.%s", exp, a.sign(exp)), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		Expires: time.Unix(exp, 0),
	})
	jok(w, map[string]bool{"ok": true})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "cm_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	jok(w, map[string]bool{"ok": true})
}

// ---------------- helpers ----------------

func jok(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func jerr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(v)
}

func (a *App) snapshot() (State, Settings) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	b, _ := json.Marshal(a.store.state)
	var st State
	json.Unmarshal(b, &st)
	return st, a.store.settings
}

func (a *App) routes(mux *http.ServeMux) {
	sub, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.Contains(r.URL.Path[1:], "/") {
			if _, err := fs.Stat(sub, r.URL.Path[1:]); err == nil {
				w.Header().Set("Cache-Control", "no-cache")
				files.ServeHTTP(w, r)
				return
			}
		}
		b, _ := fs.ReadFile(sub, "index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(b)
	})
	mux.HandleFunc("POST /api/login", a.login)
	mux.HandleFunc("POST /api/logout", a.logout)
	api := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !a.authed(r) {
				jerr(w, 401, "Please sign in")
				return
			}
			if r.Method != "GET" && r.Header.Get("X-Requested-With") != "caddy-manager" {
				jerr(w, 403, "Missing request header")
				return
			}
			h(w, r)
		})
	}
	api("GET /api/me", func(w http.ResponseWriter, r *http.Request) { jok(w, map[string]bool{"ok": true}) })
	api("GET /api/state", a.getState)
	api("GET /api/status", a.getStatus)
	api("POST /api/render", a.render)
	api("POST /api/preview", a.preview)
	api("POST /api/apply", a.apply)
	api("POST /api/import", a.analyzeImport)
	api("GET /api/caddyfile", a.downloadCaddyfile)
	api("GET /api/backups", a.listBackups)
	api("POST /api/backups/restore", a.restoreBackup)
	api("POST /api/check", a.checkNow)
	api("GET /api/stats", a.stats)
	api("GET /api/logs", a.logs)
	api("GET /api/settings", a.getSettings)
	api("POST /api/settings", a.saveSettings)
	api("POST /api/settings/test", a.testAI)
	api("POST /api/ai", a.ai)
	api("POST /api/certs/{id}", a.uploadCert)
	api("POST /api/caddy/restart", func(w http.ResponseWriter, r *http.Request) {
		if err := a.caddy.Restart(); err != nil {
			jerr(w, 500, err.Error())
			return
		}
		a.caddy.note("info", "Caddy restarted from the UI")
		jok(w, map[string]bool{"ok": true})
	})
}

// ---------------- state & preview/apply ----------------

func (a *App) getState(w http.ResponseWriter, r *http.Request) {
	st, set := a.snapshot()
	b, _ := os.ReadFile(a.caddy.caddyfilePath())
	kept := []string{}
	if nodes, err := parseCaddyfile(st.ExtraConfig); err == nil {
		for _, n := range nodes {
			if len(n.toks) > 0 && !strings.HasPrefix(n.name(), "(") {
				parts := []string{}
				for _, t := range n.toks {
					parts = append(parts, t.text)
				}
				kept = append(kept, strings.Join(parts, " "))
			}
		}
	}
	jok(w, map[string]any{"state": publicState(st), "caddyfile": string(b), "email": set.Email, "keptSites": kept})
}

func (a *App) render(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host *Host       `json:"host"`
		List *AccessList `json:"list"`
	}
	if err := readJSON(r, &req); err != nil {
		jerr(w, 400, "bad request")
		return
	}
	st, _ := a.snapshot()
	if req.List != nil {
		l := *req.List
		for i := range l.Users {
			if l.Users[i].Hash == "" {
				l.Users[i].Hash = "<hashed password>"
			}
		}
		jok(w, map[string]string{"text": renderList(l)})
		return
	}
	if req.Host == nil {
		jerr(w, 400, "bad request")
		return
	}
	h := *req.Host
	normalizeHost(&h)
	h.Domains = cleanList(h.Domains)
	if len(h.Domains) == 0 {
		h.Domains = []string{"example.com"}
	}
	if len(h.Upstreams) == 0 {
		h.Upstreams = []string{"localhost:3000"}
	}
	for i := range h.Access.Users {
		h.Access.Users[i].Hash = "<hashed password>"
	}
	jok(w, map[string]string{"text": renderHost(h, &st, a.dataDir)})
}

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail
	Detail string `json:"detail"`
}

func (a *App) checksFor(st State, cur State) []check {
	out := []check{}
	curDomains := map[string]bool{}
	for _, h := range cur.Hosts {
		if h.Enabled {
			for _, d := range h.Domains {
				curDomains[d] = true
			}
		}
	}
	type upRes struct {
		up string
		h  Health
	}
	var ups []string
	seen := map[string]bool{}
	var newDomains []string
	before := map[string]string{}
	for _, h := range cur.Hosts {
		b, _ := json.Marshal(h)
		before[h.ID] = string(b)
	}
	for _, h := range st.Hosts {
		if !h.Enabled {
			continue
		}
		// only check what this change touches
		if b, _ := json.Marshal(h); before[h.ID] == string(b) {
			continue
		}
		if h.Type == "proxy" {
			list := append([]string{}, h.Upstreams...)
			for _, p := range h.Paths {
				list = append(list, p.To)
			}
			for _, u := range list {
				if !seen[u] {
					seen[u] = true
					ups = append(ups, u)
				}
			}
		}
		for _, d := range h.Domains {
			if !curDomains[d] && h.Proto == "https" && (h.Cert == "auto") && !strings.HasPrefix(d, "*.") {
				newDomains = append(newDomains, d)
			}
		}
	}
	res := make([]upRes, len(ups))
	var wg sync.WaitGroup
	for i, u := range ups {
		wg.Add(1)
		go func(i int, u string) { defer wg.Done(); res[i] = upRes{u, checkUpstream(u)} }(i, u)
	}
	wg.Wait()
	down := []string{}
	for _, r := range res {
		a.mon.mu.Lock()
		a.mon.health[r.up] = r.h
		a.mon.mu.Unlock()
		if r.h.Status == "down" {
			down = append(down, r.up+" ("+r.h.Error+")")
		}
	}
	if len(ups) > 0 {
		if len(down) == 0 {
			out = append(out, check{"Upstreams answer", "ok", fmt.Sprintf("%d of %d reachable", len(ups), len(ups))})
		} else {
			out = append(out, check{"Upstreams answer", "warn", "Not reachable: " + strings.Join(down, ", ")})
		}
	}
	for _, d := range newDomains {
		host := d
		if h, _, err := net.SplitHostPort(d); err == nil {
			host = h
		}
		if net.ParseIP(host) != nil || host == "localhost" {
			continue
		}
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			out = append(out, check{"DNS for " + host, "warn", "No DNS record found — the certificate can't be issued until it points here"})
		} else {
			out = append(out, check{"DNS for " + host, "ok", "Points to " + strings.Join(addrs, ", ")})
		}
	}
	return out
}

func (a *App) hashPasswords(st *State) error {
	for i := range st.Hosts {
		for j := range st.Hosts[i].Access.Users {
			u := &st.Hosts[i].Access.Users[j]
			if u.Password != "" {
				h, err := a.caddy.Hash(u.Password)
				if err != nil {
					return err
				}
				u.Hash, u.Password = h, ""
			}
		}
	}
	for i := range st.AccessLists {
		for j := range st.AccessLists[i].Users {
			u := &st.AccessLists[i].Users[j]
			if u.Password != "" {
				h, err := a.caddy.Hash(u.Password)
				if err != nil {
					return err
				}
				u.Hash, u.Password = h, ""
			}
		}
	}
	return nil
}

func (a *App) preview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hosts       *[]Host       `json:"hosts"`
		AccessLists *[]AccessList `json:"accessLists"`
		Caddyfile   *string       `json:"caddyfile"` // manual upload
		Managed     bool          `json:"managed"`   // switch back from manual mode
		GlobalExtra *string       `json:"globalExtra"`
		ExtraConfig *string       `json:"extraConfig"`
		Import      *struct {
			Caddyfile        string `json:"caddyfile"`
			RewriteLocalhost bool   `json:"rewriteLocalhost"`
		} `json:"import"`
		Label string `json:"label"`
	}
	if err := readJSON(r, &req); err != nil {
		jerr(w, 400, "bad request")
		return
	}
	if strings.TrimSpace(req.Label) == "" {
		req.Label = "Config change"
	}
	cur, set := a.snapshot()
	next := cur
	setEmail := ""
	if req.Import != nil {
		res, err := importCaddyfile(req.Import.Caddyfile)
		if err != nil {
			jerr(w, 400, err.Error())
			return
		}
		hosts := []Host{}
		taken := map[string]bool{}
		for i := range res.Hosts {
			h := &res.Hosts[i]
			if req.Import.RewriteLocalhost {
				for j := range h.Upstreams {
					h.Upstreams[j] = rewriteLocal(h.Upstreams[j])
				}
				for j := range h.Paths {
					h.Paths[j].To = rewriteLocal(h.Paths[j].To)
				}
			}
			for _, d := range h.Domains {
				taken[d] = true
			}
			hosts = append(hosts, *h)
		}
		// keep existing hosts unless the import brings the same domain
		for _, h := range cur.Hosts {
			clash := false
			for _, d := range h.Domains {
				clash = clash || taken[d]
			}
			if !clash {
				hosts = append(hosts, h)
			}
		}
		next.Hosts = hosts
		next.GlobalExtra = res.GlobalExtra
		next.ExtraConfig = res.ExtraConfig
		if req.Import.RewriteLocalhost {
			next.ExtraConfig = rewriteLocalInConfig(next.ExtraConfig)
		}
		next.Manual, next.ManualCaddyfile = false, ""
		if res.Email != "" && set.Email == "" {
			setEmail = res.Email
			set.Email = res.Email
		}
		mergeHashes(&next, cur)
		if err := validateState(&next); err != nil {
			jerr(w, 400, err.Error())
			return
		}
	} else if req.Caddyfile != nil {
		next.Manual = true
		next.ManualCaddyfile = *req.Caddyfile
		if strings.TrimSpace(next.ManualCaddyfile) == "" {
			jerr(w, 400, "The uploaded Caddyfile is empty")
			return
		}
	} else {
		if req.Hosts != nil {
			next.Hosts = *req.Hosts
		}
		if req.AccessLists != nil {
			next.AccessLists = *req.AccessLists
		}
		if req.GlobalExtra != nil {
			next.GlobalExtra = *req.GlobalExtra
		}
		if req.ExtraConfig != nil {
			next.ExtraConfig = *req.ExtraConfig
		}
		if req.Hosts != nil || req.AccessLists != nil || req.Managed {
			next.Manual = false
			next.ManualCaddyfile = ""
		}
		mergeHashes(&next, cur)
		if err := validateState(&next); err != nil {
			jerr(w, 400, err.Error())
			return
		}
		if err := a.hashPasswords(&next); err != nil {
			jerr(w, 500, err.Error())
			return
		}
	}
	text := generate(&next, set, a.dataDir)
	currentText, _ := os.ReadFile(a.caddy.caddyfilePath())
	resp := map[string]any{"caddyfile": text, "current": string(currentText), "label": req.Label}
	checks := []check{}
	if err := a.caddy.Validate(text); err != nil {
		checks = append(checks, check{"Caddyfile is valid", "fail", err.Error()})
		resp["valid"] = false
		resp["error"] = err.Error()
	} else {
		checks = append(checks, check{"Caddyfile is valid", "ok", "caddy validate passed"})
		resp["valid"] = true
		if !next.Manual {
			checks = append(checks, a.checksFor(next, cur)...)
		}
	}
	resp["checks"] = checks
	if resp["valid"] == true {
		token := newID() + newID()
		a.pmu.Lock()
		for k, v := range a.pending {
			if time.Since(v.Created) > 30*time.Minute {
				delete(a.pending, k)
			}
		}
		a.pending[token] = pendingChange{State: next, Caddyfile: text, Label: req.Label, Created: time.Now(), SetEmail: setEmail}
		a.pmu.Unlock()
		resp["token"] = token
	}
	jok(w, resp)
}

func (a *App) analyzeImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Caddyfile string `json:"caddyfile"`
	}
	if err := readJSON(r, &req); err != nil {
		jerr(w, 400, "bad request")
		return
	}
	res, err := importCaddyfile(req.Caddyfile)
	if err != nil {
		jerr(w, 400, err.Error())
		return
	}
	_, inDocker := os.Stat("/.dockerenv")
	jok(w, map[string]any{"result": res, "inDocker": inDocker == nil})
}

func (a *App) apply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	readJSON(r, &req)
	a.pmu.Lock()
	p, ok := a.pending[req.Token]
	delete(a.pending, req.Token)
	a.pmu.Unlock()
	if !ok {
		jerr(w, 400, "This preview expired — review the change again")
		return
	}
	if err := a.applyChange(p.State, p.Caddyfile, p.Label); err != nil {
		jerr(w, 500, err.Error())
		return
	}
	if p.SetEmail != "" {
		a.store.mu.Lock()
		s := a.store.settings
		if s.Email == "" {
			s.Email = p.SetEmail
			a.store.saveSettings(s)
		}
		a.store.mu.Unlock()
	}
	go a.mon.CheckAll(p.State)
	jok(w, map[string]bool{"ok": true})
}

func (a *App) applyChange(next State, text, label string) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	cf := a.caddy.caddyfilePath()
	prevText, _ := os.ReadFile(cf)
	cur, _ := a.snapshot()
	a.backup(cur, string(prevText), label)
	if err := os.WriteFile(cf, []byte(text), 0600); err != nil {
		return err
	}
	if err := a.caddy.Reload(); err != nil {
		os.WriteFile(cf, prevText, 0600)
		a.caddy.Reload()
		a.caddy.note("error", "Apply failed, previous config kept: "+err.Error())
		return fmt.Errorf("Caddy rejected the change, so the previous config is still running: %s", err.Error())
	}
	a.store.mu.Lock()
	err := a.store.saveState(next)
	a.store.mu.Unlock()
	if err != nil {
		return err
	}
	if label == "" {
		label = "Config applied"
	}
	a.caddy.note("info", "Applied: "+label)
	return nil
}

// ---------------- backups ----------------

var reBackupID = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}(-[0-9]+)?$`)

func (a *App) backup(st State, caddyfile, label string) {
	id := time.Now().Format("20060102-150405")
	dir := filepath.Join(a.dataDir, "backups", id)
	for i := 1; ; i++ {
		if _, err := os.Stat(dir); err != nil {
			break
		}
		dir = filepath.Join(a.dataDir, "backups", fmt.Sprintf("%s-%d", id, i))
	}
	os.MkdirAll(dir, 0700)
	writeJSONFile(filepath.Join(dir, "state.json"), st)
	os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte(caddyfile), 0600)
	os.WriteFile(filepath.Join(dir, "label"), []byte("Before “"+label+"”"), 0600)
	entries, _ := os.ReadDir(filepath.Join(a.dataDir, "backups"))
	var ids []string
	for _, e := range entries {
		if e.IsDir() && reBackupID.MatchString(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	for len(ids) > 30 {
		os.RemoveAll(filepath.Join(a.dataDir, "backups", ids[0]))
		ids = ids[1:]
	}
}

func (a *App) listBackups(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(filepath.Join(a.dataDir, "backups"))
	out := []map[string]any{}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !e.IsDir() || !reBackupID.MatchString(e.Name()) {
			continue
		}
		label, _ := os.ReadFile(filepath.Join(a.dataDir, "backups", e.Name(), "label"))
		t, _ := time.ParseInLocation("20060102-150405", e.Name()[:15], time.Local)
		out = append(out, map[string]any{"id": e.Name(), "label": string(label), "ts": t.Unix()})
	}
	jok(w, out)
}

func (a *App) restoreBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	readJSON(r, &req)
	if !reBackupID.MatchString(req.ID) {
		jerr(w, 400, "bad backup id")
		return
	}
	dir := filepath.Join(a.dataDir, "backups", req.ID)
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		jerr(w, 404, "backup not found")
		return
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		jerr(w, 500, "backup is unreadable")
		return
	}
	text, _ := os.ReadFile(filepath.Join(dir, "Caddyfile"))
	cfText := string(text)
	if cfText == "" {
		_, set := a.snapshot()
		cfText = generate(&st, set, a.dataDir)
	}
	cur, _ := os.ReadFile(a.caddy.caddyfilePath())
	resp := map[string]any{"caddyfile": cfText, "current": string(cur), "label": "Restore backup " + req.ID}
	if err := a.caddy.Validate(cfText); err != nil {
		resp["valid"] = false
		resp["error"] = err.Error()
		resp["checks"] = []check{{"Caddyfile is valid", "fail", err.Error()}}
	} else {
		resp["valid"] = true
		resp["checks"] = []check{{"Caddyfile is valid", "ok", "caddy validate passed"}}
		token := newID() + newID()
		a.pmu.Lock()
		a.pending[token] = pendingChange{State: st, Caddyfile: cfText, Label: "Restore backup " + req.ID, Created: time.Now()}
		a.pmu.Unlock()
		resp["token"] = token
	}
	jok(w, resp)
}

func (a *App) downloadCaddyfile(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(a.caddy.caddyfilePath())
	if err != nil {
		jerr(w, 404, "no Caddyfile yet")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="Caddyfile"`)
	w.Write(b)
}

func (a *App) uploadCert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !regexp.MustCompile(`^[a-f0-9]{12}$`).MatchString(id) {
		jerr(w, 400, "bad host id")
		return
	}
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		jerr(w, 400, "upload a certificate and a key")
		return
	}
	read := func(name string) []byte {
		f, _, err := r.FormFile(name)
		if err != nil {
			return nil
		}
		defer f.Close()
		b, _ := io.ReadAll(io.LimitReader(f, 1<<20))
		return b
	}
	crt, key := read("cert"), read("key")
	if crt == nil || key == nil {
		jerr(w, 400, "choose both the certificate and the key file")
		return
	}
	if _, err := tls.X509KeyPair(crt, key); err != nil {
		jerr(w, 400, "the certificate and key don't match: "+err.Error())
		return
	}
	os.WriteFile(filepath.Join(a.dataDir, "certs", id+".crt"), crt, 0600)
	os.WriteFile(filepath.Join(a.dataDir, "certs", id+".key"), key, 0600)
	jok(w, map[string]bool{"ok": true})
}

// ---------------- status / overview ----------------

type hostStatus struct {
	Health     string `json:"health"` // up | slow | down | na
	Ms         int64  `json:"ms"`
	Error      string `json:"error,omitempty"`
	Cert       string `json:"cert"` // valid | expiring | failed | pending | http | none
	CertDays   int    `json:"certDays"`
	CertIssuer string `json:"certIssuer,omitempty"`
	CertError  string `json:"certError,omitempty"`
}

func (a *App) hostStatuses(st State) map[string]hostStatus {
	health := a.mon.Snapshot()
	certs := scanCerts(a.dataDir)
	events := a.caddy.Events()
	out := map[string]hostStatus{}
	now := time.Now()
	for _, h := range st.Hosts {
		hs := hostStatus{Health: "na"}
		if h.Type == "proxy" && h.Enabled {
			hs.Health = "up"
			for _, u := range h.Upstreams {
				if r, ok := health[u]; ok {
					if r.Status == "down" || (r.Status == "slow" && hs.Health == "up") {
						hs.Health = r.Status
						hs.Error = r.Error
					}
					if r.Ms > hs.Ms {
						hs.Ms = r.Ms
					}
				} else if hs.Health == "up" {
					hs.Health = "unknown"
				}
			}
		}
		if h.Proto == "http" {
			hs.Cert = "http"
		} else {
			best, bestFrom := int64(0), int64(0)
			for _, c := range certs {
				for _, n := range c.Names {
					for _, d := range h.Domains {
						dh := d
						if x, _, err := net.SplitHostPort(d); err == nil {
							dh = x
						}
						if (certMatches(n, dh) || n == dh) && c.NotAfter > best {
							if h.Cert == "internal" && c.Issuer != "local" {
								continue
							}
							best, bestFrom = c.NotAfter, c.NotBefore
							hs.CertIssuer = c.Issuer
						}
					}
				}
			}
			if best > 0 {
				left := time.Until(time.Unix(best, 0))
				life := time.Duration(best-bestFrom) * time.Second
				hs.CertDays = int(left.Hours() / 24)
				hs.Cert = "valid"
				// Caddy renews with about a third of the lifetime left; under a fifth means renewal is overdue
				if left < life/5 || left <= 0 {
					hs.Cert = "expiring"
				}
			} else {
				hs.Cert = "pending"
			}
			// recent certificate errors for this host's domains
			for i := len(events) - 1; i >= 0; i-- {
				e := events[i]
				if now.Sub(time.Unix(int64(e.TS), 0)) > 24*time.Hour {
					break
				}
				match := false
				for _, d := range h.Domains {
					if e.Ident != "" && (e.Ident == d || certMatches(d, e.Ident)) {
						match = true
					}
				}
				if !match {
					continue
				}
				if e.Level == "error" {
					if hs.Cert != "valid" {
						hs.Cert = "failed"
					}
					hs.CertError = strings.TrimSpace(e.Msg + ": " + e.Err)
				}
				break
			}
		}
		out[h.ID] = hs
	}
	return out
}

func (a *App) checkNow(w http.ResponseWriter, r *http.Request) {
	st, _ := a.snapshot()
	a.mon.CheckAll(st)
	jok(w, map[string]bool{"ok": true})
}

func (a *App) getStatus(w http.ResponseWriter, r *http.Request) {
	st, _ := a.snapshot()
	running, exitErr, since := a.caddy.Status()
	events := a.caddy.Events()
	if len(events) > 40 {
		events = events[len(events)-40:]
	}
	certs := scanCerts(a.dataDir)
	jok(w, map[string]any{
		"caddy":  map[string]any{"running": running, "error": exitErr, "since": since.Unix()},
		"hosts":  a.hostStatuses(st),
		"events": events,
		"certs":  len(certs),
		"manual": st.Manual,
	})
}

// ---------------- stats & logs ----------------

func rangeMs(r string) (int64, int64, int) {
	now := time.Now().UnixMilli()
	switch r {
	case "1h":
		return now - 3600_000, 5 * 60_000, 12
	case "7d":
		return now - 7*24*3600_000, 6 * 3600_000, 28
	}
	return now - 24*3600_000, 3600_000, 24
}

func (a *App) filterEntries(r *http.Request) ([]Entry, int64, int64, int, *Host) {
	st, _ := a.snapshot()
	from, step, n := rangeMs(r.URL.Query().Get("range"))
	entries := a.alog.Since(from)
	var host *Host
	if id := r.URL.Query().Get("host"); id != "" {
		for i := range st.Hosts {
			if st.Hosts[i].ID == id {
				host = &st.Hosts[i]
			}
		}
		if host != nil {
			f := entries[:0:0]
			for _, e := range entries {
				if hostMatches(host.Domains, e.Host) {
					f = append(f, e)
				}
			}
			entries = f
		}
	}
	return entries, from, step, n, host
}

type statsOut struct {
	Total   int              `json:"total"`
	Err5    int              `json:"err5"`
	Err4    int              `json:"err4"`
	Blocked int              `json:"blocked"`
	Median  int64            `json:"medianMs"`
	Bytes   int64            `json:"bytes"`
	Codes   map[string]int   `json:"codes"`
	Buckets []map[string]int `json:"buckets"`
	From    int64            `json:"from"`
	Step    int64            `json:"step"`
	PerHost []map[string]any `json:"perHost"`
}

func computeStats(entries []Entry, from, step int64, n int, st State) statsOut {
	s := statsOut{Codes: map[string]int{"2": 0, "3": 0, "4": 0, "5": 0}, From: from, Step: step}
	s.Buckets = make([]map[string]int, n)
	for i := range s.Buckets {
		s.Buckets[i] = map[string]int{"ok": 0, "err": 0}
	}
	durs := make([]int64, 0, len(entries))
	type agg struct {
		req, err int
		ms, by   int64
	}
	per := map[string]*agg{}
	for _, e := range entries {
		s.Total++
		s.Bytes += e.Size
		durs = append(durs, e.Ms)
		c := strconv.Itoa(e.Status / 100)
		s.Codes[c]++
		if e.Status >= 500 {
			s.Err5++
		} else if e.Status >= 400 {
			s.Err4++
		}
		if e.Status == 403 {
			s.Blocked++
		}
		bi := int((e.TS - from) / step)
		if bi >= n {
			bi = n - 1
		}
		if bi >= 0 {
			if e.Status >= 500 {
				s.Buckets[bi]["err"]++
			} else {
				s.Buckets[bi]["ok"]++
			}
		}
		for _, h := range st.Hosts {
			if hostMatches(h.Domains, e.Host) {
				g := per[h.ID]
				if g == nil {
					g = &agg{}
					per[h.ID] = g
				}
				g.req++
				g.ms += e.Ms
				g.by += e.Size
				if e.Status >= 500 {
					g.err++
				}
				break
			}
		}
	}
	if len(durs) > 0 {
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		s.Median = durs[len(durs)/2]
	}
	s.PerHost = []map[string]any{}
	for _, h := range st.Hosts {
		g := per[h.ID]
		if g == nil {
			g = &agg{}
		}
		avg := int64(0)
		if g.req > 0 {
			avg = g.ms / int64(g.req)
		}
		s.PerHost = append(s.PerHost, map[string]any{"id": h.ID, "domain": strings.Join(h.Domains, ", "), "req": g.req, "err": g.err, "avgMs": avg, "bytes": g.by})
	}
	sort.SliceStable(s.PerHost, func(i, j int) bool { return s.PerHost[i]["req"].(int) > s.PerHost[j]["req"].(int) })
	return s
}

func (a *App) stats(w http.ResponseWriter, r *http.Request) {
	entries, from, step, n, _ := a.filterEntries(r)
	st, _ := a.snapshot()
	jok(w, computeStats(entries, from, step, n, st))
}

func filterLines(entries []Entry, status, q string, limit int) []Entry {
	q = strings.ToLower(q)
	out := []Entry{}
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if status != "" && status != "all" && strconv.Itoa(e.Status/100) != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.URI+" "+e.IP+" "+e.Host+" "+e.Method), q) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (a *App) logs(w http.ResponseWriter, r *http.Request) {
	entries, _, _, _, _ := a.filterEntries(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 2000 {
		limit = 300
	}
	lines := filterLines(entries, r.URL.Query().Get("status"), r.URL.Query().Get("q"), limit)
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="access-log.txt"`)
		for _, e := range lines {
			fmt.Fprintf(w, "%s %d %s %s%s %s %dms %dB\n", time.UnixMilli(e.TS).Format(time.RFC3339), e.Status, e.Method, e.Host, e.URI, e.IP, e.Ms, e.Size)
		}
		return
	}
	jok(w, map[string]any{"lines": lines, "total": len(entries)})
}

// ---------------- settings ----------------

func (a *App) getSettings(w http.ResponseWriter, r *http.Request) {
	_, s := a.snapshot()
	jok(w, map[string]any{
		"email": s.Email, "hasDnsToken": s.DNSToken != "",
		"ai": map[string]any{"enabled": s.AI.Enabled, "provider": s.AI.Provider, "baseUrl": s.AI.BaseURL, "model": s.AI.Model, "maskIps": s.AI.MaskIPs, "hasKey": s.AI.APIKey != ""},
	})
}

func (a *App) saveSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email         *string `json:"email"`
		DNSToken      *string `json:"dnsToken"`
		ClearDNSToken bool    `json:"clearDnsToken"`
		AI            *struct {
			Enabled  bool   `json:"enabled"`
			Provider string `json:"provider"`
			BaseURL  string `json:"baseUrl"`
			APIKey   string `json:"apiKey"`
			ClearKey bool   `json:"clearKey"`
			Model    string `json:"model"`
			MaskIPs  bool   `json:"maskIps"`
		} `json:"ai"`
	}
	if err := readJSON(r, &req); err != nil {
		jerr(w, 400, "bad request")
		return
	}
	cur, s := a.snapshot()
	next := s
	emailChanged, tokenChanged := false, false
	if req.Email != nil {
		e := strings.TrimSpace(*req.Email)
		if e != "" && !regexp.MustCompile(`^[^@\s"{}]+@[^@\s"{}]+\.[^@\s"{}]+$`).MatchString(e) {
			jerr(w, 400, "That email address doesn't look right")
			return
		}
		emailChanged = e != s.Email
		next.Email = e
	}
	if req.ClearDNSToken {
		tokenChanged = s.DNSToken != ""
		next.DNSToken = ""
	} else if req.DNSToken != nil && strings.TrimSpace(*req.DNSToken) != "" {
		next.DNSToken = strings.TrimSpace(*req.DNSToken)
		tokenChanged = true
	}
	if req.AI != nil {
		p := req.AI.Provider
		if p != "anthropic" && p != "openai" && p != "compat" && p != "ollama" {
			jerr(w, 400, "unknown provider")
			return
		}
		if req.AI.BaseURL != "" && !regexp.MustCompile(`^https?://[^\s]+$`).MatchString(req.AI.BaseURL) {
			jerr(w, 400, "Base URL must start with http:// or https://")
			return
		}
		key := s.AI.APIKey
		if req.AI.ClearKey {
			key = ""
		} else if strings.TrimSpace(req.AI.APIKey) != "" {
			key = strings.TrimSpace(req.AI.APIKey)
		}
		next.AI = AISettings{Enabled: req.AI.Enabled, Provider: p, BaseURL: strings.TrimSpace(req.AI.BaseURL), APIKey: key, Model: strings.TrimSpace(req.AI.Model), MaskIPs: req.AI.MaskIPs}
	}
	if emailChanged && !cur.Manual {
		text := generate(&cur, next, a.dataDir)
		if err := a.caddy.Validate(text); err != nil {
			jerr(w, 400, "Caddy rejected the email: "+err.Error())
			return
		}
		a.store.mu.Lock()
		err := a.store.saveSettings(next)
		a.store.mu.Unlock()
		if err != nil {
			jerr(w, 500, err.Error())
			return
		}
		if err := a.applyChange(cur, text, "ACME email changed"); err != nil {
			jerr(w, 500, err.Error())
			return
		}
	} else {
		a.store.mu.Lock()
		err := a.store.saveSettings(next)
		a.store.mu.Unlock()
		if err != nil {
			jerr(w, 500, err.Error())
			return
		}
	}
	if tokenChanged {
		a.caddy.setEnv(next.DNSToken)
		if err := a.caddy.Restart(); err != nil {
			jerr(w, 500, "saved, but Caddy failed to restart: "+err.Error())
			return
		}
		a.caddy.note("info", "DNS token updated, Caddy restarted")
	}
	jok(w, map[string]bool{"ok": true})
}

func (a *App) testAI(w http.ResponseWriter, r *http.Request) {
	_, s := a.snapshot()
	t := time.Now()
	out, err := callAI(s.AI, "Reply with the single word OK.", "ping", 16)
	if err != nil {
		jerr(w, 400, err.Error())
		return
	}
	jok(w, map[string]any{"ok": true, "reply": strings.TrimSpace(out), "ms": time.Since(t).Milliseconds()})
}
