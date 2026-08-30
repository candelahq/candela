package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/candelahq/candela/pkg/forecast"
)

func cmdForecast() {
	// Parse flags.
	jsonOutput := false
	userID := "me"
	for i := 0; i < len(os.Args[2:]); i++ {
		arg := os.Args[2+i]
		switch {
		case arg == "--json":
			jsonOutput = true
		case arg == "--user" && i+1 < len(os.Args[2:]):
			userID = os.Args[3+i]
			i++
		case strings.HasPrefix(arg, "--user="):
			userID = strings.TrimPrefix(arg, "--user=")
		}
	}

	// Use the local proxy — it forwards to the remote server with IAP auth.
	port := resolvePort(os.Args[2:])
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Check if proxy is running.
	pidPath := pidFilePath()
	if pidPath != "" {
		data, err := os.ReadFile(pidPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: proxy not running. Start it first:")
			fmt.Fprintln(os.Stderr, "  candela start")
			os.Exit(1)
		}
		pid, _ := fmt.Sscanf(string(data), "%d", new(int))
		_ = pid
	}

	// Call the REST API through the local proxy.
	url := baseURL + "/api/v1/users/" + userID + "/budget-forecast"
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot reach proxy at %s\n  %v\n\nIs the proxy running? Try: candela start\n", baseURL, err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Error: HTTP %d\n  %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result forecast.Result
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	// Pretty output.
	printForecast(result, userID)
}

func printForecast(r forecast.Result, userID string) {
	label := userID
	if label == "me" {
		label = "you"
	}

	fmt.Printf("Budget Forecast for %s\n", label)
	fmt.Println("─────────────────────────")

	if r.BurnRatePerHour == 0 && r.AvgDailySpend == 0 {
		fmt.Println("  No budget data available.")
		fmt.Println("  This may mean no budget is configured or there's no spend history.")
		return
	}

	// Burn rate & projection.
	fmt.Printf("  Burn Rate: $%.2f/hr\n", r.BurnRatePerHour)
	fmt.Printf("  Projected: $%.2f EOD", r.ProjectedEODSpend)
	if r.WillExceedBudget {
		fmt.Print(" ⚠️  WILL EXCEED")
	}
	fmt.Println()

	// 7-day history sparkline.
	if len(r.SpendHistory) > 0 {
		fmt.Println()
		fmt.Println("  7-Day History:")

		maxSpend := 0.0
		for _, d := range r.SpendHistory {
			if d.SpendUSD > maxSpend {
				maxSpend = d.SpendUSD
			}
		}
		if maxSpend < 0.01 {
			maxSpend = 0.01
		}

		for _, d := range r.SpendHistory {
			bars := int((d.SpendUSD / maxSpend) * 10)
			day := dayOfWeek(d.Date)
			filled := strings.Repeat("█", bars)
			empty := strings.Repeat("░", 10-bars)
			fmt.Printf("    %s  %s%s  $%.2f\n", day, filled, empty, d.SpendUSD)
		}
	}

	// Average and exhaustion.
	fmt.Println()
	if r.AvgDailySpend > 0 {
		fmt.Printf("  Avg Daily: $%.2f\n", r.AvgDailySpend)
	}

	if r.DaysUntilExhaustion == 0 {
		fmt.Println("  Exhaustion: 🚫 Budget exhausted TODAY")
	} else if r.DaysUntilExhaustion > 0 {
		dateStr := formatCLIDate(r.EstimatedExhaustion)
		fmt.Printf("  Exhaustion: %s (%d days)\n", dateStr, r.DaysUntilExhaustion)
	}
}

func dayOfWeek(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr[:3]
	}
	return t.Weekday().String()[:3]
}

func formatCLIDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("Mon Jan 2")
}
