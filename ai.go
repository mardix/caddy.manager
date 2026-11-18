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
	if s.Provider == "anthropic" && s.APIKey == "" {
		return "", errors.New("add your Anthropic API key in Settings")
	}
	// OpenAI's newer models only accept max_completion_tokens; most other OpenAI-style servers want max_tokens.
	tokenParam := "max_tokens"
	if s.Provider == "openai" {
		tokenParam = "max_completion_tokens"
	}
	b, status, err := sendAI(s, base, system, user, maxTokens, tokenParam)
	if err == nil && status == 400 && s.Provider != "anthropic" {
		msg := string(b)
		if strings.Contains(msg, "max_tokens") || strings.Contains(msg, "max_completion_tokens") {
			other := "max_completion_tokens"
			if tokenParam == other {
				other = "max_tokens"
			}
			b, status, err = sendAI(s, base, system, user, maxTokens, other)
		}
	}
	if err != nil {
		return "", fmt.Errorf("couldn't reach the model provider: %v", err)
	}
	if status >= 300 {
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
		return "", fmt.Errorf("provider returned %d: %s", status, msg)
	}
	var text string
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
		text = strings.Join(out, "\n")
	} else {
		var r struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(b, &r); err != nil || len(r.Choices) == 0 {
			return "", errors.New("unexpected reply from provider")
		}
		text = r.Choices[0].Message.Content
		if strings.TrimSpace(text) == "" && r.Choices[0].FinishReason == "length" {
			return "", errors.New("the model used its whole token budget thinking and returned no text; try a non-reasoning model")
		}
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("the model returned an empty reply")
	}
	return text, nil
}

func sendAI(s AISettings, base, system, user string, maxTokens int, tokenParam string) ([]byte, int, error) {
	var req *http.Request
	if s.Provider == "anthropic" {
		body, _ := json.Marshal(map[string]any{
			"model": s.Model, "max_tokens": maxTokens, "system": system,
			"messages": []map[string]string{{"role": "user", "content": user}},
		})
		req, _ = http.NewRequest("POST", base+"/v1/messages", bytes.NewReader(body))
		req.Header.Set("x-api-key", s.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		body, _ := json.Marshal(map[string]any{
			"model": s.Model, tokenParam: maxTokens,
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
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return b, resp.StatusCode, nil
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
