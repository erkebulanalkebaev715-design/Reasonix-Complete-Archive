package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/provider"
)

const balanceV20UsageReceiptSchema = "balance-provider-usage-receipt-v1"

type balanceV20UsageReceipt struct {
	Schema           string `json:"schema"`
	ModelRef         string `json:"modelRef"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	TotalTokens      int    `json:"totalTokens"`
	CacheHitTokens   int    `json:"cacheHitTokens"`
	CacheMissTokens  int    `json:"cacheMissTokens"`
	ReasoningTokens  int    `json:"reasoningTokens"`
	RequestCount     int    `json:"requestCount"`
	Estimated        bool   `json:"estimated"`
	FinishReason     string `json:"finishReason,omitempty"`
}

// writeBalanceV20UsageReceipt is a no-op unless the bounded v0.20 real gate
// explicitly provides a receipt path. It copies the same Usage record that the
// agent emits to the normal event sink after provider completion.
func writeBalanceV20UsageReceipt(modelRef string, usage *provider.Usage) {
	path := strings.TrimSpace(os.Getenv("BALANCE_V20_USAGE_RECEIPT_PATH"))
	if path == "" || usage == nil {
		return
	}
	requestCount := usage.RequestCount
	if requestCount <= 0 {
		// provider.Usage keeps zero as the backward-compatible representation
		// of one request. Persist the semantic count so the gate can require 1.
		requestCount = 1
	}
	receipt := balanceV20UsageReceipt{
		Schema: balanceV20UsageReceiptSchema, ModelRef: strings.TrimSpace(modelRef),
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		TotalTokens: usage.TotalTokens, CacheHitTokens: usage.CacheHitTokens,
		CacheMissTokens: usage.CacheMissTokens, ReasoningTokens: usage.ReasoningTokens,
		RequestCount: requestCount, Estimated: usage.Estimated, FinishReason: usage.FinishReason,
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".balance-v20-usage-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	_ = tmp.Chmod(0o600)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return
	}
	_ = os.Rename(tmpName, path)
}
