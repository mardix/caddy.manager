package main

import (
	"bufio"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------- upstream health ----------------

type Health struct {
	Status  string `json:"status"` // up | slow | down
	Ms      int64  `json:"ms"`
	Error   string `json:"error,omitempty"`
	Checked int64  `json:"checked"`
}

type Monitor struct {
	mu     sync.Mutex
	health map[string]Health // key: upstream address
}

func upstreamAddr(u string) string {
	if strings.Contains(u, "://") {
		if p, err := url.Parse(u); err == nil {
			host := p.Host
			if p.Port() == "" {
				if p.Scheme == "https" {
					host += ":443"
				} else {
					host += ":80"
				}
			}
			return host
		}
	}
	if _, _, err := net.SplitHostPort(u); err != nil {
		return u + ":80"
	}
	return u
}

func checkUpstream(u string) Health {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", upstreamAddr(u), 3*time.Second)
	ms := time.Since(start).Milliseconds()
	h := Health{Ms: ms, Checked: time.Now().Unix()}
	if err != nil {
		h.Status = "down"
		msg := err.Error()
		if i := strings.LastIndex(msg, ": "); i > 0 {
			msg = msg[i+2:]
		}
		h.Error = msg
		return h
	}
	conn.Close()
	h.Status = "up"
	if ms > 1000 {
		h.Status = "slow"
	}
	return h
}

func (m *Monitor) CheckAll(st State) {
	seen := map[string]bool{}
	var wg sync.WaitGroup
	for _, h := range st.Hosts {
		if h.Type != "proxy" || !h.Enabled {
			continue
		}
		ups := append([]string{}, h.Upstreams...)
		for _, p := range h.Paths {
			ups = append(ups, p.To)
		}
		for _, u := range ups {
			if seen[u] {
				continue
			}
			seen[u] = true
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				r := checkUpstream(u)
				m.mu.Lock()
				m.health[u] = r
				m.mu.Unlock()
			}(u)
		}
	}
	wg.Wait()
}

func (m *Monitor) Snapshot() map[string]Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]Health{}
	for k, v := range m.health {
		out[k] = v
	}
	return out
}

// ---------------- certificates ----------------

type CertInfo struct {
	Names     []string `json:"names"`
	NotBefore int64    `json:"notBefore"`
	NotAfter  int64    `json:"notAfter"`
	Issuer    string   `json:"issuer"`
}

// caddyStorage follows Caddy's own rule: $XDG_DATA_HOME/caddy, else ~/.local/share/caddy
func caddyStorage() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "caddy")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "caddy")
}

func scanCerts(dataDir string) []CertInfo {
	root := filepath.Join(caddyStorage(), "certificates")
	out := []CertInfo{}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".crt") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil
		}
		c, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		issuer := strings.Split(rel, string(filepath.Separator))[0]
		names := c.DNSNames
		for _, ip := range c.IPAddresses {
			names = append(names, ip.String())
		}
		out = append(out, CertInfo{Names: names, NotBefore: c.NotBefore.Unix(), NotAfter: c.NotAfter.Unix(), Issuer: issuer})
		return nil
	})
	// custom uploaded certs
	filepath.WalkDir(filepath.Join(dataDir, "certs"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".crt") {
			return nil
		}
		b, _ := os.ReadFile(p)
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil
		}
		if c, err := x509.ParseCertificate(blk.Bytes); err == nil {
			out = append(out, CertInfo{Names: c.DNSNames, NotBefore: c.NotBefore.Unix(), NotAfter: c.NotAfter.Unix(), Issuer: "custom"})
		}
		return nil
	})
	return out
}

func certMatches(name, domain string) bool {
	if name == domain {
		return true
	}
	if strings.HasPrefix(name, "*.") {
		i := strings.Index(domain, ".")
		return i > 0 && domain[i:] == name[1:]
	}
	return false
}

// ---------------- access log ----------------

type Entry struct {
	TS     int64  `json:"ts"` // unix ms
	Host   string `json:"host"`
	Method string `json:"method"`
	URI    string `json:"uri"`
	IP     string `json:"ip"`
	Status int    `json:"status"`
	Ms     int64  `json:"ms"`
	Size   int64  `json:"size"`
	Proto  string `json:"proto,omitempty"`
	UA     string `json:"ua,omitempty"`
	Ref    string `json:"ref,omitempty"`
}

type AccessLog struct {
	mu      sync.Mutex
	path    string
	offset  int64
	entries []Entry
}

const maxEntries = 300000

func (a *AccessLog) refresh() {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.Open(a.path)
	if err != nil {
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return
	}
	if fi.Size() < a.offset { // rotated
		a.offset = 0
	}
	if fi.Size() == a.offset {
		return
	}
	if a.offset == 0 && fi.Size() > 60<<20 {
		a.offset = fi.Size() - 60<<20
	}
	f.Seek(a.offset, io.SeekStart)
	r := bufio.NewReaderSize(f, 256*1024)
	var read int64
	for {
		line, err := r.ReadBytes('\n')
		if err != nil { // partial line: leave for next time
			break
		}
		read += int64(len(line))
		var raw struct {
			TS      float64 `json:"ts"`
			Request struct {
				Host     string              `json:"host"`
				Method   string              `json:"method"`
				URI      string              `json:"uri"`
				RemoteIP string              `json:"remote_ip"`
				ClientIP string              `json:"client_ip"`
				Proto    string              `json:"proto"`
				Headers  map[string][]string `json:"headers"`
			} `json:"request"`
			Duration float64 `json:"duration"`
			Size     int64   `json:"size"`
			Status   int     `json:"status"`
		}
		if json.Unmarshal(line, &raw) != nil || raw.Request.Method == "" {
			continue
		}
		ip := raw.Request.ClientIP
		if ip == "" {
			ip = raw.Request.RemoteIP
		}
		host := raw.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		a.entries = append(a.entries, Entry{
			TS: int64(raw.TS * 1000), Host: strings.ToLower(host), Method: raw.Request.Method, URI: raw.Request.URI,
			IP: ip, Status: raw.Status, Ms: int64(raw.Duration * 1000), Size: raw.Size, Proto: raw.Request.Proto,
			UA: clip(first(raw.Request.Headers["User-Agent"]), 300), Ref: clip(first(raw.Request.Headers["Referer"]), 300),
		})
	}
	a.offset += read
	if len(a.entries) > maxEntries {
		a.entries = append([]Entry{}, a.entries[len(a.entries)-maxEntries:]...)
	}
}

func (a *AccessLog) Since(ms int64) []Entry {
	a.refresh()
	a.mu.Lock()
	defer a.mu.Unlock()
	i := sort.Search(len(a.entries), func(i int) bool { return a.entries[i].TS >= ms })
	out := make([]Entry, len(a.entries)-i)
	copy(out, a.entries[i:])
	return out
}

func hostMatches(domains []string, host string) bool {
	for _, d := range domains {
		if h, _, err := net.SplitHostPort(d); err == nil {
			d = h
		}
		if certMatches(d, host) {
			return true
		}
	}
	return false
}

func first(v []string) string {
	if len(v) > 0 {
		return v[0]
	}
	return ""
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
