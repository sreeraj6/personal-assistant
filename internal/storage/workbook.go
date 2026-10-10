// Package storage persists activities in the existing daily-sheet Excel format.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
)

const DateFormat = "2006-01-02"
const TimeFormat = "15:04:05"

type Entry struct {
	ID       string `json:"id"`
	Time     time.Time
	Category string
	Minutes  int
	Activity string
}

func (e Entry) Validate() error {
	if e.Time.IsZero() {
		return errors.New("entry time is required")
	}
	if strings.TrimSpace(e.Category) == "" {
		return errors.New("category is required")
	}
	if e.Minutes <= 0 {
		return errors.New("minutes must be positive")
	}
	if strings.TrimSpace(e.Activity) == "" {
		return errors.New("activity is required")
	}
	return nil
}

// Workbook serializes updates. The desktop app also holds a process lock to
// prevent two tracker instances from overwriting one another's entries.
type Workbook struct {
	path string
	mu   sync.Mutex
}

func NewWorkbook(path string) *Workbook { return &Workbook{path: path} }

func (w *Workbook) Save(e Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.ID == "" {
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			return fmt.Errorf("generate entry ID: %w", err)
		}
		e.ID = hex.EncodeToString(id)
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	var f *excelize.File
	_, err := os.Stat(w.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		f = excelize.NewFile()
		if err := f.SetSheetName(f.GetSheetName(0), e.Time.Format(DateFormat)); err != nil {
			f.Close()
			return err
		}
	case err != nil:
		return fmt.Errorf("stat workbook: %w", err)
	default:
		f, err = excelize.OpenFile(w.path)
		if err != nil {
			return fmt.Errorf("open workbook: %w", err)
		}
	}
	defer f.Close()

	sheet := e.Time.Format(DateFormat)
	idx, err := f.GetSheetIndex(sheet)
	if err != nil {
		return err
	}
	if idx == -1 {
		if _, err := f.NewSheet(sheet); err != nil {
			return err
		}
	}
	// Always establish headers: this also repairs sheets created by the old app.
	if err := formatSheet(f, sheet); err != nil {
		return fmt.Errorf("format sheet: %w", err)
	}
	// A row iterator avoids materializing every existing cell just to count rows.
	rows, err := f.Rows(sheet)
	if err != nil {
		return err
	}
	row := 1
	for rows.Next() {
		row++
	}
	iterErr := rows.Error()
	closeErr := rows.Close()
	if iterErr != nil {
		return iterErr
	}
	if closeErr != nil {
		return closeErr
	}
	if row < 2 {
		row = 2
	}
	values := []interface{}{e.Time.Format(TimeFormat), strings.TrimSpace(e.Category), e.Minutes, float64(e.Minutes) / 60, strings.TrimSpace(e.Activity)}
	if err := f.SetSheetRow(sheet, fmt.Sprintf("A%d", row), &values); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	if err := queueEntry(f, e); err != nil {
		return fmt.Errorf("queue sync entry: %w", err)
	}
	return atomicSave(f, w.path)
}

func formatSheet(f *excelize.File, sheet string) error {
	headers := []interface{}{"Time", "Category", "Minutes", "Hours", "Activity"}
	if err := f.SetSheetRow(sheet, "A1", &headers); err != nil {
		return err
	}
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"2980B9"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(sheet, "A1", "E1", style); err != nil {
		return err
	}
	for _, col := range []struct {
		name  string
		width float64
	}{{"A", 14}, {"B", 24}, {"C", 11}, {"D", 11}, {"E", 60}} {
		if err := f.SetColWidth(sheet, col.name, col.name, col.width); err != nil {
			return err
		}
	}
	return nil
}

// Write beside the destination and rename only after a complete, synced write.
// A failed save leaves the previous workbook intact.
func atomicSave(f *excelize.File, path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".workbook-*.xlsx")
	if err != nil {
		return fmt.Errorf("create temporary workbook: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := f.Write(tmp); err != nil {
		tmp.Close()
		return fmt.Errorf("write workbook: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync workbook: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace workbook: %w", err)
	}
	return nil
}
