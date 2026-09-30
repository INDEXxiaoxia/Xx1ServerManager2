package aiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.deepseek.com/v1":                       "https://api.deepseek.com/v1/chat/completions",
		"https://api.deepseek.com/v1/":                      "https://api.deepseek.com/v1/chat/completions",
		"https://api.deepseek.com":                          "https://api.deepseek.com/chat/completions",
		"  https://api.deepseek.com/v1  ":                   "https://api.deepseek.com/v1/chat/completions",
		"https://x.com/v1/chat/completions":                 "https://x.com/v1/chat/completions",
		"localhost:11434/v1":                                "http://localhost:11434/v1/chat/completions",
		"http://127.0.0.1:8000/v1/":                         "http://127.0.0.1:8000/v1/chat/completions",
		"https://dashscope.aliyuncs.com/compatible-mode/v1": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		"": "",
	}
	for in, want := range cases {
		if got := Endpoint(in); got != want {
			t.Errorf("Endpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := New(Config{BaseURL: srv.URL + "/v1", APIKey: "sk-test", Model: "deepseek-chat", Timeout: 5 * time.Second})
	return c, srv
}

func TestChatSuccess(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody chatRequest
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"model":"deepseek-chat","choices":[{"message":{"content":"  hello  "},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	})
	defer srv.Close()

	rep, err := c.Chat([]Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if rep.Text != "hello" {
		t.Errorf("回复未去空白: %q", rep.Text)
	}
	if rep.Truncated || rep.Model != "deepseek-chat" || rep.PromptTokens != 3 {
		t.Errorf("回复字段错误: %+v", rep)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("请求路径 = %q", gotPath)
	}
	if gotBody.Model != "deepseek-chat" || len(gotBody.Messages) != 1 || gotBody.Stream {
		t.Errorf("请求体错误: %+v", gotBody)
	}
}

func TestChatErrors(t *testing.T) {
	// 401：必须带状态码与响应体，用户才知道是 Key 的问题
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Authentication Fails, Your api key is invalid"}}`))
	})
	defer srv.Close()
	_, err := c.Chat(nil)
	if err == nil {
		t.Fatalf("401 应当报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "401") || !strings.Contains(msg, "Authentication Fails") || !strings.Contains(msg, "API Key") {
		t.Errorf("401 错误信息不够明确: %q", msg)
	}

	// 404：要提示可能是地址/模型名的问题
	c2, srv2 := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
	})
	defer srv2.Close()
	if _, err := c2.Chat(nil); err == nil || !strings.Contains(err.Error(), "/v1") {
		t.Errorf("404 应提示地址可能漏了 /v1: %v", err)
	}

	// 非 JSON 响应
	c3, srv3 := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>bad gateway</html>`))
	})
	defer srv3.Close()
	if _, err := c3.Chat(nil); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Errorf("非 JSON 应给出可读错误: %v", err)
	}

	// choices 为空
	c4, srv4 := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	})
	defer srv4.Close()
	if _, err := c4.Chat(nil); err == nil || !strings.Contains(err.Error(), "choices") {
		t.Errorf("空 choices 应给出可读错误: %v", err)
	}
}

func TestChatReasoningFallback(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","reasoning_content":"我先想想"},"finish_reason":"length"}]}`))
	})
	defer srv.Close()
	rep, err := c.Chat(nil)
	if err != nil {
		t.Fatalf("应回退到 reasoning_content: %v", err)
	}
	if rep.Text != "我先想想" || !rep.FromReasoning || !rep.Truncated {
		t.Errorf("回退结果错误: %+v", rep)
	}
}

func TestChatEmptyReply(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"   "}}]}`))
	})
	defer srv.Close()
	if _, err := c.Chat(nil); err == nil || !strings.Contains(err.Error(), "空回复") {
		t.Errorf("空回复应报错: %v", err)
	}
}

func TestChatMissingConfig(t *testing.T) {
	if _, err := New(Config{Model: "m"}).Chat(nil); err == nil || !strings.Contains(err.Error(), "接口地址") {
		t.Errorf("缺地址应提示去设置: %v", err)
	}
	if _, err := New(Config{BaseURL: "http://x/v1"}).Chat(nil); err == nil || !strings.Contains(err.Error(), "模型名") {
		t.Errorf("缺模型名应提示去设置: %v", err)
	}
}

func TestPingTimeout(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	defer srv.Close()
	c.http.Timeout = 200 * time.Millisecond
	if _, err := c.Ping(); err == nil {
		t.Fatalf("超时应报错")
	}
}

func TestPingLimitTokens(t *testing.T) {
	var got chatRequest
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	})
	defer srv.Close()
	ms, err := c.Ping()
	if err != nil {
		t.Fatalf("Ping 失败: %v", err)
	}
	if ms < 0 {
		t.Errorf("耗时不应为负: %d", ms)
	}
	if got.MaxTokens != 1 {
		t.Errorf("测试连通应限制 max_tokens=1，实际 %d", got.MaxTokens)
	}
}
