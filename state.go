package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type PathRoute struct {
	Path  string `json:"path"`
	To    string `json:"to"`
	Strip bool   `json:"strip"`
}

type User struct {
	Name     string `json:"name"`
	Hash     string `json:"hash,omitempty"`
	Password string `json:"password,omitempty"` // plaintext, only in requests; never stored
}

type Rule struct {
	Act  string `json:"act"` // allow | deny
	CIDR string `json:"cidr"`
}

type Access struct {
	Mode    string `json:"mode"` // none | list | custom
	ListID  string `json:"listId"`
	Users   []User `json:"users"`
	Rules   []Rule `json:"rules"`
	Satisfy string `json:"satisfy"` // any | all
}

type Header struct {
	Kind  string `json:"kind"` // res | req
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Host struct {
	ID           string      `json:"id"`
	Type         string      `json:"type"` // proxy | redirect | dead
	Enabled      bool        `json:"enabled"`
	Domains      []string    `json:"domains"`
	Upstreams    []string    `json:"upstreams"`
	LB           string      `json:"lb"`
	HealthPath   string      `json:"healthPath"`
	RedirectTo   string      `json:"redirectTo"`
	RedirectCode int         `json:"redirectCode"`
	KeepPath     bool        `json:"keepPath"`
	DeadCode     string      `json:"deadCode"` // 404 | 410 | abort
	DeadMsg      string      `json:"deadMsg"`
	Paths        []PathRoute `json:"paths"`
	Access       Access      `json:"access"`
	Proto        string      `json:"proto"` // https | http
	Cert         string      `json:"cert"`  // auto | dns | internal | custom
	ForceHTTPS   bool        `json:"forceHttps"`
	HSTS         bool        `json:"hsts"`
	Exploits     bool        `json:"exploits"`
	Gzip         bool        `json:"gzip"`
	Cache        bool        `json:"cache"`
	Maintenance  bool        `json:"maintenance"`
	Headers      []Header    `json:"headers"`
	Raw          string      `json:"raw"`
}

type AccessList struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Users   []User `json:"users"`
	Rules   []Rule `json:"rules"`
	Satisfy string `json:"satisfy"`
	Pass    bool   `json:"pass"`
}

type State struct {
	Hosts           []Host       `json:"hosts"`
	AccessLists     []AccessList `json:"accessLists"`
	Manual          bool         `json:"manual"`
	ManualCaddyfile string       `json:"manualCaddyfile,omitempty"`
}

type AISettings struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"` // anthropic | openai | compat | ollama
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey,omitempty"`
	Model    string `json:"model"`
	MaskIPs  bool   `json:"maskIps"`
}

type Settings struct {
	Email    string     `json:"email"`
	DNSToken string     `json:"dnsToken,omitempty"` // Cloudflare API token
	AI       AISettings `json:"ai"`
}

type Store struct {
	mu       sync.Mutex
	dir      string
	state    State
	settings Settings
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) load() error {
	s.state = State{Hosts: []Host{}, AccessLists: []AccessList{}}
	if b, err := os.ReadFile(filepath.Join(s.dir, "state.json")); err == nil {
		if err := json.Unmarshal(b, &s.state); err != nil {
			return fmt.Errorf("state.json: %w", err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(s.dir, "settings.json")); err == nil {
		if err := json.Unmarshal(b, &s.settings); err != nil {
			return fmt.Errorf("settings.json: %w", err)
		}
	}
	if s.state.Hosts == nil {
		s.state.Hosts = []Host{}
	}
	if s.state.AccessLists == nil {
		s.state.AccessLists = []AccessList{}
	}
	return nil
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) saveState(st State) error {
	if err := writeJSONFile(filepath.Join(s.dir, "state.json"), st); err != nil {
		return err
	}
	s.state = st
	return nil
}

func (s *Store) saveSettings(st Settings) error {
	if err := writeJSONFile(filepath.Join(s.dir, "settings.json"), st); err != nil {
		return err
	}
	s.settings = st
	return nil
}

// ---------- validation (also guards against Caddyfile injection) ----------

var (
	reDomain   = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9\-\.]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)
	reUpstream = regexp.MustCompile(`^(https?://)?[A-Za-z0-9\.\-_\[\]:]+$`)
	rePath     = regexp.MustCompile(`^/[A-Za-z0-9\._~\-/]*$`)
	reHdrName  = regexp.MustCompile(`^[A-Za-z0-9\-]+$`)
	reCIDR     = regexp.MustCompile(`^([0-9a-fA-F:\.]+(/[0-9]{1,3})?|private_ranges)$`)
	reUser     = regexp.MustCompile(`^[A-Za-z0-9\._@\-]+$`)
	reURL      = regexp.MustCompile(`^https?://[^\s"{}]+$`)
	reHealth   = regexp.MustCompile(`^/[A-Za-z0-9\._~\-/?=&]*$`)
)

