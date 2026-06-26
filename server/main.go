package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io"

	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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
	if len(os.Args) > 1 && (os.Args[1] == "-test-cli" || os.Args[1] == "--test-cli") {
		runTestCLI()
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "-render-test" || os.Args[1] == "--render-test") {
		runRenderTest()
		return
	}

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
		GeminiCost:        2.45,
		GeminiWeeklyCost:  14.20,
		GeminiInputToken:  124500,
		GeminiOutputToken: 62000,
		ClaudeCost:        1.80,
		ClaudeWeeklyCost:  9.60,
		ClaudeInputToken:  45000,
		ClaudeOutputToken: 12500,
		LastUpdated:       time.Now(),
	}
	statsMu.Unlock()

	// Start background polling worker if API keys are set or Claude CLI is available
	geminiKey := os.Getenv("GEMINI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	claudeCLI := findClaudeCLI()

	if geminiKey != "" || anthropicKey != "" || claudeCLI != "" {
		log.Printf("Starting background API/CLI poller... (Claude CLI found: %s)\n", claudeCLI)
		go pollAPIs(geminiKey, anthropicKey, claudeCLI)
	} else {
		log.Println("No API keys or Claude CLI found. Running in Manual/Mock mode.")
		log.Println("You can push real-time stats to /api/update via HTTP POST.")
	}

	// Define HTTP routes
	http.HandleFunc("/display.bin", handleDisplayBin)
	http.HandleFunc("/debug.png", handleDebugPng)
	http.HandleFunc("/api/stats", handleApiStats)
	http.HandleFunc("/api/update", handleApiUpdate)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8296"
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
	GeminiCost          *float64 `json:"gemini_cost"`
	GeminiWeeklyCost    *float64 `json:"gemini_weekly_cost"`
	GeminiCostPct       *float64 `json:"gemini_cost_pct"`
	GeminiWeeklyCostPct *float64 `json:"gemini_weekly_cost_pct"`
	GeminiInputToken    *int64   `json:"gemini_input"`
	GeminiOutputToken   *int64   `json:"gemini_output"`
	ClaudeCost          *float64 `json:"claude_cost"`
	ClaudeWeeklyCost    *float64 `json:"claude_weekly_cost"`
	ClaudeCostPct       *float64 `json:"claude_cost_pct"`
	ClaudeWeeklyCostPct *float64 `json:"claude_weekly_cost_pct"`
	ClaudeInputToken    *int64   `json:"claude_input"`
	ClaudeOutputToken   *int64   `json:"claude_output"`
	ClaudeResetTime     *string  `json:"claude_reset_time"`
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
	if req.GeminiCost != nil {
		globalStats.GeminiCost = *req.GeminiCost
	} else if req.GeminiCostPct != nil {
		globalStats.GeminiCost = dailyBudget * (*req.GeminiCostPct / 100.0)
	}

	if req.GeminiWeeklyCost != nil {
		globalStats.GeminiWeeklyCost = *req.GeminiWeeklyCost
	} else if req.GeminiWeeklyCostPct != nil {
		globalStats.GeminiWeeklyCost = (dailyBudget * 7.0) * (*req.GeminiWeeklyCostPct / 100.0)
	}

	if req.GeminiInputToken != nil {
		globalStats.GeminiInputToken = *req.GeminiInputToken
	}
	if req.GeminiOutputToken != nil {
		globalStats.GeminiOutputToken = *req.GeminiOutputToken
	}

	if req.ClaudeCost != nil {
		globalStats.ClaudeCost = *req.ClaudeCost
	} else if req.ClaudeCostPct != nil {
		globalStats.ClaudeCost = dailyBudget * (*req.ClaudeCostPct / 100.0)
	}

	if req.ClaudeWeeklyCost != nil {
		globalStats.ClaudeWeeklyCost = *req.ClaudeWeeklyCost
	} else if req.ClaudeWeeklyCostPct != nil {
		globalStats.ClaudeWeeklyCost = (dailyBudget * 7.0) * (*req.ClaudeWeeklyCostPct / 100.0)
	}

	if req.ClaudeInputToken != nil {
		globalStats.ClaudeInputToken = *req.ClaudeInputToken
	}
	if req.ClaudeOutputToken != nil {
		globalStats.ClaudeOutputToken = *req.ClaudeOutputToken
	}
	if req.ClaudeResetTime != nil {
		globalStats.ClaudeResetTime = *req.ClaudeResetTime
	}
	globalStats.LastUpdated = time.Now()
	statsMu.Unlock()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
	log.Println("Manual stats update received via API")
}

