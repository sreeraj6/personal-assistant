package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func entry(day int) Entry {
	return Entry{Time: time.Date(2026, 10, day, 23, 59, 59, 0, time.FixedZone("IST", 19800)), Category: "Development", Minutes: 45, Activity: "Wrote API endpoint"}
}

func readRows(t *testing.T, path, sheet string) [][]string {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestSaveHeadersAppendAndNewDay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	w := NewWorkbook(path)
	for _, e := range []Entry{entry(10), entry(10), entry(11)} {
		if err := w.Save(e); err != nil {
			t.Fatal(err)
		}
	}
	rows := readRows(t, path, "2026-10-10")
	want := [][]string{{"Time", "Category", "Minutes", "Hours", "Activity"}, {"23:59:59", "Development", "45", "0.75", "Wrote API endpoint"}, {"23:59:59", "Development", "45", "0.75", "Wrote API endpoint"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %#v; want %#v", rows, want)
	}
	if rows := readRows(t, path, "2026-10-11"); len(rows) != 2 {
		t.Fatalf("new day rows = %#v", rows)
	}
}

func TestConcurrentSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	w := NewWorkbook(path)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Save(entry(10)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := len(readRows(t, path, "2026-10-10")); got != 13 {
		t.Fatalf("got %d rows, want 13", got)
	}
}

func TestRepairLegacyHeadersPreservesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", "2026-10-10"); err != nil {
		t.Fatal(err)
	}
	old := []interface{}{"10:00:00", "Meetings", 30, 0.5, "Standup"}
	if err := f.SetSheetRow("2026-10-10", "A2", &old); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := NewWorkbook(path).Save(entry(10)); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, path, "2026-10-10")
	if len(rows) != 3 || rows[0][0] != "Time" || rows[1][4] != "Standup" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestInvalidEntriesDoNotCreateWorkbook(t *testing.T) {
	for _, mutate := range []func(*Entry){func(e *Entry) { e.Minutes = 0 }, func(e *Entry) { e.Category = " " }, func(e *Entry) { e.Activity = " " }, func(e *Entry) { e.Time = time.Time{} }} {
		path := filepath.Join(t.TempDir(), "test.xlsx")
		e := entry(10)
		mutate(&e)
		if err := NewWorkbook(path).Save(e); err == nil {
			t.Fatal("expected validation error")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid entry wrote workbook: %v", err)
		}
	}
}

func TestCorruptWorkbookIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	original := []byte("not an Excel workbook")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := NewWorkbook(path).Save(entry(10)); err == nil {
		t.Fatal("expected open error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatal("corrupt workbook was replaced")
	}
}
