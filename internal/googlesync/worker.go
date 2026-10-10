package googlesync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"metracker/internal/storage"
)

type destination interface {
	Read(context.Context) ([][]any, error)
	Write(context.Context, int, storage.Entry) error
}

type Worker struct {
	workbook *storage.Workbook
	remote   destination
	config   Config
	wake     chan struct{}
	mu       sync.Mutex
	statusMu sync.RWMutex
	status   Status
}

// Status is a nonblocking snapshot for the desktop panel.
type Status struct {
	Syncing bool
	Error   string
}

func (w *Worker) Status() Status {
	w.statusMu.RLock()
	defer w.statusMu.RUnlock()
	return w.status
}

func NewWorker(workbook *storage.Workbook, remote destination, c Config) *Worker {
	return &Worker{workbook: workbook, remote: remote, config: c, wake: make(chan struct{}, 1)}
}

func (w *Worker) Notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run retries in the background, never making Excel entry dialogs wait for Google.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	w.Notify()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.wake:
		}
		if err := w.Flush(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Google sync: %v", err)
		}
	}
}

func (w *Worker) Flush(ctx context.Context) (result error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.statusMu.Lock()
	w.status.Syncing = true
	w.statusMu.Unlock()
	defer func() {
		w.statusMu.Lock()
		defer w.statusMu.Unlock()
		w.status.Syncing = false
		w.status.Error = ""
		if result != nil && ctx.Err() == nil {
			w.status.Error = result.Error()
		}
	}()
	pending, err := w.workbook.Pending()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	rows, err := w.remote.Read(ctx)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p.Row > 0 && (p.SpreadsheetID != w.config.SpreadsheetID || p.Tab != w.config.Tab) {
			return errors.New("queued entries are reserved for another sheet; restore the original sync settings")
		}
		// A previous successful upload may not have been acknowledged locally.
		seen := false
		for _, row := range rows {
			if len(row) > 6 && row[6] == p.Entry.ID {
				seen = true
				break
			}
		}
		if !seen {
			if p.Row == 0 {
				p.Row = len(rows) + 1
				if err := w.workbook.Reserve(p.Entry.ID, w.config.SpreadsheetID, w.config.Tab, p.Row); err != nil {
					return err
				}
			}
			if p.Row <= len(rows) && !emptyRow(rows[p.Row-1]) {
				return fmt.Errorf("reserved Google row %d contains other data; leaving entry queued", p.Row)
			}
			if err := w.remote.Write(ctx, p.Row, p.Entry); err != nil {
				return err
			}
			for len(rows) < p.Row {
				rows = append(rows, nil)
			}
			rows[p.Row-1] = []any{nil, nil, nil, nil, nil, nil, p.Entry.ID}
		}
		if err := w.workbook.Acknowledge(p.Entry.ID); err != nil {
			return err
		}
	}
	return nil
}

func emptyRow(row []any) bool {
	for _, value := range row {
		if value != nil && value != "" {
			return false
		}
	}
	return true
}
