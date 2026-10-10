// Package nativeui hosts a small GTK3 window in the system Python runtime.
// The window starts on demand; it has no browser, HTTP server, or webview.
package nativeui

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"

	"metracker/internal/storage"
)

//go:embed panel.py
var script string

type State struct {
	Categories   []string             `json:"categories"`
	Entries      []storage.DailyEntry `json:"entries"`
	Pending      int                  `json:"pending"`
	Connected    bool                 `json:"connected"`
	Syncing      bool                 `json:"syncing"`
	SyncError    string               `json:"sync_error"`
	NextReminder int64                `json:"next_reminder"`
}

type Actions struct {
	State        func() (State, error)
	Save         func(storage.Entry) error
	Sync         func()
	Reminder     func()
	OpenWorkbook func()
	OpenFolder   func()
	Error        func(error)
}

type UI struct {
	actions Actions
	show    chan string
}

func New(actions Actions) *UI { return &UI{actions: actions, show: make(chan string, 1)} }
func (u *UI) Show(view string) {
	select {
	case u.show <- view:
	default:
	}
}

func (u *UI) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case view := <-u.show:
			if err := u.window(ctx, view); err != nil && ctx.Err() == nil {
				log.Printf("native window: %v", err)
				if u.actions.Error != nil {
					u.actions.Error(err)
				}
			}
		}
	}
}

type event struct {
	Action   string `json:"action"`
	Category string `json:"category"`
	Minutes  int    `json:"minutes"`
	Activity string `json:"activity"`
}

func (u *UI) window(ctx context.Context, view string) error {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(child, "/usr/bin/python3", "-u", "-c", script)
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start GTK interface: %w", err)
	}
	defer func() { input.Close(); cancel(); _ = cmd.Wait() }()
	lines := make(chan []byte)
	scanError := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 65536)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case lines <- line:
			case <-child.Done():
				return
			}
		}
		scanError <- scanner.Err()
		close(lines)
	}()
	encoder := json.NewEncoder(input)
	send := func(value any) error { return encoder.Encode(value) }
	update := func() error {
		state, err := u.actions.State()
		if err != nil {
			log.Printf("native window state: %v", err)
			return send(map[string]any{"type": "error", "message": "Unable to read your workbook. Check the tracker logs."})
		}
		return send(map[string]any{"type": "state", "state": state})
	}
	if err := update(); err != nil {
		return err
	}
	if err := send(map[string]string{"type": "show", "view": view}); err != nil {
		return err
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case view := <-u.show:
			if err := update(); err != nil {
				return err
			}
			if err := send(map[string]string{"type": "show", "view": view}); err != nil {
				return err
			}
		case <-ticker.C:
			if err := update(); err != nil {
				return err
			}
		case line, ok := <-lines:
			if !ok {
				err := <-scanError
				if err != nil {
					return err
				}
				return cmd.Wait()
			}
			var e event
			if err := json.Unmarshal(line, &e); err != nil {
				return errors.New("invalid GTK response")
			}
			if e.Action == "save" {
				entry := storage.Entry{Time: time.Now(), Category: e.Category, Minutes: e.Minutes, Activity: e.Activity}
				state, err := u.actions.State()
				if err == nil {
					err = validate(entry, state.Categories)
				}
				if err == nil {
					err = u.actions.Save(entry)
				}
				if err != nil {
					log.Printf("native save: %v", err)
					if err := send(map[string]any{"type": "error", "message": "Could not save the entry. Check the values and try again."}); err != nil {
						return err
					}
				} else {
					if err := send(map[string]any{"type": "saved", "message": "Saved to Excel. Google uploads run in the background when connected."}); err != nil {
						return err
					}
				}
			} else {
				switch e.Action {
				case "sync":
					if u.actions.Sync != nil {
						u.actions.Sync()
					}
				case "reminder":
					if u.actions.Reminder != nil {
						u.actions.Reminder()
					}
				case "workbook":
					if u.actions.OpenWorkbook != nil {
						u.actions.OpenWorkbook()
					}
				case "folder":
					if u.actions.OpenFolder != nil {
						u.actions.OpenFolder()
					}
				default:
					return errors.New("unknown GTK action")
				}
			}
			if err := update(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
				return err
			}
		}
	}
}

func validate(entry storage.Entry, categories []string) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	if utf8.RuneCountInString(entry.Activity) > 8000 {
		return errors.New("activity is too long")
	}
	for _, category := range categories {
		if entry.Category == category {
			return nil
		}
	}
	return errors.New("unknown category")
}
