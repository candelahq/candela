package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/candelahq/candela/pkg/cloudauth"
)

// DoctorCheck is a single diagnostic check result.
type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "warn", "fail", "skip"
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// DoctorReport is the full diagnostic report.
type DoctorReport struct {
	Checks  []DoctorCheck `json:"checks"`
	Summary struct {
		Pass int `json:"pass"`
		Warn int `json:"warn"`
		Fail int `json:"fail"`
		Skip int `json:"skip"`
	} `json:"summary"`
}

func (r *DoctorReport) add(name, status, msg, detail string) {
	r.Checks = append(r.Checks, DoctorCheck{
		Name:    name,
		Status:  status,
		Message: msg,
		Detail:  detail,
	})
	switch status {
	case "pass":
		r.Summary.Pass++
	case "warn":
		r.Summary.Warn++
	case "fail":
		r.Summary.Fail++
	case "skip":
		r.Summary.Skip++
	}
}

// cmdDoctorExpanded runs the full diagnostic suite.
func cmdDoctorExpanded() {
	jsonOutput := false
	fixMode := false
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--output":
			// Support --output json (split form).
			if i+1 < len(args) && args[i+1] == "json" {
				jsonOutput = true
				i++
			}
		case "--output=json":
			jsonOutput = true
		case "--fix":
			fixMode = true
		}
	}

	report := &DoctorReport{}

	// 1. CLI Version
	checkVersion(report)

	// 2. Config — resolve path once and use it everywhere.
	configPath := findConfigPath()
	cfg := loadConfigFromPath(configPath)
	checkConfig(report, configPath, cfg)

	// 3. Cloud Auth
	checkAuth(report)

	// 4. Proxy status — use doctor args for port resolution.
	port := resolvePort(args)
	checkProxy(report, port)

	// 5. Remote server connectivity
	checkServer(report, cfg)

	// 6. Budget
	checkBudget(report, cfg)

	// 7. Local runtime (Ollama / LM Studio)
	checkRuntime(report, cfg)

	// 8. State DB
	checkStateDB(report, cfg)

	// 9. Port conflicts — always included in report.
	checkPortConflicts(report, port, cfg)

	// JSON output
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		if report.Summary.Fail > 0 {
			os.Exit(1)
		}
		return
	}

	// Pretty output
	fmt.Println("🩺 Candela Doctor")
	fmt.Println()

	for _, c := range report.Checks {
		icon := statusIcon(c.Status)
		fmt.Printf("  %s %s: %s\n", icon, c.Name, c.Message)
		if c.Detail != "" {
			for _, line := range strings.Split(c.Detail, "\n") {
				fmt.Printf("      %s\n", line)
			}
		}
	}

	fmt.Println()
	fmt.Printf("  %d passed, %d warnings, %d failed, %d skipped\n",
		report.Summary.Pass, report.Summary.Warn, report.Summary.Fail, report.Summary.Skip)

	// --fix: kill conflicting processes (existing behavior).
	if fixMode {
		fmt.Println()
		cmdDoctorPortConflicts(true)
	}

	if report.Summary.Fail > 0 {
		os.Exit(1)
	}
}

func statusIcon(status string) string {
	switch status {
	case "pass":
		return "✅"
	case "warn":
		return "⚠️ "
	case "fail":
		return "❌"
	case "skip":
		return "⏭️ "
	default:
		return "?"
	}
}

// ── Individual checks ──

func checkVersion(r *DoctorReport) {
	detail := fmt.Sprintf("Go: %s, OS: %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if version == "dev" {
		r.add("CLI version", "warn", "dev build (no version tag)", detail)
	} else {
		r.add("CLI version", "pass", version, detail)
	}
}

func checkConfig(r *DoctorReport, configPath string, cfg *Config) {
	if configPath == "" {
		r.add("Config file", "warn", "no config file found",
			"Create ~/.config/candela/config.yaml or set CANDELA_CONFIG")
		return
	}

	issues := []string{}
	if cfg.Remote == "" && cfg.LocalUpstream == "" {
		issues = append(issues, "no remote or local_upstream configured")
	}
	if cfg.Port == 0 {
		issues = append(issues, "port not set (using default 8181)")
	}

	if len(issues) > 0 {
		r.add("Config file", "warn", configPath, strings.Join(issues, "; "))
	} else {
		detail := fmt.Sprintf("port: %d", cfg.Port)
		if cfg.Remote != "" {
			detail += fmt.Sprintf(", remote: %s", cfg.Remote)
		}
		if cfg.LocalUpstream != "" {
			detail += fmt.Sprintf(", upstream: %s", cfg.LocalUpstream)
		}
		r.add("Config file", "pass", configPath, detail)
	}
}

