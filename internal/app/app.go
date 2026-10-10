// Package app coordinates the tray, dialogs, and reminder lifecycle.
package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"metracker/internal/config"
	"metracker/internal/googlesync"
	"metracker/internal/nativeui"
	"metracker/internal/platform"
	"metracker/internal/storage"

	"github.com/getlantern/systray"
	"github.com/ncruces/zenity"
)

type application struct {
	ctx            context.Context
	cancel         context.CancelFunc
	paths          config.Paths
	categories     []string
	workbook       *storage.Workbook
	requests       chan struct{}
	reminder       chan struct{}
	opens          chan string
	wg             sync.WaitGroup
	reminderActive atomic.Bool
	syncer         *googlesync.Worker
	ui             *nativeui.UI
	syncProblem    string
	nextReminder   atomic.Int64
}

func Run() error {
	paths, err := config.UserPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.Directory, 0700); err != nil {
		return fmt.Errorf("create tracker directory: %w", err)
	}
	lock, err := platform.LockInstance(filepath.Join(paths.Directory, "tracker.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	categories, err := config.LoadCategories(paths.Categories)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &application{
		ctx: ctx, cancel: cancel,
		paths: paths, categories: categories,
		workbook: storage.NewWorkbook(paths.Workbook),
		requests: make(chan struct{}, 1),
		reminder: make(chan struct{}, 1),
		opens:    make(chan string, 1),
	}
	// Google setup failures never prevent local Excel tracking.
	if settings, err := googlesync.LoadConfig(paths.Directory); err != nil {
		a.syncProblem = err.Error()
		log.Printf("Google sync disabled: %v", err)
	} else if settings != nil {
		if client, err := googlesync.HTTPClient(ctx, paths.Directory); err != nil {
			a.syncProblem = err.Error()
			log.Printf("Google sync disabled: %v", err)
		} else {
			a.syncer = googlesync.NewWorker(a.workbook, googlesync.NewRemote(client, *settings), *settings)
		}
	}

	a.nextReminder.Store(time.Now().Add(config.ReminderInterval).Unix())
	a.ui = nativeui.New(nativeui.Actions{
		State: func() (nativeui.State, error) {
			entries, err := a.workbook.Daily(time.Now())
			if err != nil {
				return nativeui.State{}, err
			}
			pending, err := a.workbook.Pending()
			if err != nil {
				return nativeui.State{}, err
			}
			state := nativeui.State{Categories: a.categories, Entries: entries, Pending: len(pending), SyncError: a.syncProblem, NextReminder: a.nextReminder.Load()}
			if a.syncer != nil {
				cloud := a.syncer.Status()
				state.Connected = true
				state.Syncing = cloud.Syncing
				state.SyncError = cloud.Error
			}
			return state, nil
		},
		Save:         a.saveEntry,
		OpenWorkbook: func() { a.requestOpen(a.paths.Workbook) },
		OpenFolder:   func() { a.requestOpen(a.paths.Directory) },
		Reminder: func() {
			select {
			case a.reminder <- struct{}{}:
			default:
			}
		},
		Sync: func() {
			if a.syncer != nil {
				a.syncer.Notify()
			}
		},
		Error: func(err error) {
			a.showError("Could not open the native window. Ensure python3-gi and gir1.2-gtk-3.0 are installed.")
		},
	})
	systray.Run(a.onReady, a.onExit)
	return nil
}

