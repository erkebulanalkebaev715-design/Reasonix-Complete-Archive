package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/provider"
)

func TestBalanceV20UsageReceiptFixed11(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt.json")
	t.Setenv("BALANCE_V20_USAGE_RECEIPT_PATH", path)
	usage := &provider.Usage{PromptTokens: 101, CompletionTokens: 7, TotalTokens: 108, CacheHitTokens: 40, CacheMissTokens: 61, ReasoningTokens: 2, RequestCount: 0, FinishReason: "stop"}
	writeBalanceV20UsageReceipt("deepseek-v20/deepseek-v4-flash", usage)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got balanceV20UsageReceipt
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != balanceV20UsageReceiptSchema || got.ModelRef != "deepseek-v20/deepseek-v4-flash" {
		t.Fatalf("identity=%+v", got)
	}
	if got.PromptTokens != 101 || got.CompletionTokens != 7 || got.TotalTokens != 108 || got.CacheHitTokens != 40 || got.CacheMissTokens != 61 || got.ReasoningTokens != 2 || got.RequestCount != 1 || got.Estimated || got.FinishReason != "stop" {
		t.Fatalf("usage=%+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("receipt mode=%o", info.Mode().Perm())
	}
}

func TestBalanceV20UsageReceiptDisabledWithoutEnvFixed11(t *testing.T) {
	t.Setenv("BALANCE_V20_USAGE_RECEIPT_PATH", "")
	writeBalanceV20UsageReceipt("deepseek-v20/deepseek-v4-flash", &provider.Usage{PromptTokens: 1})
}
