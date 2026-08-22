package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBalanceStrictRetryLimitZeroStartsOneHTTPAttempt(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("retryable"))
	}))
	defer srv.Close()

	ctx := WithRequestRetryLimit(context.Background(), 0)
	_, err := SendWithRetry(ctx, srv.Client(), SendOptions{Provider: "balance-test"}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, nil)
	})
	if err == nil {
		t.Fatal("expected final 500 error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP attempts=%d, want exactly 1 in strict hard-budget mode", got)
	}
}
