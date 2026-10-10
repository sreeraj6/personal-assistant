// Package config owns paths and category settings for the tracker.
package config

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed categories.json
var defaults embed.FS

const ReminderInterval = 30 * time.Minute

type Paths struct {
	Directory  string
	Workbook   string
	Categories string
	Trigger    string
}

func UserPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("find home directory: %w", err)
	}
	dir := filepath.Join(home, ".metracker")
	return Paths{dir, filepath.Join(dir, "me_tracker.xlsx"), filepath.Join(dir, "categories.json"), filepath.Join(dir, "add-entry.trigger")}, nil
}

// LoadCategories creates defaults only when the file is missing. Invalid or
// unreadable user settings are reported without overwriting them.
func LoadCategories(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data, err = defaults.ReadFile("categories.json")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return nil, fmt.Errorf("create categories: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read categories: %w", err)
	}
	var categories []string
	if err := json.Unmarshal(data, &categories); err != nil {
		return nil, fmt.Errorf("parse categories %s: %w", path, err)
	}
	if len(categories) == 0 {
		return nil, fmt.Errorf("categories %s must not be empty", path)
	}
	for i, category := range categories {
		categories[i] = strings.TrimSpace(category)
		if categories[i] == "" {
			return nil, fmt.Errorf("category %d must not be blank", i+1)
		}
	}
	return categories, nil
}
