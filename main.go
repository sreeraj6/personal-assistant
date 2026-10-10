package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/systray"
	"github.com/ncruces/zenity"
	"github.com/xuri/excelize/v2"
)

var (
	workbookPath   string
	categoriesPath string
	categories     []string
	timeFormat     = "15:04:05"
	dateFormat     = "2006-01-02"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dir := filepath.Join(home, ".metracker")
	if err := os.MkdirAll(dir, 0755); err != nil {
		showError("Failed to create tracker directory: " + err.Error())
		return
	}
	workbookPath = filepath.Join(dir, "me_tracker.xlsx")
	categoriesPath = filepath.Join(dir, "categories.json")

	loadCategories()

	systray.Run(onReady, onExit)
}

func loadCategories() {
	data, err := os.ReadFile(categoriesPath)
	if err != nil {
		// Use defaults and write them out
		categories = defaultCategories()
		_ = saveCategories()
		return
	}
	if err := json.Unmarshal(data, &categories); err != nil || len(categories) == 0 {
		categories = defaultCategories()
		_ = saveCategories()
	}
}

func saveCategories() error {
	data, err := json.MarshalIndent(categories, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(categoriesPath, append(data, '\n'), 0644)
}

func defaultCategories() []string {
	return []string{
		"Development",
		"Bug Fixing",
		"Code Review",
		"Meetings",
		"Learning",
		"Documentation",
		"Design / Architecture",
		"DevOps / Infra",
		"Testing / QA",
		"Management",
		"Other",
	}
}

func onReady() {
	data, _ := base64.StdEncoding.DecodeString(IconBase64)
	systray.SetIcon(data)
	// systray.SetTitle("Me")
	systray.SetTooltip("Me Tracker - click to log your day")

	mAdd := systray.AddMenuItem("Add Entry...", "Log what you just did")
	mTest := systray.AddMenuItem("Iron Man Reminder", "Trigger the Iron Man fly-by now")
	mView := systray.AddMenuItem("View Today", "Open today's sheet")
	mOpen := systray.AddMenuItem("Open Workbook", "Open the Excel file")
	mFolder := systray.AddMenuItem("Open Folder", "Open tracker folder")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit Me Tracker")

	go reminderLoop()
	go watchTrigger()

	go func() {
		for {
			select {
			case <-mAdd.ClickedCh:
				if err := promptAndSave(); err != nil {
					showError("Failed to save: " + err.Error())
				}
			case <-mTest.ClickedCh:
				go triggerReminder()
			case <-mView.ClickedCh:
				openFile(workbookPath)
			case <-mOpen.ClickedCh:
				openFile(workbookPath)
			case <-mFolder.ClickedCh:
				openFile(filepath.Dir(workbookPath))
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func watchTrigger() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	trigger := filepath.Join(home, ".metracker", "add-entry.trigger")
	for {
		time.Sleep(500 * time.Millisecond)
		if _, err := os.Stat(trigger); err != nil {
			continue
		}
		_ = os.Remove(trigger)
		if err := promptAndSave(); err != nil {
			showError("Failed to save: " + err.Error())
		}
	}
}

func reminderLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		<-ticker.C
		go triggerReminder()
	}
}

func triggerReminder() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exe)
	reminder := filepath.Join(dir, "ironman-reminder")
	cmd := exec.Command(reminder)
	cmd.Env = os.Environ()
	_ = cmd.Start()
}

func onExit() {}

func promptAndSave() error {
	// 1. Category
	category, err := zenity.ListItems(
		"Select the category for this work:",
		categories...,
	)
	if err == zenity.ErrCanceled {
		return nil
	}
	if err != nil {
		return fmt.Errorf("category dialog: %w", err)
	}
	if category == "" {
		return nil
	}

	// 2. Duration
	durationStr, err := zenity.Entry(
		"How many minutes did you spend?",
		zenity.Title("Me Tracker"),
		zenity.EntryText("30"),
	)
	if err == zenity.ErrCanceled {
		return nil
	}
	if err != nil {
		return fmt.Errorf("duration dialog: %w", err)
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(durationStr))
	if err != nil || minutes <= 0 {
		showError("Please enter a valid number of minutes.")
		return nil
	}

	// 3. Activity
	activity, err := zenity.Entry(
		"What did you do?",
		zenity.Title("Me Tracker"),
	)
	if err == zenity.ErrCanceled {
		return nil
	}
	if err != nil {
		return fmt.Errorf("activity dialog: %w", err)
	}
	activity = strings.TrimSpace(activity)
	if activity == "" {
		return nil
	}

	if err := saveEntry(category, minutes, activity); err != nil {
		return err
	}

	hours := float64(minutes) / 60.0
	msg := fmt.Sprintf("Saved!\nCategory: %s\nDuration: %d min (%.2f h)\nTime: %s",
		category, minutes, hours, time.Now().Format(timeFormat))
	_ = zenity.Info(msg, zenity.Title("Me Tracker"))
	return nil
}

func showError(msg string) {
	_ = zenity.Error(msg, zenity.Title("Me Tracker"))
}

func saveEntry(category string, minutes int, activity string) error {
	var f *excelize.File
	if _, err := os.Stat(workbookPath); os.IsNotExist(err) {
		f = excelize.NewFile()
		defaultSheet := f.GetSheetName(0)
		_ = f.SetSheetName(defaultSheet, todaySheet())
	} else {
		f, err = excelize.OpenFile(workbookPath)
		if err != nil {
			return fmt.Errorf("open workbook: %w", err)
		}
	}
	defer f.Close()

	sheet := todaySheet()
	idx, err := f.GetSheetIndex(sheet)
	if err != nil || idx == -1 {
		_, _ = f.NewSheet(sheet)
		_ = f.SetCellValue(sheet, "A1", "Time")
		_ = f.SetCellValue(sheet, "B1", "Category")
		_ = f.SetCellValue(sheet, "C1", "Minutes")
		_ = f.SetCellValue(sheet, "D1", "Hours")
		_ = f.SetCellValue(sheet, "E1", "Activity")
		style, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"2980B9"}, Pattern: 1},
		})
		_ = f.SetCellStyle(sheet, "A1", "E1", style)
		_ = f.SetColWidth(sheet, "A", "A", 14)
		_ = f.SetColWidth(sheet, "B", "B", 24)
		_ = f.SetColWidth(sheet, "C", "C", 11)
		_ = f.SetColWidth(sheet, "D", "D", 11)
		_ = f.SetColWidth(sheet, "E", "E", 60)
	}

	rows, _ := f.GetRows(sheet)
	row := len(rows) + 1
	if row < 2 {
		row = 2
	}

	hours := float64(minutes) / 60.0

	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), time.Now().Format(timeFormat))
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), category)
	_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), minutes)
	_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), hours)
	_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), activity)

	return f.SaveAs(workbookPath)
}

func todaySheet() string {
	return time.Now().Format(dateFormat)
}

func openFile(path string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{path}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", "\"\"", path}
	default:
		cmd = "xdg-open"
		args = []string{path}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		showError("Failed to open: " + err.Error())
	}
}