// API Poller Worker

func pollAPIs(geminiKey, claudeKey, claudeCLI string) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// Initial fetch
	fetchAndUpdate(geminiKey, claudeKey, claudeCLI)

	for range ticker.C {
		fetchAndUpdate(geminiKey, claudeKey, claudeCLI)
	}
}

func fetchAndUpdate(geminiKey, claudeKey, claudeCLI string) {
	log.Println("Fetching usage data from AI APIs/CLI...")
	now := time.Now()
	// Fetch today's usage from midnight to now in the local timezone
	year, month, day := now.Date()
	localMidnight := time.Date(year, month, day, 0, 0, 0, 0, now.Location())

	var (
		geminiCost, claudeCost float64
		claudeWeeklyCost       float64
		claudeResetTime        string
		geminiIn, geminiOut    int64
		claudeIn, claudeOut    int64
		errGemini, errClaude   error
		hasClaudeCLIStats      bool
	)

	if geminiKey != "" {
		geminiCost, geminiIn, geminiOut, errGemini = fetchGemini(geminiKey)
		if errGemini != nil {
			log.Printf("Error fetching Gemini stats: %v\n", errGemini)
		}
	}

	if claudeKey != "" {
		claudeCost, claudeIn, claudeOut, errClaude = fetchClaude(claudeKey, localMidnight.Format(time.RFC3339))
		if errClaude != nil {
			log.Printf("Error fetching Claude stats from API: %v\n", errClaude)
		}
	} else if claudeCLI != "" {
		claudeCost, claudeWeeklyCost, claudeResetTime, errClaude = fetchClaudeFromCLI(claudeCLI)
		if errClaude != nil {
			log.Printf("Error fetching Claude stats from CLI: %v\n", errClaude)
		} else {
			hasClaudeCLIStats = true
		}
	}

	// Update cached values (only override if fetched successfully)
	statsMu.Lock()
	if geminiKey != "" && errGemini == nil {
		globalStats.GeminiCost = geminiCost
		globalStats.GeminiInputToken = geminiIn
		globalStats.GeminiOutputToken = geminiOut
	}
	if errClaude == nil {
		if claudeKey != "" {
			globalStats.ClaudeCost = claudeCost
			globalStats.ClaudeInputToken = claudeIn
			globalStats.ClaudeOutputToken = claudeOut
		} else if claudeCLI != "" && hasClaudeCLIStats {
			globalStats.ClaudeCost = claudeCost
			globalStats.ClaudeWeeklyCost = claudeWeeklyCost
			globalStats.ClaudeResetTime = claudeResetTime
			// Reset tokens since CLI usage report doesn't provide fine-grained tokens
			globalStats.ClaudeInputToken = 0
			globalStats.ClaudeOutputToken = 0
		}
	}
	globalStats.LastUpdated = time.Now()
	statsMu.Unlock()
}

// findClaudeCLI searches for the absolute path of the `claude` executable.
func findClaudeCLI() string {
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	// Check common location on macOS
	commonPath := "/Users/jaredwarren/.local/bin/claude"
	if _, err := os.Stat(commonPath); err == nil {
		return commonPath
	}
	// Check relative to user home directory
	if home, err := os.UserHomeDir(); err == nil {
		homePath := home + "/.local/bin/claude"
		if _, err := os.Stat(homePath); err == nil {
			return homePath
		}
	}
	return ""
}

