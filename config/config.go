package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/olohmann/my-voice-cli/profiles"
)

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

// InitProfiles writes embedded default profiles to the user's config directory.
// Existing files are not overwritten.
func InitProfiles(configDir string) error {
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