func (a *application) onReady() {
	data, err := base64.StdEncoding.DecodeString(IconBase64)
	if err != nil {
		log.Printf("decode tray icon: %v", err)
	} else {
		systray.SetIcon(data)
	}
	systray.SetTooltip("Me Tracker - click to log your day")
	panel := systray.AddMenuItem("Open Me Tracker", "Open the native activity window")
	add := systray.AddMenuItem("Log Activity...", "Log what you just did")
	test := systray.AddMenuItem("Iron Man Reminder", "Trigger the Iron Man fly-by now")
	view := systray.AddMenuItem("View Today", "View today's activity in the native window")
	open := systray.AddMenuItem("Open Workbook", "Open the Excel file")
	folder := systray.AddMenuItem("Open Folder", "Open tracker folder")
	syncNow := systray.AddMenuItem("Sync Google Sheets Now", "Retry queued uploads")
	if a.syncer == nil {
		syncNow.Disable()
	} else {
		a.start(func() { a.syncer.Run(a.ctx) })
	}
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Exit Me Tracker")
	a.start(func() { a.ui.Run(a.ctx) })
	a.start(a.watchTrigger)
	a.start(a.reminderLoop)
	a.start(func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case path := <-a.opens:
				a.open(path)
			}
		}
	})
	a.start(func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-a.requests:
				a.ui.Show("entry")
			}
		}
	})
	a.start(func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-panel.ClickedCh:
				a.ui.Show("entry")
			case <-add.ClickedCh:
				select {
				case a.requests <- struct{}{}:
				default:
				}
			case <-test.ClickedCh:
				select {
				case a.reminder <- struct{}{}:
				default:
				}
			case <-view.ClickedCh:
				a.ui.Show("today")
			case <-open.ClickedCh:
				a.requestOpen(a.paths.Workbook)
			case <-folder.ClickedCh:
				a.requestOpen(a.paths.Directory)
			case <-syncNow.ClickedCh:
				if a.syncer != nil {
					a.syncer.Notify()
				}
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	})
}

func (a *application) start(fn func()) {
	a.wg.Add(1)
	go func() { defer a.wg.Done(); fn() }()
}

func (a *application) onExit() {
	a.cancel()
	a.wg.Wait()
}

func (a *application) watchTrigger() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			// Removal both consumes the request and avoids a separate stat call.
			if err := os.Remove(a.paths.Trigger); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					log.Printf("consume entry trigger: %v", err)
				}
				continue
			}
			select {
			case a.requests <- struct{}{}:
			default:
			}
		}
	}
}

func (a *application) reminderLoop() {
	ticker := time.NewTicker(config.ReminderInterval)
	defer ticker.Stop()
	var children sync.WaitGroup
	defer children.Wait()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.nextReminder.Store(time.Now().Add(config.ReminderInterval).Unix())
		case <-a.reminder:
		}
		// An ignored reminder keeps hovering; never stack another overlay over it.
		if !a.reminderActive.CompareAndSwap(false, true) {
			continue
		}
		children.Add(1)
		go func() {
			defer children.Done()
			defer a.reminderActive.Store(false)
			exe, err := os.Executable()
			if err != nil {
				log.Printf("locate reminder: %v", err)
				return
			}
			cmd := exec.CommandContext(a.ctx, filepath.Join(filepath.Dir(exe), "ironman-reminder"))
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			// Run waits for the process, preventing zombies, and context cancels it on exit.
			if err := cmd.Run(); err != nil && a.ctx.Err() == nil {
				log.Printf("run reminder: %v", err)
			}
		}()
	}
}

func (a *application) showError(msg string) {
	log.Print(msg)
	if err := zenity.Error(msg, zenity.Title("Me Tracker"), zenity.Context(a.ctx)); err != nil && a.ctx.Err() == nil {
		log.Printf("show error dialog: %v", err)
	}
}

func (a *application) open(path string) {
	if err := platform.Open(a.ctx, path); err != nil && a.ctx.Err() == nil {
		a.showError("Failed to open: " + err.Error())
	}
}

func (a *application) requestOpen(path string) {
	select {
	case a.opens <- path:
	default:
	}
}

func (a *application) saveEntry(entry storage.Entry) error {
	if err := a.workbook.Save(entry); err != nil {
		return err
	}
	if a.syncer != nil {
		a.syncer.Notify()
	}
	return nil
}
