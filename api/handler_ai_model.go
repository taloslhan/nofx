package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nofx/config"
	"nofx/crypto"
	"nofx/logger"
	"nofx/mcp"
	"nofx/security"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ModelConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	Enabled      bool   `json:"enabled"`
	APIKey       string `json:"apiKey,omitempty"`
	CustomAPIURL string `json:"customApiUrl,omitempty"`
}

// SafeModelConfig Safe model configuration structure (does not contain sensitive information)
type SafeModelConfig struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Enabled         bool   `json:"enabled"`
	CustomAPIURL    string `json:"customApiUrl"`    // Custom API URL (usually not sensitive)
	CustomModelName string `json:"customModelName"` // Custom model name (not sensitive)
}

type UpdateModelConfigRequest struct {
	Models map[string]struct {
		Enabled         bool   `json:"enabled"`
		APIKey          string `json:"api_key"`
		CustomAPIURL    string `json:"custom_api_url"`
		CustomModelName string `json:"custom_model_name"`
	} `json:"models"`
}

type TestModelRequest struct {
	Provider        string `json:"provider"`
	APIKey          string `json:"api_key"`
	CustomAPIURL    string `json:"custom_api_url"`
	CustomModelName string `json:"custom_model_name"`
}

type TestModelResponse struct {
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latency_ms"`
	Model     string `json:"model,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) decodeModelRequest(c *gin.Context, userID string, target interface{}) bool {
	cfg := config.Get()

	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return false
	}

	if !cfg.TransportEncryption {
		if err := json.Unmarshal(bodyBytes, target); err != nil {
			logger.Infof("❌ Failed to parse plain model request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return false
		}
		logger.Infof("📝 Received plain text model request (UserID: %s)", userID)
		return true
	}

	var encryptedPayload crypto.EncryptedPayload
	if err := json.Unmarshal(bodyBytes, &encryptedPayload); err != nil {
		logger.Infof("❌ Failed to parse encrypted model payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format, encrypted transmission required"})
		return false
	}

	if encryptedPayload.WrappedKey == "" {
		logger.Infof("❌ Detected unencrypted model request (UserID: %s)", userID)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "This endpoint only supports encrypted transmission, please use encrypted client",
			"code":    "ENCRYPTION_REQUIRED",
			"message": "Encrypted transmission is required for security reasons",
		})
		return false
	}

	decrypted, err := s.cryptoHandler.cryptoService.DecryptSensitiveData(&encryptedPayload)
	if err != nil {
		logger.Infof("❌ Failed to decrypt model request (UserID: %s): %v", userID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decrypt data"})
		return false
	}

	if err := json.Unmarshal([]byte(decrypted), target); err != nil {
		logger.Infof("❌ Failed to parse decrypted model request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse decrypted data"})
		return false
	}

	logger.Infof("🔓 Decrypted model request data (UserID: %s)", userID)
	return true
}

// handleGetModelConfigs Get AI model configurations
func (s *Server) handleGetModelConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	logger.Infof("🔍 Querying AI model configs for user %s", userID)
	models, err := s.store.AIModel().List(userID)
	if err != nil {
		logger.Infof("❌ Failed to get AI model configs: %v", err)
		SafeInternalError(c, "Failed to get AI model configs", err)
		return
	}

	// If no models in database, return default models
	if len(models) == 0 {
		logger.Infof("⚠️ No AI models in database, returning defaults")
		defaultModels := []SafeModelConfig{
			{ID: "deepseek", Name: "DeepSeek AI", Provider: "deepseek", Enabled: false},
			{ID: "qwen", Name: "Qwen AI", Provider: "qwen", Enabled: false},
			{ID: "openai", Name: "OpenAI", Provider: "openai", Enabled: false},
			{ID: "claude", Name: "Claude AI", Provider: "claude", Enabled: false},
			{ID: "gemini", Name: "Gemini AI", Provider: "gemini", Enabled: false},
			{ID: "grok", Name: "Grok AI", Provider: "grok", Enabled: false},
			{ID: "kimi", Name: "Kimi AI", Provider: "kimi", Enabled: false},
			{ID: "minimax", Name: "MiniMax AI", Provider: "minimax", Enabled: false},
		}
		c.JSON(http.StatusOK, defaultModels)
		return
	}

	logger.Infof("✅ Found %d AI model configs", len(models))

	// Convert to safe response structure, remove sensitive information
	safeModels := make([]SafeModelConfig, len(models))
	for i, model := range models {
		safeModels[i] = SafeModelConfig{
			ID:              model.ID,
			Name:            model.Name,
			Provider:        model.Provider,
			Enabled:         model.Enabled,
			CustomAPIURL:    model.CustomAPIURL,
			CustomModelName: model.CustomModelName,
		}
	}

	c.JSON(http.StatusOK, safeModels)
}

// handleUpdateModelConfigs Update AI model configurations (supports both encrypted and plain text based on config)
func (s *Server) handleUpdateModelConfigs(c *gin.Context) {
	userID := c.GetString("user_id")

	var req UpdateModelConfigRequest
	if !s.decodeModelRequest(c, userID, &req) {
		return
	}

	// Update each model's configuration and track traders that need reload
	tradersToReload := make(map[string]bool)
	for modelID, modelData := range req.Models {
		// SSRF protection: validate custom_api_url before storing
		if modelData.CustomAPIURL != "" {
			cleanURL := strings.TrimSuffix(modelData.CustomAPIURL, "#")
			if err := security.ValidateURL(cleanURL); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid custom_api_url for model %s: %s", modelID, err.Error())})
				return
			}
		}

		// Find traders using this AI model BEFORE updating
		traders, _ := s.store.Trader().ListByAIModelID(userID, modelID)
		for _, t := range traders {
			tradersToReload[t.ID] = true
		}

		err := s.store.AIModel().Update(userID, modelID, modelData.Enabled, modelData.APIKey, modelData.CustomAPIURL, modelData.CustomModelName)
		if err != nil {
			SafeInternalError(c, fmt.Sprintf("Update model %s", modelID), err)
			return
		}
	}

	// Remove affected traders from memory BEFORE reloading to pick up new config
	for traderID := range tradersToReload {
		logger.Infof("🔄 Removing trader %s from memory to reload with new AI model config", traderID)
		s.traderManager.RemoveTrader(traderID)
	}

	// Reload all traders for this user to make new config take effect immediately
	err := s.traderManager.LoadUserTradersFromStore(s.store, userID)
	if err != nil {
		logger.Infof("⚠️ Failed to reload user traders into memory: %v", err)
		// Don't return error here since model config was successfully updated to database
	}

	logger.Infof("✓ AI model config updated: %+v", req.Models)
	c.JSON(http.StatusOK, gin.H{"message": "Model configuration updated"})
}

func validateModelConnectionInput(provider, apiKey, customAPIURL string) error {
	if provider == "" {
		return fmt.Errorf("provider is required")
	}
	if apiKey == "" {
		return fmt.Errorf("api_key is required")
	}
	if customAPIURL != "" {
		cleanURL := strings.TrimSuffix(customAPIURL, "#")
		if err := security.ValidateURL(cleanURL); err != nil {
			return fmt.Errorf("Invalid custom_api_url: %s", err.Error())
		}
	}
	return nil
}

func runModelConnectionTest(userID, provider, apiKey, customAPIURL, customModelName string) TestModelResponse {
	client := mcp.NewAIClientByProvider(
		provider,
		mcp.WithTimeout(30*time.Second),
		mcp.WithMaxRetries(1),
		mcp.WithMaxTokens(50),
	)

	resp := TestModelResponse{}
	if client == nil {
		resp.Error = fmt.Sprintf("unsupported provider: %s", provider)
		return resp
	}

	client.SetAPIKey(apiKey, customAPIURL, customModelName)

	if embedder, ok := client.(mcp.ClientEmbedder); ok {
		resp.Model = embedder.BaseClient().Model
	}

	start := time.Now()
	result, err := client.CallWithMessages("Reply with exactly one word: OK", "test")
	resp.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		resp.Error = err.Error()
		logger.Infof("❌ Model connection test failed (user=%s provider=%s model=%s): %v", userID, provider, resp.Model, err)
		return resp
	}

	resp.Success = true
	resp.Message = strings.TrimSpace(result)
	if resp.Message == "" {
		resp.Message = "OK"
	}

	logger.Infof("✅ Model connection test passed (user=%s provider=%s model=%s latency=%dms)", userID, provider, resp.Model, resp.LatencyMs)
	return resp
}

func (s *Server) handleTestModelConnection(c *gin.Context) {
	userID := c.GetString("user_id")

	var req TestModelRequest
	if !s.decodeModelRequest(c, userID, &req) {
		return
	}

	req.Provider = strings.TrimSpace(req.Provider)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.CustomAPIURL = strings.TrimSpace(req.CustomAPIURL)
	req.CustomModelName = strings.TrimSpace(req.CustomModelName)

	if err := validateModelConnectionInput(req.Provider, req.APIKey, req.CustomAPIURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp := runModelConnectionTest(
		userID,
		req.Provider,
		req.APIKey,
		req.CustomAPIURL,
		req.CustomModelName,
	)
	if resp.Error != "" && strings.HasPrefix(resp.Error, "unsupported provider:") {
		c.JSON(http.StatusBadRequest, gin.H{"error": resp.Error})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleTestSavedModelConnection(c *gin.Context) {
	userID := c.GetString("user_id")
	modelID := strings.TrimSpace(c.Param("id"))
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model id is required"})
		return
	}

	model, err := s.store.AIModel().Get(userID, modelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Model not found"})
			return
		}
		SafeInternalError(c, "Get AI model config", err)
		return
	}

	provider := strings.TrimSpace(model.Provider)
	apiKey := strings.TrimSpace(model.APIKey.String())
	customAPIURL := strings.TrimSpace(model.CustomAPIURL)
	customModelName := strings.TrimSpace(model.CustomModelName)

	if err := validateModelConnectionInput(provider, apiKey, customAPIURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp := runModelConnectionTest(
		userID,
		provider,
		apiKey,
		customAPIURL,
		customModelName,
	)
	if resp.Error != "" && strings.HasPrefix(resp.Error, "unsupported provider:") {
		c.JSON(http.StatusBadRequest, gin.H{"error": resp.Error})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// handleGetSupportedModels Get list of AI models supported by the system
func (s *Server) handleGetSupportedModels(c *gin.Context) {
	// Return static list of supported AI models with default versions
	supportedModels := []map[string]interface{}{
		{"id": "deepseek", "name": "DeepSeek", "provider": "deepseek", "defaultModel": "deepseek-chat"},
		{"id": "qwen", "name": "Qwen", "provider": "qwen", "defaultModel": "qwen3-max"},
		{"id": "openai", "name": "OpenAI", "provider": "openai", "defaultModel": "gpt-5.1"},
		{"id": "claude", "name": "Claude", "provider": "claude", "defaultModel": "claude-opus-4-6"},
		{"id": "gemini", "name": "Google Gemini", "provider": "gemini", "defaultModel": "gemini-3-pro-preview"},
		{"id": "grok", "name": "Grok (xAI)", "provider": "grok", "defaultModel": "grok-3-latest"},
		{"id": "kimi", "name": "Kimi (Moonshot)", "provider": "kimi", "defaultModel": "moonshot-v1-auto"},
		{"id": "minimax", "name": "MiniMax", "provider": "minimax", "defaultModel": "MiniMax-M2.7"},
		{"id": "claw402", "name": "Claw402 (Base USDC)", "provider": "claw402", "defaultModel": "deepseek"},
	}

	c.JSON(http.StatusOK, supportedModels)
}
