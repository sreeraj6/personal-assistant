package googlesync

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"metracker/internal/storage"
)

type fakeRemote struct {
	rows      [][]any
	readErr   error
	writeErr  error
	uncertain bool
	writes    []int
}

func (f *fakeRemote) Read(context.Context) ([][]any, error) { return f.rows, f.readErr }
func (f *fakeRemote) Write(_ context.Context, row int, e storage.Entry) error {
	f.writes = append(f.writes, row)
	if f.writeErr != nil {
		return f.writeErr
	}
	for len(f.rows) < row {
		f.rows = append(f.rows, nil)
	}
	f.rows[row-1] = []any{e.Time.Format(storage.DateFormat), e.Time.Format(storage.TimeFormat), e.Category, e.Minutes, float64(e.Minutes) / 60, e.Activity, e.ID}
	if f.uncertain {
		f.uncertain = false
		return errors.New("connection lost after Google accepted upload")
	}
	return nil
}
func savedWorkbook(t *testing.T) (*storage.Workbook, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tracker.xlsx")
	w := storage.NewWorkbook(path)
	for _, id := range []string{"first", "second"} {
		e := storage.Entry{ID: id, Time: time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC), Category: "Development", Minutes: 30, Activity: "Implemented API"}
		if err := w.Save(e); err != nil {
			t.Fatal(err)
		}
	}
	return w, path
}
func assertPending(t *testing.T, w *storage.Workbook, count int) []storage.PendingEntry {
	t.Helper()
	p, err := w.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != count {
		t.Fatalf("pending=%d, want %d", len(p), count)
	}
	return p
}
func TestOfflineUploadSurvivesRestart(t *testing.T) {
	w, path := savedWorkbook(t)
	c := Config{SpreadsheetID: "sheet-id", Tab: "Activities"}
	remote := &fakeRemote{rows: [][]any{headers}, writeErr: errors.New("offline")}
	if err := NewWorker(w, remote, c).Flush(context.Background()); err == nil {
		t.Fatal("expected offline error")
	}
	p := assertPending(t, w, 2)
	if p[0].Row != 2 {
		t.Fatalf("reserved row=%d", p[0].Row)
	}
	w = storage.NewWorkbook(path)
	remote.writeErr = nil
	if err := NewWorker(w, remote, c).Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPending(t, w, 0)
	if len(remote.rows) != 3 || len(remote.writes) != 3 || remote.writes[0] != 2 || remote.writes[1] != 2 || remote.writes[2] != 3 {
		t.Fatalf("rows=%v writes=%v", remote.rows, remote.writes)
	}
}
func TestAmbiguousUploadIsNotDuplicated(t *testing.T) {
	w, path := savedWorkbook(t)
	c := Config{SpreadsheetID: "sheet-id", Tab: "Activities"}
	remote := &fakeRemote{rows: [][]any{headers}, uncertain: true}
	if err := NewWorker(w, remote, c).Flush(context.Background()); err == nil {
		t.Fatal("expected lost response error")
	}
	assertPending(t, w, 2)
	w = storage.NewWorkbook(path)
	if err := NewWorker(w, remote, c).Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPending(t, w, 0)
	if len(remote.writes) != 2 || len(remote.rows) != 3 {
		t.Fatalf("duplicate writes: %v; rows=%v", remote.writes, remote.rows)
	}
}
func TestReservedRowConflictPreservesRemoteData(t *testing.T) {
	w, _ := savedWorkbook(t)
	c := Config{SpreadsheetID: "sheet-id", Tab: "Activities"}
	if err := w.Reserve("first", c.SpreadsheetID, c.Tab, 2); err != nil {
		t.Fatal(err)
	}
	remote := &fakeRemote{rows: [][]any{headers, {"existing activity"}}}
	if err := NewWorker(w, remote, c).Flush(context.Background()); err == nil {
		t.Fatal("expected conflict error")
	}
	assertPending(t, w, 2)
	if len(remote.writes) != 0 || remote.rows[1][0] != "existing activity" {
		t.Fatal("existing data changed")
	}
}
func TestChangingTargetKeepsReservedEntriesQueued(t *testing.T) {
	w, _ := savedWorkbook(t)
	if err := w.Reserve("first", "old-sheet", "Activities", 2); err != nil {
		t.Fatal(err)
	}
	remote := &fakeRemote{rows: [][]any{headers}}
	if err := NewWorker(w, remote, Config{SpreadsheetID: "new-sheet", Tab: "Activities"}).Flush(context.Background()); err == nil {
		t.Fatal("expected target mismatch")
	}
	if len(remote.writes) != 0 {
		t.Fatal("wrote reserved entry to a different destination")
	}
	assertPending(t, w, 2)
}
func TestCanceledSyncDoesNotLoseEntries(t *testing.T) {
	w, _ := savedWorkbook(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	remote := &fakeRemote{rows: [][]any{headers}}
	if err := NewWorker(w, remote, Config{SpreadsheetID: "sheet-id", Tab: "Activities"}).Flush(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	assertPending(t, w, 2)
}
