//go:build js && wasm

// Package provider's js/wasm factory.
//
// The non-js factory.go pulls in every provider — including dmr (os/exec),
// bedrock and vertexai (cloud SDKs). None of those can be cross-compiled to
// js/wasm, so this file replaces factory.go under js/wasm with a slim variant
// that only knows about the providers that work over plain net/http (which
// the Go runtime maps to fetch in the browser):
//
//   - openai / openai_chatcompletions / openai_responses
//   - anthropic
//   - google (Gemini API; Vertex AI is unsupported under wasm)
//
// Docker Model Runner is unsupported and returns an error. Rule-based routing
// works under wasm because the rulebased provider no longer depends on bleve.
package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/environment"
	"github.com/docker/docker-agent/pkg/model/provider/anthropic"
	"github.com/docker/docker-agent/pkg/model/provider/gemini"
	"github.com/docker/docker-agent/pkg/model/provider/openai"
	"github.com/docker/docker-agent/pkg/model/provider/options"
	"github.com/docker/docker-agent/pkg/model/provider/rulebased"
)

// createRuleBasedRouter creates a rule-based routing provider.
func createRuleBasedRouter(ctx context.Context, cfg *latest.ModelConfig, models map[string]latest.ModelConfig, env environment.Provider, opts ...options.Opt) (Provider, error) {
	return rulebased.NewClient(ctx, cfg, models, env, resolveRoutedModel, opts...)
}

// resolveRoutedModel is the rulebased.ProviderFactory used by
// createRuleBasedRouter. It resolves a routing target — which is either a name
// from the models map or an inline "provider/model" spec — and returns the
// provider for it. Routing targets cannot themselves have routing rules.
func resolveRoutedModel(
	ctx context.Context,
	modelSpec string,
	models map[string]latest.ModelConfig,
	env environment.Provider,
	factoryOpts ...options.Opt,
) (rulebased.Provider, error) {
	if modelCfg, exists := models[modelSpec]; exists {
		if len(modelCfg.Routing) > 0 {
			return nil, fmt.Errorf("model %q has routing rules and cannot be used as a routing target", modelSpec)
		}
		return createDirectProvider(ctx, &modelCfg, env, factoryOpts...)
	}

	inlineCfg, parseErr := latest.ParseModelRef(modelSpec)
	if parseErr != nil {
		return nil, fmt.Errorf("invalid model spec %q: expected 'provider/model' format or a model reference", modelSpec)
	}
	return createDirectProvider(ctx, &inlineCfg, env, factoryOpts...)
}

// createDirectProvider mirrors the non-wasm version but only dispatches to
// providers that are reachable from a browser.
func createDirectProvider(ctx context.Context, cfg *latest.ModelConfig, env environment.Provider, opts ...options.Opt) (Provider, error) {
	var globalOptions options.ModelOptions
	for _, opt := range opts {
		opt(&globalOptions)
	}

	enhancedCfg := applyProviderDefaults(cfg, globalOptions.Providers())

	providerType := resolveProviderType(enhancedCfg)

	factory, ok := providerFactories[providerType]
	if !ok {
		slog.ErrorContext(ctx, "Unknown or unsupported provider type under js/wasm", "type", providerType)
		return nil, fmt.Errorf("provider type %q is not supported under js/wasm (only openai/anthropic/google work in the browser)", providerType)
	}
	return factory(ctx, enhancedCfg, env, opts...)
}

type providerFactory func(ctx context.Context, cfg *latest.ModelConfig, env environment.Provider, opts ...options.Opt) (Provider, error)

// providerFactories: js/wasm-only subset. dmr (os/exec), amazon-bedrock and
// vertex AI (cloud SDKs that don't compile to wasm) are deliberately absent.
var providerFactories = map[string]providerFactory{
	"openai":                 openaiFactory,
	"openai_chatcompletions": openaiFactory,
	"openai_responses":       openaiFactory,
	"anthropic":              anthropicFactory,
	"google":                 googleFactory,
}

func openaiFactory(ctx context.Context, cfg *latest.ModelConfig, env environment.Provider, opts ...options.Opt) (Provider, error) {
	return openai.NewClient(ctx, cfg, env, opts...)
}

func anthropicFactory(ctx context.Context, cfg *latest.ModelConfig, env environment.Provider, opts ...options.Opt) (Provider, error) {
	return anthropic.NewClient(ctx, cfg, env, opts...)
}

func googleFactory(ctx context.Context, cfg *latest.ModelConfig, env environment.Provider, opts ...options.Opt) (Provider, error) {
	return gemini.NewClient(ctx, cfg, env, opts...)
}
