package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProviderProtocols(t *testing.T) {
	for _, protocol := range []string{OpenAI, Anthropic} {
		t.Run(protocol, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if protocol == Anthropic {
					if r.Header.Get("x-api-key") != "test-secret" || r.Header.Get("anthropic-version") == "" {
						t.Error("missing Anthropic authentication")
					}
				} else if r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Error("missing bearer authentication")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/models" {
					fmt.Fprint(w, `{"data":[{"id":"embedding-only"},{"id":"gpt-5.6"},{"id":"gpt-5.6"}]}`)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "gpt-5.6" || body["stream"] != false {
					t.Errorf("unexpected body: %v", body)
				}
				messages := body["messages"].([]any)
				if protocol == Anthropic {
					if r.URL.Path != "/v1/messages" || len(messages) != 1 || body["system"] != "system" || body["max_tokens"] == nil {
						t.Errorf("invalid Anthropic request: %v", body)
					}
					fmt.Fprint(w, `{"content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"分析成功"}]}`)
				} else {
					if r.URL.Path != "/v1/chat/completions" || len(messages) != 2 {
						t.Error("invalid chat completion request")
					}
					fmt.Fprint(w, `{"choices":[{"message":{"content":"分析成功"}}]}`)
				}
			}))
			defer server.Close()
			p := Provider{Name: "test", Protocol: protocol, BaseURL: server.URL + "/v1/", APIKey: "test-secret", AutoModel: true}
			c := NewClient()
			model, err := c.ResolveModel(context.Background(), p)
			if err != nil || model != "gpt-5.6" {
				t.Fatalf("model=%q err=%v", model, err)
			}
			text, err := c.Complete(context.Background(), p, model, "system", "process JSON")
			if err != nil || text != "分析成功" || calls != 2 {
				t.Fatalf("text=%q err=%v calls=%d", text, err, calls)
			}
		})
	}
}

func TestFailuresDoNotExposeCredentials(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
		want   string
	}{
		{401, `{"error":{"message":"test-secret"}}`, "API 密钥"},
		{429, "test-secret", "额度"},
		{200, "<html>test-secret</html>", "JSON"},
		{200, `{"choices":[]}`, "未返回文本"},
		{200, `{"choices":[{"message":{"refusal":"no"}}]}`, "拒绝"},
	} {
		t.Run(fmt.Sprint(test.status, test.want), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status); fmt.Fprint(w, test.body) }))
			defer s.Close()
			p := Provider{Name: "test", Protocol: OpenAI, BaseURL: s.URL, APIKey: "test-secret", Model: "manual"}
			_, err := NewClient().Complete(context.Background(), p, p.Model, "system", "prompt")
			if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "test-secret") {
				t.Fatalf("unsafe/unexpected error: %v", err)
			}
		})
	}
}

func TestCancellationRedirectAndManualFallback(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(404)
			return
		}
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer s.Close()
	p := Provider{Name: "local", Protocol: OpenAI, BaseURL: s.URL, Model: "custom-model", AutoModel: true}
	c := NewClient()
	model, err := c.ResolveModel(context.Background(), p)
	if err != nil || model != p.Model {
		t.Fatalf("manual fallback failed: %s %v", model, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err = c.Complete(ctx, p, model, "system", "prompt"); err == nil {
		t.Fatal("request wasn't canceled")
	}
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	p.BaseURL = redirect.URL
	if _, err = c.Complete(context.Background(), p, model, "system", "prompt"); err == nil || targetCalls != 0 {
		t.Fatal("redirect forwarded credentials or succeeded")
	}
}

func TestValidationAndModelChoice(t *testing.T) {
	base := Provider{Name: "test", Protocol: OpenAI, BaseURL: "https://example.com/v1", APIKey: "key", Model: "gpt-5.6"}
	for _, endpoint := range []string{"http://example.com/v1", "https://key@example.com/v1", "https://example.com?key=secret", "file:///etc/passwd", "https://example.com/#secret", "missing-url"} {
		p := base
		p.BaseURL = endpoint
		if p.Validate() == nil {
			t.Errorf("accepted %s", endpoint)
		}
	}
	models := []string{"text-embedding-3-small", "gpt-5.6", "gpt-image-1", "custom-chat"}
	if got, _ := ChooseModel(models, ""); got != "gpt-5.6" {
		t.Fatal(got)
	}
	if got, _ := ChooseModel(models, "custom-chat"); got != "custom-chat" {
		t.Fatal(got)
	}
	if _, err := ChooseModel([]string{"embedding-model", "tts-model"}, ""); err == nil {
		t.Fatal("selected non-text model")
	}
	if got := endpoint(base, "/chat/completions"); got != "https://example.com/v1/chat/completions" {
		t.Fatal(got)
	}
}
