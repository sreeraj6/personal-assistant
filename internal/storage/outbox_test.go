package storage

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestEntryAndQueueCommitTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracker.xlsx")
	w := NewWorkbook(path)
	if err := w.Save(entry(10)); err != nil {
		t.Fatal(err)
	}
	p, err := NewWorkbook(path).Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 || p[0].Entry.ID == "" || p[0].Entry.Activity != entry(10).Activity {
		t.Fatalf("pending=%v", p)
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	visible, err := f.GetSheetVisible(outboxSheet)
	if err != nil {
		t.Fatal(err)
	}
	if visible {
		t.Fatal("outbox should be hidden")
	}
	if err := w.Reserve(p[0].Entry.ID, "sheet-id", "Activities", 2); err != nil {
		t.Fatal(err)
	}
	p, err = NewWorkbook(path).Pending()
	if err != nil {
		t.Fatal(err)
	}
	if p[0].Row != 2 || p[0].SpreadsheetID != "sheet-id" {
		t.Fatalf("reservation=%v", p)
	}
	if err := w.Acknowledge(p[0].Entry.ID); err != nil {
		t.Fatal(err)
	}
	if rows := readRows(t, path, "2026-10-10"); len(rows) != 2 {
		t.Fatal("acknowledgment changed daily entries")
	}
	p, err = w.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 0 {
		t.Fatal("acknowledged entry remains queued")
	}
}
