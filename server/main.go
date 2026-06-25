package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	globalStats Stats
	statsMu     sync.RWMutex
	renderer    *Renderer
	dailyBudget = 10.0 // Default budget of $10.00
)

func main() {
	// Parse daily budget from environment variable
	if budgetEnv := os.Getenv("DAILY_BUDGET"); budgetEnv != "" {
		if val, err := strconv.ParseFloat(budgetEnv, 64); err == nil {
			dailyBudget = val
		}
	}

	// Initialize renderer
	var err error
	renderer, err = NewRenderer("assets/Roboto-Regular.ttf", "assets/Roboto-Bold.ttf")
	if err != nil {
		log.Fatalf("Error initializing renderer: %v", err)
	}

	// Set initial mock data so there's always something to display
	statsMu.Lock()
	globalStats = Stats{
		OpenAICost:        2.45,
		OpenAIInputToken:  124500,
		OpenAIOutputToken: 62000,
		ClaudeCost:        1.80,
		ClaudeInputToken:  45000,
		ClaudeOutputToken: 12500,
		LastUpdated:       time.Now(),
	}
	statsMu.Unlock()

	// Start background polling worker if API keys are set
	openAIKey := os.Getenv("OPENAI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if openAIKey != "" || anthropicKey != "" {
		log.Println("Starting background API poller...")
		go pollAPIs(openAIKey, os.Getenv("OPENAI_ORG_ID"), anthropicKey)
	} else {
		log.Println("No API keys provided in environment. Running in Manual/Mock mode.")
		log.Println("You can push real-time stats to /api/update via HTTP POST.")
	}

	// Define HTTP routes
	http.HandleFunc("/display.bin", handleDisplayBin)
	http.HandleFunc("/debug.png", handleDebugPng)
	http.HandleFunc("/api/stats", handleApiStats)
	http.HandleFunc("/api/update", handleApiUpdate)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// HTTP Handlers

// handleDisplayBin returns the raw, rotated 4736-byte packed buffer for the e-ink display
func handleDisplayBin(w http.ResponseWriter, r *http.Request) {
	statsMu.RLock()
	stats := globalStats
	statsMu.RUnlock()

	// Draw landscape dashboard
	imgLandscape := renderer.DrawDashboard(stats, dailyBudget)
	// Rotate to match portrait SRAM
	imgPortrait := Rotate90CW(imgLandscape)
	// Pack into 1-bit stream
	packedBytes := PackImage(imgPortrait)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(packedBytes)))
	w.WriteHeader(http.StatusOK)
	w.Write(packedBytes)
	log.Printf("Pico W fetched display.bin (%d bytes) from %s\n", len(packedBytes), r.RemoteAddr)
}

// handleDebugPng displays the unrotated landscape dashboard as a standard PNG in the browser
func handleDebugPng(w http.ResponseWriter, r *http.Request) {
	statsMu.RLock()
	stats := globalStats
	statsMu.RUnlock()

	img := renderer.DrawDashboard(stats, dailyBudget)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, "Failed to encode PNG", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// handleApiStats returns current stats as JSON
func handleApiStats(w http.ResponseWriter, r *http.Request) {
	statsMu.RLock()
	defer statsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(globalStats)
}

type UpdateRequest struct {
	OpenAICost        *float64 `json:"openai_cost"`
	OpenAIInputToken  *int64   `json:"openai_input"`
	OpenAIOutputToken *int64   `json:"openai_output"`
	ClaudeCost        *float64 `json:"claude_cost"`
	ClaudeInputToken  *int64   `json:"claude_input"`
	ClaudeOutputToken *int64   `json:"claude_output"`
}

// handleApiUpdate allows external services to push cost and token updates manually
func handleApiUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	statsMu.Lock()
	if req.OpenAICost != nil {
		globalStats.OpenAICost = *req.OpenAICost
	}
	if req.OpenAIInputToken != nil {
		globalStats.OpenAIInputToken = *req.OpenAIInputToken
	}
	if req.OpenAIOutputToken != nil {
		globalStats.OpenAIOutputToken = *req.OpenAIOutputToken
	}
	if req.ClaudeCost != nil {
		globalStats.ClaudeCost = *req.ClaudeCost
	}
	if req.ClaudeInputToken != nil {
		globalStats.ClaudeInputToken = *req.ClaudeInputToken
	}
	if req.ClaudeOutputToken != nil {
		globalStats.ClaudeOutputToken = *req.ClaudeOutputToken
	}
	globalStats.LastUpdated = time.Now()
	statsMu.Unlock()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
	log.Println("Manual stats update received via API")
}

// API Poller Worker

func pollAPIs(openAIKey, orgID, claudeKey string) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// Initial fetch
	fetchAndUpdate(openAIKey, orgID, claudeKey)

	for range ticker.C {
		fetchAndUpdate(openAIKey, orgID, claudeKey)
	}
}

