package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/candelahq/candela/pkg/processor"
)

func cmdWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	port := fs.Int("port", 0, "server port (default: from config or 8181)")
	project := fs.String("project", "", "filter by project ID")
	model := fs.String("model", "", "filter by model")
	provider := fs.String("provider", "", "filter by provider")
	jsonOut := fs.Bool("json", false, "output as JSON lines")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "invalid flags: %v\n", err)
		os.Exit(1)
	}

	// Resolve port
	p := *port
	if p == 0 {
		cfg := loadConfig("")
		if cfg != nil && cfg.Port != 0 {
			p = cfg.Port
		} else {
			p = 8181
		}
	}

	// Build SSE URL
	params := url.Values{}
	if *project != "" {
		params.Set("project", *project)
	}
	if *model != "" {
		params.Set("model", *model)
	}
	if *provider != "" {
		params.Set("provider", *provider)
	}
	sseURL := fmt.Sprintf("http://127.0.0.1:%d/_local/api/watch?%s", p, params.Encode())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !*jsonOut {
		fmt.Println("🕯️  candela watch — streaming traces (Ctrl+C to stop)")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	}

	var count int
	start := time.Now()

	// Connect to SSE with retry
	for {
		err := streamTraces(ctx, sseURL, *jsonOut, &count)
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  connection lost: %v — retrying in 2s...\n", err)
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
			}
		}
	}

	if !*jsonOut {
		fmt.Printf("\n📊 Watched %d traces in %s\n", count, time.Since(start).Round(time.Second))
	}
}

func streamTraces(ctx context.Context, sseURL string, jsonOut bool, count *int) error {
	req, err := http.NewRequestWithContext(ctx, "GET", sseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:] // strip "data: "

		*count++

		if jsonOut {
			fmt.Println(data)
			continue
		}

		var t processor.TraceBroadcast
		if err := json.Unmarshal([]byte(data), &t); err != nil {
			continue
		}
		printTraceRow(t)
	}
	return scanner.Err()
}

func printTraceRow(t processor.TraceBroadcast) {
	statusIcon := "✅"
	if t.Status == "error" {
		statusIcon = "❌"
	}

	name := truncateRunes(t.RootSpanName, 20)

	model := truncateRunes(t.Model, 14)

	spans := "span"
	if t.SpanCount != 1 {
		spans = "spans"
	}

	fmt.Printf("%s  %s  %-20s  %-14s  $%.4f  %4dms  %d %s\n",
		t.Timestamp, statusIcon, name, model, t.CostUSD, t.DurationMs, t.SpanCount, spans)
}

// truncateRunes truncates s to at most maxRunes Unicode characters.
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return s
}
