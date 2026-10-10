package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveEntry(t *testing.T) {
	tmp := t.TempDir()
	workbookPath = filepath.Join(tmp, "test.xlsx")

	if err := saveEntry("Development", 45, "Wrote API endpoint"); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	if err := saveEntry("Meetings", 30, "Daily standup"); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	info, err := os.Stat(workbookPath)
	if err != nil {
		t.Fatalf("workbook not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("workbook is empty")
	}

	t.Logf("workbook created: %s (size %d bytes)", workbookPath, info.Size())
}

func TestTodaySheet(t *testing.T) {
	want := time.Now().Format("2006-01-02")
	if got := todaySheet(); got != want {
		t.Fatalf("todaySheet() = %s, want %s", got, want)
	}
}