func fetchAndUpdate(openAIKey, orgID, claudeKey string) {
	log.Println("Fetching usage data from AI APIs...")
	now := time.Now()
	// Fetch today's usage from midnight to now in the local timezone
	year, month, day := now.Date()
	localMidnight := time.Date(year, month, day, 0, 0, 0, 0, now.Location())
	startTime := localMidnight.Unix()
	endTime := now.Unix()

	var (
		openaiCost, claudeCost float64
		openaiIn, openaiOut    int64
		claudeIn, claudeOut    int64
		errOpenAI, errClaude   error
	)

	if openAIKey != "" {
		openaiCost, openaiIn, openaiOut, errOpenAI = fetchOpenAI(openAIKey, orgID, startTime, endTime)
		if errOpenAI != nil {
			log.Printf("Error fetching OpenAI stats: %v\n", errOpenAI)
		}
	}

	if claudeKey != "" {
		claudeCost, claudeIn, claudeOut, errClaude = fetchClaude(claudeKey, localMidnight.Format(time.RFC3339))
		if errClaude != nil {
			log.Printf("Error fetching Claude stats: %v\n", errClaude)
		}
	}

	// Update cached values (only override if fetched successfully)
	statsMu.Lock()
	if openAIKey != "" && errOpenAI == nil {
		globalStats.OpenAICost = openaiCost
		globalStats.OpenAIInputToken = openaiIn
		globalStats.OpenAIOutputToken = openaiOut
	}
	if claudeKey != "" && errClaude == nil {
		globalStats.ClaudeCost = claudeCost
		globalStats.ClaudeInputToken = claudeIn
		globalStats.ClaudeOutputToken = claudeOut
	}
	globalStats.LastUpdated = time.Now()
	statsMu.Unlock()
}

// fetchOpenAI queries the OpenAI Organization Cost and Usage APIs
func fetchOpenAI(apiKey, orgID string, start, end int64) (float64, int64, int64, error) {
	// 1. Fetch Costs
	costURL := fmt.Sprintf("https://api.openai.com/v1/organization/costs?start_time=%d&end_time=%d", start, end)
	req, _ := http.NewRequest(http.MethodGet, costURL, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if orgID != "" {
		req.Header.Set("OpenAI-Organization", orgID)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := ioutil.ReadAll(resp.Body)
		return 0, 0, 0, fmt.Errorf("openai cost status %d: %s", resp.StatusCode, string(b))
	}

	var costResp struct {
		Data []struct {
			Amount struct {
				Value float64 `json:"value"`
			} `json:"amount"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&costResp); err != nil {
		return 0, 0, 0, err
	}

	var totalCost float64
	for _, item := range costResp.Data {
		totalCost += item.Amount.Value
	}

	// 2. Fetch Completion Tokens
	usageURL := fmt.Sprintf("https://api.openai.com/v1/organization/usage/completions?start_time=%d&end_time=%d", start, end)
	reqUsage, _ := http.NewRequest(http.MethodGet, usageURL, nil)
	reqUsage.Header.Set("Authorization", "Bearer "+apiKey)
	if orgID != "" {
		reqUsage.Header.Set("OpenAI-Organization", orgID)
	}

	respUsage, err := client.Do(reqUsage)
	if err != nil {
		return totalCost, 0, 0, err
	}
	defer respUsage.Body.Close()

	if respUsage.StatusCode != http.StatusOK {
		b, _ := ioutil.ReadAll(respUsage.Body)
		return totalCost, 0, 0, fmt.Errorf("openai usage status %d: %s", respUsage.StatusCode, string(b))
	}

	var usageResp struct {
		Data []struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"data"`
	}
	if err := json.NewDecoder(respUsage.Body).Decode(&usageResp); err != nil {
		return totalCost, 0, 0, err
	}

	var inputTokens, outputTokens int64
	for _, item := range usageResp.Data {
		inputTokens += item.InputTokens
		outputTokens += item.OutputTokens
	}

	return totalCost, inputTokens, outputTokens, nil
}

// fetchClaude queries the Anthropic /v1/organizations/usage_report/messages API (requires Admin key)
func fetchClaude(apiKey string, startingAt string) (float64, int64, int64, error) {
	url := fmt.Sprintf("https://api.anthropic.com/v1/organizations/usage_report/messages?starting_at=%s", startingAt)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := ioutil.ReadAll(resp.Body)
		return 0, 0, 0, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(b))
	}

	// Read and parse usage report
	var report struct {
		Data []struct {
			UncachedInputTokens int64   `json:"uncached_input_tokens"`
			OutputTokens        int64   `json:"output_tokens"`
			EstimatedCostUsd    float64 `json:"estimated_cost_usd"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return 0, 0, 0, err
	}

	var totalCost float64
	var inputTokens, outputTokens int64
	for _, item := range report.Data {
		totalCost += item.EstimatedCostUsd
		inputTokens += item.UncachedInputTokens // Approximates input tokens
		outputTokens += item.OutputTokens
	}

	return totalCost, inputTokens, outputTokens, nil
}