func checkAuth(r *DoctorReport) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	providers := cloudauth.All()
	if len(providers) == 0 {
		r.add("Cloud auth", "skip", "no providers registered", "")
		return
	}

	for _, p := range providers {
		status, err := p.Status(ctx)
		if err != nil {
			r.add("Auth: "+p.Name(), "fail", "error checking credentials",
				fmt.Sprintf("Run: candela auth login --provider %s", p.Name()))
			continue
		}
		if !status.Valid {
			r.add("Auth: "+p.Name(), "warn", "not configured",
				fmt.Sprintf("Run: candela auth login --provider %s", p.Name()))
		} else {
			msg := status.Account
			if status.ExpiresIn > 0 {
				msg += fmt.Sprintf(" (expires in %s)", cloudauth.FormatDuration(status.ExpiresIn))
			}
			r.add("Auth: "+p.Name(), "pass", msg, "")
		}
	}
}

func checkProxy(r *DoctorReport, port int) {
	// Check PID file.
	pidPath := pidFilePath()
	if pidPath == "" {
		r.add("Proxy", "fail", "cannot determine PID file path", "")
		return
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		r.add("Proxy", "warn", "not running",
			"Run: candela start")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !processRunning(pid) {
		r.add("Proxy", "warn", "not running (stale PID file)",
			"Run: candela start")
		return
	}

	// Health check.
	client := &http.Client{Timeout: 3 * time.Second}
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/_local/", port)
	resp, err := client.Get(healthURL)
	if err != nil {
		r.add("Proxy", "warn",
			fmt.Sprintf("PID %d running but not responding on :%d", pid, port),
			fmt.Sprintf("URL: %s", healthURL))
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode == 200 {
		r.add("Proxy", "pass",
			fmt.Sprintf("running (PID %d, port %d)", pid, port), "")
	} else {
		r.add("Proxy", "warn",
			fmt.Sprintf("PID %d responding with HTTP %d", pid, resp.StatusCode), "")
	}
}

func checkServer(r *DoctorReport, cfg *Config) {
	if cfg.Remote == "" {
		r.add("Remote server", "skip", "no remote configured", "")
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	healthURL := strings.TrimRight(cfg.Remote, "/") + "/healthz"
	resp, err := client.Get(healthURL)
	if err != nil {
		r.add("Remote server", "fail",
			fmt.Sprintf("cannot reach %s", cfg.Remote),
			fmt.Sprintf("Error: %v", err))
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode == 200 {
		r.add("Remote server", "pass",
			fmt.Sprintf("%s (HTTP %d)", cfg.Remote, resp.StatusCode), "")
	} else {
		r.add("Remote server", "warn",
			fmt.Sprintf("%s returned HTTP %d", cfg.Remote, resp.StatusCode),
			"This may be expected if behind IAP or other auth proxy")
	}
}

func checkBudget(r *DoctorReport, cfg *Config) {
	if cfg.MaxRequestCost <= 0 && len(cfg.DailyLimits) == 0 {
		r.add("Budget", "warn", "no spend limits configured",
			"Set max_request_cost or daily_limits in config to prevent runaway spend")
		return
	}

	details := []string{}
	if cfg.MaxRequestCost > 0 {
		details = append(details, fmt.Sprintf("max_request_cost: $%.2f", cfg.MaxRequestCost))
	}
	if len(cfg.DailyLimits) > 0 {
		details = append(details, fmt.Sprintf("%d daily limit rule(s)", len(cfg.DailyLimits)))
	}
	r.add("Budget", "pass", strings.Join(details, ", "), "")
}

func checkRuntime(r *DoctorReport, cfg *Config) {
	client := &http.Client{Timeout: 3 * time.Second}

	// Check Ollama.
	ollamaURL := "http://127.0.0.1:11434/api/version"
	if cfg.LocalUpstream != "" {
		ollamaURL = strings.TrimRight(cfg.LocalUpstream, "/") + "/api/version"
	}

	resp, err := client.Get(ollamaURL)
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			r.add("Ollama", "pass", "running on "+ollamaURL, "")
		} else {
			r.add("Ollama", "warn",
				fmt.Sprintf("responding but HTTP %d", resp.StatusCode), "")
		}
	} else {
		if _, lookErr := exec.LookPath("ollama"); lookErr != nil {
			r.add("Ollama", "skip", "not installed", "Install: https://ollama.com")
		} else {
			r.add("Ollama", "warn", "installed but not running",
				"Run: ollama serve")
		}
	}

	// Check LM Studio.
	lmPort := 1234
	if cfg.LMStudioPort != 0 {
		lmPort = cfg.LMStudioPort
	}
	lmURL := fmt.Sprintf("http://127.0.0.1:%d/v1/models", lmPort)
	resp, err = client.Get(lmURL)
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			r.add("LM Studio", "pass", fmt.Sprintf("running on port %d", lmPort), "")
		} else {
			r.add("LM Studio", "warn",
				fmt.Sprintf("port %d responded with HTTP %d (may not be LM Studio)", lmPort, resp.StatusCode), "")
		}
	} else {
		if _, lookErr := exec.LookPath("lms"); lookErr == nil {
			r.add("LM Studio", "warn", "CLI installed but server not running",
				"Run: lms server start")
		}
		// Don't report if not installed — it's optional.
	}
}

