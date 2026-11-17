package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var aiClient = &http.Client{Timeout: 120 * time.Second}

func aiBaseURL(s AISettings) string {
	if b := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/"); b != "" {
		return b
	}
	switch s.Provider {
	case "anthropic":
		return "https://api.anthropic.com"
	case "openai":
		return "https://api.openai.com/v1"
	case "ollama":
		return "http://host.docker.internal:11434/v1"
	}
	return ""
}

// callAI sends one system + user message and returns the text reply.
func callAI(s AISettings, system, user string, maxTokens int) (string, error) {
	if !s.Enabled {
		return "", errors.New("the AI assistant is turned off in Settings")
	}
	if strings.TrimSpace(s.Model) == "" {
		return "", errors.New("set a model in Settings → AI assistant")
	}
	base := aiBaseURL(s)
	if base == "" {
		return "", errors.New("set a base URL in Settings → AI assistant")
	}
	var req *http.Request
	if s.Provider == "anthropic" {
		if s.APIKey == "" {
			return "", errors.New("add your Anthropic API key in Settings")
		}
		body, _ := json.Marshal(map[string]any{
			"model": s.Model, "max_tokens": maxTokens, "system": system,
			"messages": []map[string]string{{"role": "user", "content": user}},
		})
		req, _ = http.NewRequest("POST", base+"/v1/messages", bytes.NewReader(body))
		req.Header.Set("x-api-key", s.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		body, _ := json.Marshal(map[string]any{
			"model": s.Model, "max_tokens": maxTokens,
			"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		})
		req, _ = http.NewRequest("POST", base+"/chat/completions", bytes.NewReader(body))
		if s.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+s.APIKey)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := aiClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("couldn't reach the model provider: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(b, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = strings.TrimSpace(string(b))
			if len(msg) > 300 {
				msg = msg[:300]
			}
		}
		return "", fmt.Errorf("provider returned %d: %s", resp.StatusCode, msg)
	}
	if s.Provider == "anthropic" {
		var r struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return "", errors.New("unexpected reply from provider")
		}
		var out []string
		for _, c := range r.Content {
			if c.Type == "text" {
				out = append(out, c.Text)
			}
		}
		return strings.Join(out, "\n"), nil
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &r); err != nil || len(r.Choices) == 0 {
		return "", errors.New("unexpected reply from provider")
	}
	return r.Choices[0].Message.Content, nil
}

var reIPv4 = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
var reHash = regexp.MustCompile(`\$2[aby]?\$\d\d\$[./A-Za-z0-9]{53}`)

// redact removes password hashes always, and masks IPs when asked.
func redact(text string, maskIPs bool) string {
	text = reHash.ReplaceAllString(text, "[hash removed]")
	if maskIPs {
		seen := map[string]string{}
		text = reIPv4.ReplaceAllStringFunc(text, func(ip string) string {
			if strings.HasPrefix(ip, "127.") {
				return ip
			}
			if v, ok := seen[ip]; ok {
				return v
			}
			v := fmt.Sprintf("ip-%d", len(seen)+1)
			seen[ip] = v
			return v
		})
	}
	return text
}

const aiSystemBase = `You are the assistant inside a small web UI that manages a Caddy reverse proxy for one person.
Be concise and concrete: short paragraphs or a few bullets, plain language, no preamble.
Never invent data you weren't given. If the data doesn't answer the question, say so.
You cannot change anything yourself; when a fix needs a config change, say which setting in the UI to change.`
