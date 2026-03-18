package provider

import (
	"testing"

	"nofx/mcp"
)

func TestOptionsWithDeepSeekClient(t *testing.T) {
	logger := mcp.NewNoopLogger()

	client := NewDeepSeekClientWithOptions(
		mcp.WithAPIKey("sk-deepseek-key"),
		mcp.WithLogger(logger),
		mcp.WithMaxTokens(5000),
	)

	dsClient := client.(*DeepSeekClient)

	// Verify DeepSeek default values
	if dsClient.Provider != mcp.ProviderDeepSeek {
		t.Error("Provider should be DeepSeek")
	}

	if dsClient.BaseURL != mcp.DefaultDeepSeekBaseURL {
		t.Error("BaseURL should be DeepSeek default")
	}

	if dsClient.Model != mcp.DefaultDeepSeekModel {
		t.Error("Model should be DeepSeek default")
	}

	// Verify custom options
	if dsClient.APIKey != "sk-deepseek-key" {
		t.Error("APIKey should be set from options")
	}

	if dsClient.Log != logger {
		t.Error("Log should be set from options")
	}

	if dsClient.MaxTokens != 5000 {
		t.Error("MaxTokens should be 5000")
	}
}

func TestOptionsWithQwenClient(t *testing.T) {
	logger := mcp.NewNoopLogger()

	client := NewQwenClientWithOptions(
		mcp.WithAPIKey("sk-qwen-key"),
		mcp.WithLogger(logger),
		mcp.WithMaxTokens(6000),
	)

	qwenClient := client.(*QwenClient)

	// Verify Qwen default values
	if qwenClient.Provider != mcp.ProviderQwen {
		t.Error("Provider should be Qwen")
	}

	if qwenClient.BaseURL != mcp.DefaultQwenBaseURL {
		t.Error("BaseURL should be Qwen default")
	}

	if qwenClient.Model != mcp.DefaultQwenModel {
		t.Error("Model should be Qwen default")
	}

	// Verify custom options
	if qwenClient.APIKey != "sk-qwen-key" {
		t.Error("APIKey should be set from options")
	}

	if qwenClient.Log != logger {
		t.Error("Log should be set from options")
	}

	if qwenClient.MaxTokens != 6000 {
		t.Error("MaxTokens should be 6000")
	}
}

func TestProviderSetAPIKeyPreservesDefaultsWithoutOverrides(t *testing.T) {
	tests := []struct {
		name         string
		newClient    func() mcp.AIClient
		wantProvider string
		wantBaseURL  string
		wantModel    string
	}{
		{
			name:         "deepseek",
			newClient:    NewDeepSeekClient,
			wantProvider: mcp.ProviderDeepSeek,
			wantBaseURL:  mcp.DefaultDeepSeekBaseURL,
			wantModel:    mcp.DefaultDeepSeekModel,
		},
		{
			name:         "qwen",
			newClient:    NewQwenClient,
			wantProvider: mcp.ProviderQwen,
			wantBaseURL:  mcp.DefaultQwenBaseURL,
			wantModel:    mcp.DefaultQwenModel,
		},
		{
			name:         "openai",
			newClient:    NewOpenAIClient,
			wantProvider: mcp.ProviderOpenAI,
			wantBaseURL:  DefaultOpenAIBaseURL,
			wantModel:    DefaultOpenAIModel,
		},
		{
			name:         "claude",
			newClient:    NewClaudeClient,
			wantProvider: mcp.ProviderClaude,
			wantBaseURL:  DefaultClaudeBaseURL,
			wantModel:    DefaultClaudeModel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.newClient()
			client.SetAPIKey("sk-test", "", "")

			base := client.(mcp.ClientEmbedder).BaseClient()
			if base.Provider != tt.wantProvider {
				t.Fatalf("expected provider %q, got %q", tt.wantProvider, base.Provider)
			}
			if base.BaseURL != tt.wantBaseURL {
				t.Fatalf("expected BaseURL %q, got %q", tt.wantBaseURL, base.BaseURL)
			}
			if base.Model != tt.wantModel {
				t.Fatalf("expected Model %q, got %q", tt.wantModel, base.Model)
			}
			if base.UseFullURL {
				t.Fatal("expected UseFullURL to remain false")
			}
		})
	}
}

func TestProviderSetAPIKeySupportsFullURLOverrides(t *testing.T) {
	tests := []struct {
		name      string
		newClient func() mcp.AIClient
		fullURL   string
		model     string
	}{
		{
			name:      "openai",
			newClient: NewOpenAIClient,
			fullURL:   "https://proxy.example.com/v1/chat/completions#",
			model:     "gpt-5.4",
		},
		{
			name:      "claude",
			newClient: NewClaudeClient,
			fullURL:   "https://proxy.example.com/v1/messages#",
			model:     "claude-opus-4-6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.newClient()
			client.SetAPIKey("sk-test", tt.fullURL, tt.model)

			base := client.(mcp.ClientEmbedder).BaseClient()
			wantURL := tt.fullURL[:len(tt.fullURL)-1]
			if base.BaseURL != wantURL {
				t.Fatalf("expected BaseURL %q, got %q", wantURL, base.BaseURL)
			}
			if !base.UseFullURL {
				t.Fatal("expected UseFullURL to be true for trailing #")
			}
			if base.Model != tt.model {
				t.Fatalf("expected Model %q, got %q", tt.model, base.Model)
			}
			if got := base.Hooks.BuildUrl(); got != wantURL {
				t.Fatalf("expected BuildUrl %q, got %q", wantURL, got)
			}
		})
	}
}
