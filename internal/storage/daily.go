package storage

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/xuri/excelize/v2"
)

type DailyEntry struct {
	Time     string `json:"time"`
	Category string `json:"category"`
	Minutes  int    `json:"minutes"`
	Activity string `json:"activity"`
}

// Daily returns the visible daily log without exposing the hidden upload queue.
func (w *Workbook) Daily(day time.Time) ([]DailyEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := excelize.OpenFile(w.path)
	if errors.Is(err, os.ErrNotExist) {
		return []DailyEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet := day.Format(DateFormat)
	idx, err := f.GetSheetIndex(sheet)
	if err != nil {
		return nil, err
	}
	if idx == -1 {
		return []DailyEntry{}, nil
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	entries := []DailyEntry{}
	for i, row := range rows {
		if len(row) == 0 || i == 0 && row[0] == "Time" {
			continue
		}
		if len(row) < 5 {
			return nil, fmt.Errorf("daily row %d is incomplete", i+1)
		}
		minutes, err := strconv.Atoi(row[2])
		if err != nil {
			return nil, fmt.Errorf("daily row %d has invalid minutes", i+1)
		}
		entries = append(entries, DailyEntry{Time: row[0], Category: row[1], Minutes: minutes, Activity: row[4]})
	}
	return entries, nil
}
