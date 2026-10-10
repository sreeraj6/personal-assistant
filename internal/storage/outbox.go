package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/xuri/excelize/v2"
)

const outboxSheet = "_metracker_sync"

// PendingEntry lives in a hidden worksheet, committing the Excel entry and
// durable sync request in the same atomic workbook replacement.
type PendingEntry struct {
	Entry         Entry
	Row           int
	SpreadsheetID string
	Tab           string
}

func queueEntry(f *excelize.File, e Entry) error {
	idx, err := f.GetSheetIndex(outboxSheet)
	if err != nil {
		return err
	}
	if idx == -1 {
		if _, err := f.NewSheet(outboxSheet); err != nil {
			return err
		}
		headers := []interface{}{"Entry ID", "Entry JSON", "Remote row", "Spreadsheet", "Tab"}
		if err := f.SetSheetRow(outboxSheet, "A1", &headers); err != nil {
			return err
		}
		if err := f.SetSheetVisible(outboxSheet, false); err != nil {
			return err
		}
	}
	rows, err := f.GetRows(outboxSheet)
	if err != nil {
		return err
	}
	if len(rows) == 0 || len(rows[0]) < 2 || rows[0][0] != "Entry ID" || rows[0][1] != "Entry JSON" {
		return errors.New("sync queue header is invalid")
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	values := []interface{}{e.ID, string(data)}
	return f.SetSheetRow(outboxSheet, fmt.Sprintf("A%d", len(rows)+1), &values)
}

func (w *Workbook) Pending() ([]PendingEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := excelize.OpenFile(w.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	idx, err := f.GetSheetIndex(outboxSheet)
	if err != nil || idx == -1 {
		return nil, err
	}
	rows, err := f.GetRows(outboxSheet)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows[0]) < 2 || rows[0][0] != "Entry ID" || rows[0][1] != "Entry JSON" {
		return nil, errors.New("sync queue header is missing")
	}
	var pending []PendingEntry
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		if len(row) < 2 {
			return nil, errors.New("incomplete sync queue record")
		}
		var p PendingEntry
		if err := json.Unmarshal([]byte(row[1]), &p.Entry); err != nil {
			return nil, fmt.Errorf("read queued entry: %w", err)
		}
		if p.Entry.ID == "" || p.Entry.ID != row[0] {
			return nil, errors.New("invalid sync entry ID")
		}
		if err := p.Entry.Validate(); err != nil {
			return nil, err
		}
		if len(row) > 2 && row[2] != "" {
			p.Row, err = strconv.Atoi(row[2])
			if err != nil || p.Row < 2 {
				return nil, errors.New("invalid reserved remote row")
			}
			if len(row) < 5 || row[3] == "" || row[4] == "" {
				return nil, errors.New("incomplete sync destination")
			}
			p.SpreadsheetID, p.Tab = row[3], row[4]
		}
		pending = append(pending, p)
	}
	return pending, nil
}

func (w *Workbook) Reserve(id, spreadsheetID, tab string, row int) error {
	if row < 2 || spreadsheetID == "" || tab == "" {
		return errors.New("invalid sync reservation")
	}
	return w.updatePending(id, func(f *excelize.File, index int) error {
		values := []interface{}{row, spreadsheetID, tab}
		return f.SetSheetRow(outboxSheet, fmt.Sprintf("C%d", index), &values)
	})
}

func (w *Workbook) Acknowledge(id string) error {
	return w.updatePending(id, func(f *excelize.File, index int) error { return f.RemoveRow(outboxSheet, index) })
}

func (w *Workbook) updatePending(id string, update func(*excelize.File, int) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := excelize.OpenFile(w.path)
	if err != nil {
		return err
	}
	defer f.Close()
	rows, err := f.GetRows(outboxSheet)
	if err != nil {
		return err
	}
	for i, row := range rows {
		if i > 0 && len(row) > 0 && row[0] == id {
			if err := update(f, i+1); err != nil {
				return err
			}
			return atomicSave(f, w.path)
		}
	}
	return fmt.Errorf("queued entry %s not found", id)
}
