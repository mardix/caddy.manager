package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Event struct {
	TS    float64 `json:"ts"`
	Level string  `json:"level"`
	Msg   string  `json:"msg"`
	Ident string  `json:"identifier,omitempty"`
	Err   string  `json:"error,omitempty"`
}

type Caddy struct {
	bin     string
	dataDir string
	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	exitErr string
	started time.Time
	env     []string

	evMu   sync.Mutex
	events []Event
	evFile *os.File
}

func (c *Caddy) caddyfilePath() string { return filepath.Join(c.dataDir, "Caddyfile") }

func (c *Caddy) setEnv(dnsToken string) {
	c.env = append(os.Environ(), "CF_API_TOKEN="+dnsToken)
}

func (c *Caddy) addEvent(e Event) {
	c.evMu.Lock()
	defer c.evMu.Unlock()
	c.events = append(c.events, e)
	if len(c.events) > 1000 {
		c.events = c.events[len(c.events)-1000:]
	}
}

func (c *Caddy) Events() []Event {
	c.evMu.Lock()
	defer c.evMu.Unlock()
	out := make([]Event, len(c.events))
	copy(out, c.events)
	return out
}

func (c *Caddy) note(level, msg string) {
	e := Event{TS: float64(time.Now().UnixNano()) / 1e9, Level: level, Msg: msg}
	c.addEvent(e)
	c.persist(e)
}

func (c *Caddy) persist(e Event) {
	if c.evFile == nil {
		return
	}
	b, _ := json.Marshal(e)
	c.evFile.Write(append(b, '\n'))
}

// keep the interesting caddy log lines (errors, certificates, config)
func interesting(e Event) bool {
	m := strings.ToLower(e.Msg)
	for _, k := range []string{"skipped because", "listening only on the http port", "already trusted", "installed properly", "grace period", "exiting", "byeee"} {
		if strings.Contains(m, k) {
			return false
		}
	}
	if e.Level == "error" || e.Level == "fatal" {
		return true
	}
	for _, k := range []string{"certificate obtained", "obtaining certificate", "could not get certificate", "renew", "failed"} {
		if strings.Contains(m, k) {
			return true
		}
	}
	return e.Level == "warn"
}

func (c *Caddy) loadEvents() {
	p := filepath.Join(c.dataDir, "logs", "events.log")
	if b, err := os.ReadFile(p); err == nil {
		lines := strings.Split(string(b), "\n")
		if len(lines) > 1000 {
			lines = lines[len(lines)-1000:]
		}
		for _, l := range lines {
			var e Event
			if json.Unmarshal([]byte(l), &e) == nil && e.Msg != "" {
				c.events = append(c.events, e)
			}
		}
		// trim the file occasionally
		if len(b) > 2<<20 {
			os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0600)
		}
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		c.evFile = f
	}
}

func (c *Caddy) pipeLogs(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var raw map[string]any
		if json.Unmarshal(line, &raw) != nil {
			log.Printf("caddy: %s", line)
			continue
		}
		e := Event{}
		e.TS, _ = raw["ts"].(float64)
		e.Level, _ = raw["level"].(string)
		e.Msg, _ = raw["msg"].(string)
		e.Ident, _ = raw["identifier"].(string)
		if e.Ident == "" {
			if d, ok := raw["domain"].(string); ok {
				e.Ident = d
			}
		}
		e.Err, _ = raw["error"].(string)
		logger, _ := raw["logger"].(string)
		// per-request errors are already in the access log; health-check chatter is noise
		noisy := strings.HasPrefix(logger, "http.log.error") || strings.HasPrefix(logger, "http.handlers.reverse_proxy") || strings.Contains(e.Msg, "skipped because it requires TLS")
		if !noisy && interesting(e) {
			c.addEvent(e)
			c.persist(e)
		}
		if e.Level == "error" || e.Level == "warn" {
			log.Printf("caddy %s: %s %s %s", e.Level, e.Msg, e.Ident, e.Err)
		}
	}
}

func (c *Caddy) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startLocked()
}

func (c *Caddy) startLocked() error {
	cmd := exec.Command(c.bin, "run", "--config", c.caddyfilePath(), "--adapter", "caddyfile")
	cmd.Env = c.env
	cmd.Dir = c.dataDir
	stderr, _ := cmd.StderrPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	c.running = true
	c.exitErr = ""
	c.started = time.Now()
	go c.pipeLogs(stderr)
	go c.pipeLogs(stdout)
	go func(cmd *exec.Cmd) {
		err := cmd.Wait()
		c.mu.Lock()
		if c.cmd == cmd {
			c.running = false
			if err != nil {
				c.exitErr = err.Error()
			} else {
				c.exitErr = "exited"
			}
			c.mu.Unlock()
			c.note("error", "Caddy stopped: "+c.exitErr)
			return
		}
		c.mu.Unlock()
	}(cmd)
	return nil
}

func (c *Caddy) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}

func (c *Caddy) stopLocked() {
	if c.cmd != nil && c.running {
		cmd := c.cmd
		c.cmd = nil
		c.running = false
		cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
		}
	}
}

func (c *Caddy) Restart() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
	return c.startLocked()
}

func (c *Caddy) Status() (bool, string, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running, c.exitErr, c.started
}

func (c *Caddy) run(timeout time.Duration, stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Env = c.env
	cmd.Dir = c.dataDir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// cleanErr pulls the human-readable error out of caddy's CLI output
func cleanErr(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var msgs []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(l), &raw) == nil {
			if lv, _ := raw["level"].(string); lv == "error" || lv == "fatal" {
				if e, ok := raw["error"].(string); ok {
					msgs = append(msgs, e)
				} else if m, ok := raw["msg"].(string); ok {
					msgs = append(msgs, m)
				}
			}
			continue
		}
		if strings.HasPrefix(l, "Error:") || strings.Contains(l, "Error:") {
			msgs = append(msgs, strings.TrimSpace(strings.TrimPrefix(l, "Error:")))
		}
	}
	if len(msgs) == 0 {
		if len(lines) > 0 {
			return lines[len(lines)-1]
		}
		return "unknown error"
	}
	return strings.Join(msgs, "\n")
}

func (c *Caddy) Validate(caddyfile string) error {
	tmp := filepath.Join(c.dataDir, "Caddyfile.candidate")
	if err := os.WriteFile(tmp, []byte(caddyfile), 0600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	out, err := c.run(60*time.Second, "", "validate", "--config", tmp, "--adapter", "caddyfile")
	if err != nil {
		return errors.New(cleanErr(out))
	}
	return nil
}

func (c *Caddy) Reload() error {
	running, _, _ := c.Status()
	if !running {
		return c.Start()
	}
	out, err := c.run(60*time.Second, "", "reload", "--config", c.caddyfilePath(), "--adapter", "caddyfile", "--address", "localhost:2019", "--force")
	if err != nil {
		return errors.New(cleanErr(out))
	}
	return nil
}

func (c *Caddy) Hash(password string) (string, error) {
	out, err := c.run(30*time.Second, password, "hash-password", "--algorithm", "bcrypt", "--bcrypt-cost", "12")
	if err != nil {
		// older caddy: no algorithm flags
		out, err = c.run(30*time.Second, password, "hash-password")
		if err != nil {
			return "", fmt.Errorf("hash-password: %s", cleanErr(out))
		}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	h := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(h, "$2") {
		return "", fmt.Errorf("hash-password: unexpected output")
	}
	return h, nil
}
