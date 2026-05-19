package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// configFilePath returns the path to the candela config file.
// Uses the same search order as loadConfig: CANDELA_CONFIG, then
// ~/.config/candela/config.yaml.
func configFilePath() string {
	if p := os.Getenv("CANDELA_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "candela", "config.yaml")
}

// upsertConfigProvider ensures the given provider name appears in the
// config file's `providers:` list. If it already exists, returns false.
// If the file doesn't exist, creates it with the provider entry.
//
// This performs simple text-level insertion to preserve comments and
// formatting in the existing config file, rather than round-tripping
// through a YAML parser which would strip comments.
func upsertConfigProvider(providerName string) (changed bool, err error) {
	path := configFilePath()
	if path == "" {
		return false, fmt.Errorf("cannot determine config file path")
	}

	// Map cloud auth provider names to config provider names.
	configName := providerName
	switch providerName {
	case "gcp":
		configName = "google"
	case "aws":
		configName = "anthropic-bedrock"
	}

	// Read existing file (may not exist).
	data, readErr := os.ReadFile(path)
	content := ""
	if readErr == nil {
		content = string(data)
	}

	// Check if provider is already configured.
	if content != "" {
		lines := strings.Split(content, "\n")
		inProviders := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "providers:" {
				inProviders = true
				continue
			}
			// Detect end of providers block: a non-indented, non-empty,
			// non-comment line means we've hit the next top-level key.
			isIndented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
			if inProviders && !isIndented && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				inProviders = false
			}
			if inProviders && strings.Contains(trimmed, "name:") {
				// Extract name value.
				parts := strings.SplitN(trimmed, ":", 2)
				if len(parts) == 2 {
					val := strings.TrimSpace(parts[1])
					val = strings.Trim(val, `"'`)
					if val == configName {
						return false, nil // already present
					}
				}
			}
		}
	}

	// Build the entry to add.
	entry := fmt.Sprintf("  - name: %s\n", configName)

	if content == "" {
		// Create a new config file.
		newContent := "providers:\n" + entry
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return false, fmt.Errorf("create config directory: %w", err)
		}
		return true, os.WriteFile(path, []byte(newContent), 0o644)
	}

	// Append to existing providers block, or create one.
	if strings.Contains(content, "providers:") {
		// Find the providers: line and insert after it (before the first
		// non-provider entry).
		lines := strings.Split(content, "\n")
		var result []string
		inserted := false
		inProviders := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "providers:" {
				inProviders = true
				result = append(result, line)
				continue
			}
			// Insert at the end of the providers block.
			if inProviders && !inserted {
				isIndented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
				if !isIndented && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
					// We've reached the next top-level key — insert before it.
					result = append(result, strings.TrimSuffix(entry, "\n"))
					inserted = true
					inProviders = false
				}
			}
			result = append(result, line)
		}
		// If still in providers block at EOF, append there.
		if !inserted {
			result = append(result, strings.TrimSuffix(entry, "\n"))
		}
		return true, os.WriteFile(path, []byte(strings.Join(result, "\n")), 0o644)
	}

	// No providers block exists — append one at the end.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\nproviders:\n" + entry
	return true, os.WriteFile(path, []byte(content), 0o644)
}