// fetchClaudeFromCLI executes `claude -p "/usage"` and parses the percentages/dollar usage values and reset time.
func fetchClaudeFromCLI(binPath string) (float64, float64, string, error) {
	cmd := exec.Command(binPath, "-p", "/usage")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run with 15-second timeout to avoid hanging indefinitely
	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.Run()
	}()

	var err error
	select {
	case err = <-errChan:
		// Completed
	case <-time.After(15 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return 0, 0, "", fmt.Errorf("command execution timed out")
	}

	if err != nil {
		return 0, 0, "", fmt.Errorf("command failed: %w (stderr: %s)", err, stderr.String())
	}

	outputStr := stdout.String()

	// Parse session usage
	sessionRegex := regexp.MustCompile(`Current session:\s*(?:(\d+)%\s*used|\$(\d+(?:\.\d+)?)\s*used)`)
	sessionMatches := sessionRegex.FindStringSubmatch(outputStr)

	statsMu.RLock()
	budget := dailyBudget
	statsMu.RUnlock()

	var sessionCost float64
	if len(sessionMatches) > 0 {
		if sessionMatches[1] != "" {
			pct, _ := strconv.ParseFloat(sessionMatches[1], 64)
			sessionCost = budget * (pct / 100.0)
		} else if sessionMatches[2] != "" {
			usd, _ := strconv.ParseFloat(sessionMatches[2], 64)
			sessionCost = usd
		}
	} else {
		return 0, 0, "", fmt.Errorf("failed to parse session usage from CLI output: %q", outputStr)
	}

	// Parse weekly usage
	weeklyRegex := regexp.MustCompile(`Current week.*:\s*(?:(\d+)%\s*used|\$(\d+(?:\.\d+)?)\s*used)`)
	weeklyMatches := weeklyRegex.FindStringSubmatch(outputStr)

	var weeklyCost float64
	if len(weeklyMatches) > 0 {
		if weeklyMatches[1] != "" {
			pct, _ := strconv.ParseFloat(weeklyMatches[1], 64)
			weeklyCost = (budget * 7.0) * (pct / 100.0)
		} else if weeklyMatches[2] != "" {
			usd, _ := strconv.ParseFloat(weeklyMatches[2], 64)
			weeklyCost = usd
		}
	} else {
		return 0, 0, "", fmt.Errorf("failed to parse weekly usage from CLI output: %q", outputStr)
	}

	// Parse reset time
	resetRegex := regexp.MustCompile(`resets\s+([^\n\r]+)`)
	resetMatches := resetRegex.FindStringSubmatch(outputStr)
	var resetTime string
	if len(resetMatches) > 1 {
		resetTime = strings.TrimSpace(resetMatches[1])
	}

	return sessionCost, weeklyCost, resetTime, nil
}