func checkStateDB(r *DoctorReport, cfg *Config) {
	dbPath := cfg.StateDBPath
	if dbPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dbPath = home + "/.candela/state.db"
		}
	}
	if dbPath == "" {
		r.add("State DB", "skip", "cannot determine path", "")
		return
	}

	info, err := os.Stat(dbPath)
	if err != nil {
		r.add("State DB", "warn", "not found (will be created on first run)",
			fmt.Sprintf("Path: %s", dbPath))
		return
	}

	sizeMB := float64(info.Size()) / 1024 / 1024
	r.add("State DB", "pass",
		fmt.Sprintf("%.1f MB", sizeMB),
		fmt.Sprintf("Path: %s", dbPath))
}

func checkPortConflicts(r *DoctorReport, port int, cfg *Config) {
	lmPort := 1234
	if cfg.LMStudioPort != 0 {
		lmPort = cfg.LMStudioPort
	}

	// Read our own PID file so we can exclude our own process.
	ownPID := 0
	if pidPath := pidFilePath(); pidPath != "" {
		if data, err := os.ReadFile(pidPath); err == nil {
			ownPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
	}

	ports := []struct {
		port int
		name string
	}{
		{port, "proxy"},
		{lmPort, "LM Studio compat"},
	}

	hasConflict := false
	for _, p := range ports {
		procs := findProcessesOnPort(p.port)
		for _, proc := range procs {
			if proc.pid == ownPID {
				continue
			}
			hasConflict = true
			r.add("Port conflict",
				"warn",
				fmt.Sprintf("PID %d (%s) is using port %d (%s)", proc.pid, proc.command, p.port, p.name),
				"Run: candela doctor --fix")
		}
	}

	if !hasConflict {
		r.add("Port conflicts", "pass", "no conflicts detected", "")
	}
}

// findConfigPath returns the path to the config file that loadConfig would use.
func findConfigPath() string {
	if path := os.Getenv("CANDELA_CONFIG"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		// Explicit CANDELA_CONFIG is set but file doesn't exist — return empty.
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil {
		dotConfigPath := home + "/.config/candela/config.yaml"
		if _, err := os.Stat(dotConfigPath); err == nil {
			return dotConfigPath
		}
	}
	if configDir, err := os.UserConfigDir(); err == nil {
		nativePath := configDir + "/candela/config.yaml"
		if _, err := os.Stat(nativePath); err == nil {
			return nativePath
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		legacyPath := home + "/.candela.yaml"
		if _, err := os.Stat(legacyPath); err == nil {
			return legacyPath
		}
	}
	return ""
}

// loadConfigFromPath loads config from a specific path, or returns defaults if empty.
func loadConfigFromPath(path string) *Config {
	if path == "" {
		return &Config{}
	}
	return loadConfig(path)
}
