package swarm

import (
	"fmt"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/provider"
)

// Resolver resolves a model ref to a live provider instance plus its rate
// card. The swarm is provider-agnostic: any configured Reasonix provider
// (DeepSeek, Kimi/Moonshot, ...) resolves through this seam.
type Resolver interface {
	// Resolve returns the provider instance, its pricing (may be nil), the
	// resolved canonical model ref, and the provider name.
	Resolve(modelRef string) (provider.Provider, *provider.Pricing, string, string, error)
}

// ConfigResolver resolves through the real Reasonix config + provider factory.
type ConfigResolver struct {
	// NewProvider mirrors boot.NewProvider's construction without importing a
	// frontend. Tests may override it.
	NewProvider func(*config.ProviderEntry) (provider.Provider, error)
	Load        func() (*config.Config, error)
}

// DefaultConfigResolver uses the real config loader and provider registry.
func DefaultConfigResolver() *ConfigResolver {
	return &ConfigResolver{
		NewProvider: newProviderFromEntry,
		Load:        config.Load,
	}
}

func (r *ConfigResolver) Resolve(modelRef string) (provider.Provider, *provider.Pricing, string, string, error) {
	if r == nil {
		return nil, nil, "", "", fmt.Errorf("swarm: nil config resolver")
	}
	cfg, err := r.Load()
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("swarm: load config: %w", err)
	}
	ref := strings.TrimSpace(modelRef)
	if ref == "" {
		ref = strings.TrimSpace(cfg.DefaultModel)
	}
	if ref == "" {
		return nil, nil, "", "", fmt.Errorf("swarm: no model ref and no default model configured")
	}
	entry, ok := cfg.ResolveModel(ref)
	if !ok || entry == nil {
		return nil, nil, "", "", fmt.Errorf("swarm: model %q is not a configured provider", ref)
	}
	prov, err := r.NewProvider(entry)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("swarm: provider %q: %w", entry.Name, err)
	}
	price := entry.PriceForModel(entry.Model)
	return prov, price, canonicalModelRef(entry), entry.Name, nil
}

// canonicalModelRef renders the "provider/model" display ref used on Usage
// events, mirroring boot's modelID construction.
func canonicalModelRef(e *config.ProviderEntry) string {
	modelID := strings.TrimSpace(e.Name)
	model := strings.TrimSpace(e.Model)
	if modelID != "" && model != "" {
		return modelID + "/" + model
	}
	if model != "" {
		return model
	}
	return modelID
}

func newProviderFromEntry(e *config.ProviderEntry) (provider.Provider, error) {
	return provider.New(e.Kind, provider.Config{
		Name:    e.Name,
		BaseURL: e.BaseURL,
		Model:   e.Model,
		APIKey:  e.APIKey(),
		Extra: map[string]any{
			"api_key_env":        e.APIKeyEnv,
			"api_key_source":     e.APIKeySourceLabel(),
			"thinking":           e.Thinking,
			"effort":             config.EffectiveEffort(e),
			"supported_efforts":  e.SupportedEfforts,
			"reasoning_protocol": config.ReasoningProtocolForEntry(e),
			"max_output_tokens":  e.MaxOutputTokens,
			"chat_url":           e.ChatURL,
			"request_url":        e.RequestURL,
			"headers":            e.Headers,
			"extra_body":         e.ExtraBody,
			"auth_header":        e.AuthHeader,
			"proxy_spec":         nil,
			"vision":             config.EffectiveVision(e),
			"vision_detail":      e.VisionDetail,
			"web_search":         config.EffectiveWebSearch(e),
			"mode":               e.ResponsesMode,
			"stateful":           e.ResponsesStateful,
		},
	})
}