// runTestCLI runs the claude CLI directly and outputs the raw output and regex parsing results to the terminal.
func runTestCLI() {
	claudeCLI := findClaudeCLI()
	if claudeCLI == "" {
		fmt.Println("Error: Claude CLI not found on this system.")
		os.Exit(1)
	}
	fmt.Printf("Found Claude CLI at: %s\n", claudeCLI)
	fmt.Println("Running: claude -p /usage...")

	cmd := exec.Command(claudeCLI, "-p", "/usage")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error running command: %v\n", err)
		os.Exit(1)
	}

	outputStr := string(out)
	fmt.Println("\n--- Claude CLI Output ---")
	fmt.Print(outputStr)
	fmt.Println("-------------------------\n")

	sessionRegex := regexp.MustCompile(`Current session:\s*(?:(\d+)%\s*used|\$(\d+(?:\.\d+)?)\s*used)`)
	sessionMatches := sessionRegex.FindStringSubmatch(outputStr)

	var sessionCost float64
	if len(sessionMatches) > 0 {
		if sessionMatches[1] != "" {
			pct, _ := strconv.ParseFloat(sessionMatches[1], 64)
			sessionCost = dailyBudget * (pct / 100.0)
			fmt.Printf("Parsed Claude Session Usage: %s%% -> Mapped Cost: $%0.2f (out of $%0.2f)\n", sessionMatches[1], sessionCost, dailyBudget)
		} else if sessionMatches[2] != "" {
			usd, _ := strconv.ParseFloat(sessionMatches[2], 64)
			sessionCost = usd
			fmt.Printf("Parsed Claude Session Cost: $%0.2f (out of $%0.2f)\n", sessionCost, dailyBudget)
		}
	} else {
		fmt.Println("Warning: Could not parse session usage from CLI output.")
	}

	weeklyRegex := regexp.MustCompile(`Current week.*:\s*(?:(\d+)%\s*used|\$(\d+(?:\.\d+)?)\s*used)`)
	weeklyMatches := weeklyRegex.FindStringSubmatch(outputStr)

	var weeklyCost float64
	if len(weeklyMatches) > 0 {
		if weeklyMatches[1] != "" {
			pct, _ := strconv.ParseFloat(weeklyMatches[1], 64)
			weeklyCost = (dailyBudget * 7.0) * (pct / 100.0)
			fmt.Printf("Parsed Claude Weekly Usage: %s%% -> Mapped Cost: $%0.2f (out of $%0.2f)\n", weeklyMatches[1], weeklyCost, dailyBudget*7.0)
		} else if weeklyMatches[2] != "" {
			usd, _ := strconv.ParseFloat(weeklyMatches[2], 64)
			weeklyCost = usd
			fmt.Printf("Parsed Claude Weekly Cost: $%0.2f (out of $%0.2f)\n", weeklyCost, dailyBudget*7.0)
		}
	} else {
		fmt.Println("Warning: Could not parse weekly usage from CLI output.")
	}

	resetRegex := regexp.MustCompile(`resets\s+([^\n\r]+)`)
	resetMatches := resetRegex.FindStringSubmatch(outputStr)
	if len(resetMatches) > 1 {
		fmt.Printf("Parsed Claude Reset Time: %s\n", strings.TrimSpace(resetMatches[1]))
	} else {
		fmt.Println("Warning: Could not parse weekly reset time from CLI output.")
	}

	fmt.Println("\n--- Gemini Local Memory/Mock Stats ---")
	fmt.Printf("Gemini Session Cost:  $%0.2f (out of $%0.2f)\n", globalStats.GeminiCost, dailyBudget)
	fmt.Printf("Gemini Weekly Cost:   $%0.2f (out of $%0.2f)\n", globalStats.GeminiWeeklyCost, dailyBudget*7.0)
	fmt.Printf("Gemini Input Tokens:  %s\n", formatTokens(globalStats.GeminiInputToken))
	fmt.Printf("Gemini Output Tokens: %s\n", formatTokens(globalStats.GeminiOutputToken))
	fmt.Println("--------------------------------------\n")

	// Attempt to query the live running server if active
	port := os.Getenv("PORT")
	if port == "" {
		port = "8296"
	}
	url := fmt.Sprintf("http://localhost:%s/api/stats", port)
	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(url)
	if err == nil {
		defer resp.Body.Close()
		var liveStats Stats
		if err := json.NewDecoder(resp.Body).Decode(&liveStats); err == nil {
			fmt.Printf("--- Live Running Server Stats (Port %s) ---\n", port)
			fmt.Printf("Gemini Session Cost:  $%0.2f (out of $%0.2f)\n", liveStats.GeminiCost, dailyBudget)
			fmt.Printf("Gemini Weekly Cost:   $%0.2f (out of $%0.2f)\n", liveStats.GeminiWeeklyCost, dailyBudget*7.0)
			fmt.Printf("Gemini Input Tokens:  %s\n", formatTokens(liveStats.GeminiInputToken))
			fmt.Printf("Gemini Output Tokens: %s\n", formatTokens(liveStats.GeminiOutputToken))
			fmt.Printf("Claude Session Cost:  $%0.2f (out of $%0.2f)\n", liveStats.ClaudeCost, dailyBudget)
			fmt.Printf("Claude Weekly Cost:   $%0.2f (out of $%0.2f)\n", liveStats.ClaudeWeeklyCost, dailyBudget*7.0)
			fmt.Printf("Claude Reset Time:    %s\n", liveStats.ClaudeResetTime)
			fmt.Printf("Last Synchronized:    %s\n", liveStats.LastUpdated.Format("15:04:05"))
			fmt.Println("---------------------------------------------\n")
		}
	}
}

// fetchGemini is a stub/placeholder because Gemini API does not expose a single-key organizational usage/cost endpoint.
// We recommend pushing actual Gemini token/cost metrics manually via POST /api/update.
func fetchGemini(apiKey string) (float64, int64, int64, error) {
	log.Println("Note: Gemini API does not provide a billing cost query endpoint. Please push real-time cost updates manually to /api/update.")
	return 0, 0, 0, nil
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
		b, _ := io.ReadAll(resp.Body)
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

func runRenderTest() {
	var err error
	renderer, err = NewRenderer("assets/Roboto-Regular.ttf", "assets/Roboto-Bold.ttf")
	if err != nil {
		log.Fatalf("Error initializing renderer: %v", err)
	}

	testStats := Stats{
		ClaudeCost:        2.35,
		ClaudeWeeklyCost:  21.70,
		ClaudeInputToken:  1245000,
		ClaudeOutputToken: 320000,
		ClaudeResetTime:   "Jun 27 at 5:59am (America/Denver)",
		LastUpdated:       time.Now(),
	}

	img := renderer.DrawDashboard(testStats, 10.0)

	outFile := "/Users/jaredwarren/.gemini/antigravity-ide/brain/6d9a89a2-df65-42f3-b861-afd823655d53/debug_dashboard.png"
	f, err := os.Create(outFile)
	if err != nil {
		log.Fatalf("Failed to create test output file: %v", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		log.Fatalf("Failed to encode PNG: %v", err)
	}

	fmt.Printf("Successfully generated test dashboard render at: %s\n", outFile)
}
