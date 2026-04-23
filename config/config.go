package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/olohmann/my-voice-cli/profiles"
)

// AppConfig holds persistent settings loaded from config.toml.
type AppConfig struct {
	Model  string `toml:"model"`
	Tone   string `toml:"tone"`
	Format string `toml:"format"`
}

// DefaultConfig returns hardcoded defaults.
func DefaultConfig() AppConfig {
	return AppConfig{
		Model:  "gpt-4.1",
		Tone:   "formal",
		Format: "mail",
	}
}

// LoadConfig reads config.toml from the config directory.
// Missing file is not an error — defaults are returned.
func LoadConfig(configDir string) (AppConfig, error) {
	cfg := DefaultConfig()
	path := filepath.Join(configDir, "config.toml")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("reading config.toml: %w", err)
	}

	// Fill in any missing fields with defaults
	defaults := DefaultConfig()
	if cfg.Model == "" {
		cfg.Model = defaults.Model
	}
	if cfg.Tone == "" {
		cfg.Tone = defaults.Tone
	}
	if cfg.Format == "" {
		cfg.Format = defaults.Format
	}

	return cfg, nil
}

const defaultConfigTOML = `# my-voice configuration
# CLI flags override these defaults.

# Default LLM model
model = "gpt-4.1"

# Default tone: "formal" or "casual"
tone = "formal"

# Default format: "mail" or "chat"
format = "mail"
`

const appName = "my-voice"

// ProfileKey returns the filename (without extension) for the given tone and format.
func ProfileKey(tone, format string) string {
	return tone + "-" + format
}

// ConfigDir returns the configuration directory, respecting XDG_CONFIG_HOME.
func ConfigDir(override string) string {
	if override != "" {
		return override
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, appName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", appName)
}

// ProfilesDir returns the profiles subdirectory.
func ProfilesDir(configDir string) string {
	return filepath.Join(configDir, "profiles")
}

// LoadProfile reads the system prompt for the given tone+format combination.
// It first checks the user's config dir; if not found, falls back to embedded defaults.
func LoadProfile(configDir, tone, format string) (string, error) {
	key := ProfileKey(tone, format)
	filename := key + ".md"

	// Try user config dir first
	userPath := filepath.Join(ProfilesDir(configDir), filename)
	if data, err := os.ReadFile(userPath); err == nil {
		return string(data), nil
	}

	// Fall back to embedded defaults
	data, err := profiles.DefaultProfiles.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("profile %q not found: %w", key, err)
	}
	return string(data), nil
}

// ListProfiles returns available profiles from the user config dir and embedded defaults.
func ListProfiles(configDir string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	// Check user profiles dir
	profilesDir := ProfilesDir(configDir)
	if entries, err := os.ReadDir(profilesDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
				name := e.Name()[:len(e.Name())-3]
				if !seen[name] {
					seen[name] = true
					result = append(result, name+" (custom)")
				}
			}
		}
	}

	// Check embedded defaults
	entries, err := fs.ReadDir(profiles.DefaultProfiles, ".")
	if err != nil {
		return result, err
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			name := e.Name()[:len(e.Name())-3]
			if !seen[name] {
				seen[name] = true
				result = append(result, name+" (default)")
			}
		}
	}

	return result, nil
}

// Init writes default config.toml and profile files to the user's config directory.
// Existing files are not overwritten.
func Init(configDir string) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Write config.toml
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(defaultConfigTOML), 0o644); err != nil {
			return fmt.Errorf("writing config.toml: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  created: config.toml\n")
	} else {
		fmt.Fprintf(os.Stderr, "  skip: config.toml (already exists)\n")
	}

	// Write profile files
	dir := ProfilesDir(configDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating profiles directory: %w", err)
	}

	entries, err := fs.ReadDir(profiles.DefaultProfiles, ".")
	if err != nil {
		return err
	}

	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".md" {
			continue
		}
		dest := filepath.Join(dir, e.Name())
		if _, err := os.Stat(dest); err == nil {
			fmt.Fprintf(os.Stderr, "  skip: %s (already exists)\n", e.Name())
			continue
		}
		data, err := profiles.DefaultProfiles.ReadFile(e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", e.Name(), err)
		}
		fmt.Fprintf(os.Stderr, "  created: %s\n", e.Name())
	}
	return nil
}
