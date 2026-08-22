package swarm

import (
	"os"
	"path/filepath"
	"testing"

	_ "reasonix/internal/provider/mock" // registers the offline "mock" provider kind

	"reasonix/internal/config"
)

// writeIsolatedConfig installs a minimal REASONIX_HOME config with a mock
// provider and an openai provider so Resolver tests never touch a real key or
// network.
func writeIsolatedConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	cfg := `config_version = 6
default_model = "balance-mock"

[[providers]]
name = "balance-mock"
kind = "mock"
model = "smoke"
base_url = "http://127.0.0.1"
context_window = 1000000
price = { cache_hit = 0, input = 1, output = 2, currency = "CNY" }

[[providers]]
name = "kimi"
kind = "openai"
base_url = "https://api.moonshot.cn/v1"
model = "moonshot-v1-8k"
api_key_env = "KIMI_API_KEY"
context_window = 131072
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestConfigResolverResolvesConfiguredProviders(t *testing.T) {
	writeIsolatedConfig(t)
	r := DefaultConfigResolver()
	if r.Load == nil {
		t.Fatal("DefaultConfigResolver must wire the real config loader")
	}
	prov, price, modelRef, providerName, err := r.Resolve("balance-mock")
	if err != nil {
		t.Fatalf("resolve mock provider: %v", err)
	}
	if prov == nil || prov.Name() != "balance-mock" {
		t.Fatalf("provider = %+v, want balance-mock", prov)
	}
	if providerName != "balance-mock" {
		t.Fatalf("provider name = %q", providerName)
	}
	if modelRef == "" {
		t.Fatal("empty model ref")
	}
	if price == nil || price.Currency != "CNY" {
		t.Fatalf("pricing = %+v, want CNY rate card", price)
	}
}

func TestConfigResolverResolvesAnyConfiguredKind(t *testing.T) {
	writeIsolatedConfig(t)
	r := DefaultConfigResolver()
	prov, _, _, providerName, err := r.Resolve("kimi")
	if err != nil {
		t.Fatalf("resolve kimi provider: %v", err)
	}
	if prov == nil || providerName != "kimi" {
		t.Fatalf("provider = %+v name = %q, want kimi", prov, providerName)
	}
}

func TestConfigResolverUnknownModelFailsClosed(t *testing.T) {
	writeIsolatedConfig(t)
	r := DefaultConfigResolver()
	_, _, _, _, err := r.Resolve("does-not-exist")
	if err == nil {
		t.Fatal("unknown model ref must fail closed")
	}
}

func TestConfigResolverUsesConfiguredDefaultWhenRefEmpty(t *testing.T) {
	writeIsolatedConfig(t)
	r := DefaultConfigResolver()
	prov, _, _, providerName, err := r.Resolve("")
	if err != nil {
		t.Fatalf("resolve default model: %v", err)
	}
	if prov == nil || providerName != "balance-mock" {
		t.Fatalf("default provider = %+v name = %q, want balance-mock", prov, providerName)
	}
}

func TestConfigResolverHonorsStateDirHelpers(t *testing.T) {
	home := writeIsolatedConfig(t)
	if got := config.SwarmStateDir(); filepath.Dir(got) != filepath.Join(home, "state") {
		t.Fatalf("SwarmStateDir() = %q, want under %s/state", got, home)
	}
}
