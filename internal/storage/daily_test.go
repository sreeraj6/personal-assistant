package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDailyShowsOnlySelectedDay(t *testing.T) {
	w := NewWorkbook(filepath.Join(t.TempDir(), "tracker.xlsx"))
	rows, err := w.Daily(time.Now())
	if err != nil || len(rows) != 0 {
		t.Fatalf("new workbook: %v %v", rows, err)
	}
	for _, e := range []Entry{entry(10), entry(11), entry(10)} {
		if err := w.Save(e); err != nil {
			t.Fatal(err)
		}
	}
	rows, err = w.Daily(entry(10).Time)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Minutes != 45 || rows[0].Activity != "Wrote API endpoint" {
		t.Fatalf("daily rows=%v", rows)
	}
	rows, err = w.Daily(entry(12).Time)
	if err != nil || len(rows) != 0 {
		t.Fatalf("missing day: %v %v", rows, err)
	}
}