func noBad(s string) bool { return !strings.ContainsAny(s, "\n\r\"`") }

func cleanList(in []string) []string {
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func validateUsersRules(users []User, rules []Rule, where string) error {
	seen := map[string]bool{}
	for _, u := range users {
		if !reUser.MatchString(u.Name) {
			return fmt.Errorf("%s: username %q may only use letters, numbers, . _ @ -", where, u.Name)
		}
		if seen[u.Name] {
			return fmt.Errorf("%s: username %q is listed twice", where, u.Name)
		}
		seen[u.Name] = true
		if u.Hash == "" && u.Password == "" {
			return fmt.Errorf("%s: set a password for %q", where, u.Name)
		}
	}
	for _, r := range rules {
		if r.Act != "allow" && r.Act != "deny" {
			return fmt.Errorf("%s: rule must be allow or deny", where)
		}
		if !reCIDR.MatchString(r.CIDR) {
			return fmt.Errorf("%s: %q is not an IP address or range", where, r.CIDR)
		}
	}
	return nil
}

func normalizeHost(h *Host) {
	h.Domains = cleanList(h.Domains)
	h.Upstreams = cleanList(h.Upstreams)
	for i := range h.Domains {
		h.Domains[i] = strings.ToLower(h.Domains[i])
	}
	if h.Type == "" {
		h.Type = "proxy"
	}
	if h.Proto == "" {
		h.Proto = "https"
	}
	if h.Cert == "" {
		h.Cert = "auto"
	}
	if h.LB == "" {
		h.LB = "round_robin"
	}
	if h.RedirectCode == 0 {
		h.RedirectCode = 301
	}
	if h.DeadCode == "" {
		h.DeadCode = "404"
	}
	if h.Access.Mode == "" {
		h.Access.Mode = "none"
	}
	if h.Access.Satisfy == "" {
		h.Access.Satisfy = "any"
	}
	if h.Paths == nil {
		h.Paths = []PathRoute{}
	}
	if h.Headers == nil {
		h.Headers = []Header{}
	}
	if h.Access.Users == nil {
		h.Access.Users = []User{}
	}
	if h.Access.Rules == nil {
		h.Access.Rules = []Rule{}
	}
	if h.ID == "" {
		h.ID = newID()
	}
}

func validateState(st *State) error {
	seenDomain := map[string]string{}
	listIDs := map[string]bool{}
	for i := range st.AccessLists {
		l := &st.AccessLists[i]
		if l.ID == "" {
			l.ID = newID()
		}
		if l.Satisfy == "" {
			l.Satisfy = "any"
		}
		if l.Users == nil {
			l.Users = []User{}
		}
		if l.Rules == nil {
			l.Rules = []Rule{}
		}
		l.Name = strings.TrimSpace(l.Name)
		if l.Name == "" {
			return errors.New("every access list needs a name")
		}
		if err := validateUsersRules(l.Users, l.Rules, "Access list "+l.Name); err != nil {
			return err
		}
		listIDs[l.ID] = true
	}
	for i := range st.Hosts {
		h := &st.Hosts[i]
		normalizeHost(h)
		name := "Host"
		if len(h.Domains) > 0 {
			name = h.Domains[0]
		}
		if len(h.Domains) == 0 {
			return errors.New("every host needs at least one domain")
		}
		for _, d := range h.Domains {
			if !reDomain.MatchString(d) {
				return fmt.Errorf("%s: %q is not a valid domain", name, d)
			}
			if other, ok := seenDomain[d]; ok && h.Enabled {
				return fmt.Errorf("%s is used by two enabled hosts (%s)", d, other)
			}
			if h.Enabled {
				seenDomain[d] = name
			}
		}
		switch h.Type {
		case "proxy":
			if len(h.Upstreams) == 0 {
				return fmt.Errorf("%s: add an address to forward to", name)
			}
			for _, u := range h.Upstreams {
				if !reUpstream.MatchString(u) {
					return fmt.Errorf("%s: %q is not a valid upstream (use host:port)", name, u)
				}
			}
			switch h.LB {
			case "round_robin", "least_conn", "ip_hash", "first":
			default:
				return fmt.Errorf("%s: unknown balancing method", name)
			}
			if len(h.Upstreams) > 1 && h.HealthPath != "" && !reHealth.MatchString(h.HealthPath) {
				return fmt.Errorf("%s: health check path must start with /", name)
			}
			for _, p := range h.Paths {
				if !rePath.MatchString(p.Path) || p.Path == "/" {
					return fmt.Errorf("%s: path %q must start with / (e.g. /api)", name, p.Path)
				}
				if !reUpstream.MatchString(p.To) {
					return fmt.Errorf("%s: %q is not a valid upstream for %s", name, p.To, p.Path)
				}
			}
		case "redirect":
			if !reURL.MatchString(h.RedirectTo) {
				return fmt.Errorf("%s: redirect target must be a full URL like https://example.com", name)
			}
			switch h.RedirectCode {
			case 301, 302, 307, 308:
			default:
				return fmt.Errorf("%s: redirect code must be 301, 302, 307 or 308", name)
			}
		case "dead":
			if h.DeadCode != "404" && h.DeadCode != "410" && h.DeadCode != "abort" {
				return fmt.Errorf("%s: unknown response", name)
			}
			if !noBad(h.DeadMsg) {
				return fmt.Errorf("%s: message can't contain quotes or new lines", name)
			}
		default:
			return fmt.Errorf("%s: unknown host type", name)
		}
		switch h.Access.Mode {
		case "none":
		case "list":
			if !listIDs[h.Access.ListID] {
				return fmt.Errorf("%s: choose an access list", name)
			}
		case "custom":
			if err := validateUsersRules(h.Access.Users, h.Access.Rules, name); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unknown access mode", name)
		}
		if h.Proto != "https" && h.Proto != "http" {
			return fmt.Errorf("%s: protocol must be https or http", name)
		}
		switch h.Cert {
		case "auto", "dns", "internal", "custom":
		default:
			return fmt.Errorf("%s: unknown certificate type", name)
		}
		for _, hd := range h.Headers {
			if !reHdrName.MatchString(hd.Name) {
				return fmt.Errorf("%s: header name %q may only use letters, numbers and -", name, hd.Name)
			}
			if !noBad(hd.Value) {
				return fmt.Errorf("%s: header %s value can't contain quotes or new lines", name, hd.Name)
			}
			if hd.Kind != "res" && hd.Kind != "req" {
				return fmt.Errorf("%s: header direction must be res or req", name)
			}
		}
	}
	return nil
}

// keep existing password hashes when the client didn't send a new password
func mergeHashes(next *State, cur State) {
	old := map[string]string{}
	for _, h := range cur.Hosts {
		for _, u := range h.Access.Users {
			old["h:"+h.ID+":"+u.Name] = u.Hash
		}
	}
	for _, l := range cur.AccessLists {
		for _, u := range l.Users {
			old["l:"+l.ID+":"+u.Name] = u.Hash
		}
	}
	for i := range next.Hosts {
		for j := range next.Hosts[i].Access.Users {
			u := &next.Hosts[i].Access.Users[j]
			if u.Password == "" && u.Hash == "" {
				u.Hash = old["h:"+next.Hosts[i].ID+":"+u.Name]
			}
		}
	}
	for i := range next.AccessLists {
		for j := range next.AccessLists[i].Users {
			u := &next.AccessLists[i].Users[j]
			if u.Password == "" && u.Hash == "" {
				u.Hash = old["l:"+next.AccessLists[i].ID+":"+u.Name]
			}
		}
	}
}

// publicState strips password hashes before sending to the browser
func publicState(st State) State {
	b, _ := json.Marshal(st)
	var out State
	json.Unmarshal(b, &out)
	for i := range out.Hosts {
		for j := range out.Hosts[i].Access.Users {
			out.Hosts[i].Access.Users[j].Hash = ""
		}
	}
	for i := range out.AccessLists {
		for j := range out.AccessLists[i].Users {
			out.AccessLists[i].Users[j].Hash = ""
		}
	}
	return out
}
