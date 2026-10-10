package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingCategoriesCreatesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "categories.json")
	got, err := LoadCategories(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 11 || got[0] != "Development" {
		t.Fatalf("categories=%v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidSettingsArePreserved(t *testing.T) {
	for _, data := range []string{"broken JSON", "[]", "null", "[\" \"]"} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "categories.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadCategories(path); err == nil {
				t.Fatal("expected settings error")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != data {
				t.Fatal("user settings were overwritten")
			}
		})
	}
}
