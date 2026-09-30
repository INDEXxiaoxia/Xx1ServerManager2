// Package aiclient 是 OpenAI 兼容协议的最小客户端（/chat/completions，非流式）。
//
// 之所以只用标准库自己写：本项目的 AI 调用只需要"发几条 message、拿一段文本回来"，
// 引入第三方 SDK 不划算，而且各家兼容实现的差异点很少（地址拼接、错误体形状）。
package aiclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config AI 连接配置
type Config struct {
	BaseURL string // 例：https://api.deepseek.com/v1（填到 /v1 为止）
	APIKey  string
	Model   string
	Timeout time.Duration
}

// Message 一条对话消息
type Message struct {
	Role    string `json:"role"` // system | user | assistant
	Content string `json:"content"`
}

// Reply 一次模型回复
type Reply struct {
	Text             string
	Model            string
	Truncated        bool // finish_reason == length，回复被 max_tokens 截断
	PromptTokens     int
	CompletionTokens int
	FromReasoning    bool // 正文为空、退回用了 reasoning_content
}

// APIError 带 HTTP 状态码与响应体的错误。
// 必须把状态码和响应体带回前端：否则用户分不清是 Key 错、地址错还是模型名错。
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 500 {
		body = body[:500] + "…"
	}
	hint := ""
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		hint = "（API Key 无效或无权限）"
	case http.StatusNotFound:
		hint = "（地址或模型名不对：检查接口地址是否漏了 /v1、模型名是否拼错）"
	case http.StatusTooManyRequests:
		hint = "（触发限流，稍后重试）"
	}
	if body == "" {
		body = http.StatusText(e.Status)
	}
	return fmt.Sprintf("HTTP %d%s：%s", e.Status, hint, body)
}

// Client AI 客户端
type Client struct {
	cfg  Config
	http *http.Client
}

// New 创建客户端
func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
	}
}

// Endpoint 把用户填的 base URL 拼成 /chat/completions 的完整地址。
//
// 容忍这几种常见填法（设置页里已经提示"填到 /v1 为止"）：
//
//	https://api.deepseek.com/v1      → https://api.deepseek.com/v1/chat/completions
//	https://api.deepseek.com/v1/     → 同上（去掉多余斜杠）
//	https://api.deepseek.com         → https://api.deepseek.com/chat/completions（不擅自补 /v1）
//	.../v1/chat/completions          → 原样使用（有些用户会把整条路径粘进来）
//	localhost:11434/v1               → http://localhost:11434/v1/chat/completions（补协议）
func Endpoint(baseURL string) string {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return ""
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Chat 发一轮对话并取回文本（非流式）
func (c *Client) Chat(msgs []Message) (Reply, error) {
	return c.chat(msgs, 0)
}

// chat maxTokens 为 0 时不限制
func (c *Client) chat(msgs []Message, maxTokens int) (Reply, error) {
	var out Reply
	endpoint := Endpoint(c.cfg.BaseURL)
	if endpoint == "" {
		return out, errors.New("尚未配置 AI 接口地址（设置 → AI 设置）")
	}
	if strings.TrimSpace(c.cfg.Model) == "" {
		return out, errors.New("尚未配置模型名（设置 → AI 设置）")
	}
	body, err := json.Marshal(chatRequest{
		Model:       strings.TrimSpace(c.cfg.Model),
		Messages:    msgs,
		Temperature: 0.2, // 运维场景要稳，不要创意
		MaxTokens:   maxTokens,
		Stream:      false,
	})
	if err != nil {
		return out, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return out, fmt.Errorf("接口地址无效：%v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.APIKey))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, http.ErrHandlerTimeout) || strings.Contains(err.Error(), "context deadline exceeded") {
			return out, errors.New("请求超时（可在设置里调大 AI 超时时间）")
		}
		return out, fmt.Errorf("请求失败：%v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		return out, &APIError{Status: resp.StatusCode, Body: string(raw)}
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return out, fmt.Errorf("返回内容不是合法 JSON（HTTP %d）：%s", resp.StatusCode, snippet(raw))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return out, fmt.Errorf("接口返回错误：%s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return out, fmt.Errorf("接口没有返回任何回复（choices 为空）：%s", snippet(raw))
	}
	ch := parsed.Choices[0]
	out.Model = parsed.Model
	out.Truncated = ch.FinishReason == "length"
	out.PromptTokens = parsed.Usage.PromptTokens
	out.CompletionTokens = parsed.Usage.CompletionTokens
	out.Text = strings.TrimSpace(ch.Message.Content)
	if out.Text == "" {
		// deepseek-reasoner 这类模型正文可能为空、只有思维链：退一步用它，
		// 总比对着"接口返回了空内容"发呆强
		if r := strings.TrimSpace(ch.Message.ReasoningContent); r != "" {
			out.Text = r
			out.FromReasoning = true
		} else {
			return out, errors.New("接口返回了空回复（content 为空）")
		}
	}
	return out, nil
}

// Ping 测试连通：用一条最短的请求验证地址 / Key / 模型名三件事。
// 返回耗时（毫秒），错误里带 HTTP 状态码与响应体。
func (c *Client) Ping() (int64, error) {
	start := time.Now()
	_, err := c.chat([]Message{{Role: "user", Content: "ping"}}, 1)
	return time.Since(start).Milliseconds(), err
}

// snippet 截断响应体用于报错信息
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
