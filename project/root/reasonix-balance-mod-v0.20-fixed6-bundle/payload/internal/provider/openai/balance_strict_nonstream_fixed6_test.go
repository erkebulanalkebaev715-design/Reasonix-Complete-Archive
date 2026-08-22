package openai

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "sync/atomic"
    "testing"
    "time"

    "reasonix/internal/provider"
)

func TestBalanceStrictSingleAttemptUsesNonStreamingDeepSeek(t *testing.T) {
    var calls atomic.Int32
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        calls.Add(1)
        var body map[string]any
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil { t.Fatal(err) }
        if got, _ := body["stream"].(bool); got { t.Fatalf("stream=%v, want false in strict mode", got) }
        thinking, _ := body["thinking"].(map[string]any)
        if thinking["type"] != "disabled" { t.Fatalf("thinking=%v, want disabled", thinking) }
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"BALANCE_OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":10}}`))
    }))
    defer srv.Close()

    p, err := New(provider.Config{Name:"strict-test", BaseURL:srv.URL, Model:"deepseek-v4-flash", APIKey:"x", Extra:map[string]any{"reasoning_protocol":"deepseek","effort":"disabled"}})
    if err != nil { t.Fatal(err) }
    ctx := provider.WithRequestRetryLimit(context.Background(), 0)
    ch, err := p.Stream(ctx, provider.Request{Messages:[]provider.Message{{Role:provider.RoleUser, Content:"x"}}, MaxTokens:8})
    if err != nil { t.Fatal(err) }
    var text string; var usage *provider.Usage
    for c := range ch { if c.Type==provider.ChunkText { text += c.Text }; if c.Type==provider.ChunkUsage { usage=c.Usage }; if c.Type==provider.ChunkError { t.Fatal(c.Err) } }
    if text != "BALANCE_OK" { t.Fatalf("text=%q", text) }
    if usage == nil || usage.PromptTokens != 10 || usage.CompletionTokens != 2 { t.Fatalf("usage=%+v", usage) }
    if calls.Load()!=1 { t.Fatalf("calls=%d, want 1", calls.Load()) }
}

func TestBalanceStrictSingleAttemptHasHardProviderTimeout(t *testing.T) {
    old := strictSingleAttemptTimeout
    strictSingleAttemptTimeout = 80 * time.Millisecond
    defer func(){ strictSingleAttemptTimeout = old }()
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(300*time.Millisecond); w.WriteHeader(http.StatusOK) }))
    defer srv.Close()
    p, err := New(provider.Config{Name:"timeout-test", BaseURL:srv.URL, Model:"deepseek-v4-flash", APIKey:"x", Extra:map[string]any{"reasoning_protocol":"deepseek","effort":"disabled"}})
    if err != nil { t.Fatal(err) }
    start:=time.Now()
    _, err = p.Stream(provider.WithRequestRetryLimit(context.Background(),0), provider.Request{Messages:[]provider.Message{{Role:provider.RoleUser,Content:"x"}},MaxTokens:8})
    if err == nil { t.Fatal("expected timeout") }
    if time.Since(start) > 250*time.Millisecond { t.Fatalf("strict provider timeout was not enforced: %v", time.Since(start)) }
}
