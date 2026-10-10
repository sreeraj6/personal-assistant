// Package googlesync uploads durable workbook requests to Google Sheets.
package googlesync

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Config struct {
	SpreadsheetID string `json:"spreadsheet_id"`
	Tab           string `json:"tab"`
}

func LoadConfig(dir string) (*Config, error) {
	data, err := os.ReadFile(filepath.Join(dir, "google-sync.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("read Google sync settings: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func ParseSheetID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "https://") {
		u, err := url.Parse(value)
		if err != nil || u.Host != "docs.google.com" {
			return "", errors.New("use a docs.google.com spreadsheet URL or its ID")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 3 || parts[0] != "spreadsheets" || parts[1] != "d" {
			return "", errors.New("invalid Google Sheet URL")
		}
		value = parts[2]
	}
	if !validID.MatchString(value) {
		return "", errors.New("invalid spreadsheet ID")
	}
	return value, nil
}

func (c Config) Validate() error {
	if !validID.MatchString(c.SpreadsheetID) {
		return errors.New("invalid spreadsheet ID in Google sync settings")
	}
	if strings.TrimSpace(c.Tab) == "" {
		return errors.New("Google sync tab must not be blank")
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".google-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
