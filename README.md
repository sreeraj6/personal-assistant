# Me Tracker

A Go desktop activity tracker for Ubuntu. Log your category, minutes, and activity from the system tray. Entries are stored in an Excel workbook with one sheet per day. Iron Man reminds you every **30 minutes**; clicking him opens the native activity form.

## Current behavior

- **Open Me Tracker** opens a small native GTK3 window.
- **Log Activity...** opens its single form for category, duration, and activity.
- The panel shows today's entries, a live reminder countdown, and Google upload status.
- **Iron Man Reminder** triggers the overlay manually.
- **View Today** opens the native Today view. **Open Workbook** opens LibreOffice.
- **Open Folder** opens the tracker data directory.
- Ignored reminders hover until clicked. Only one reminder runs at a time.
- Quit cancels background timers, dialogs, and the reminder process.
- Only one tracker instance can run for the same user on Linux.
- Google Sheets sync is optional; Excel saves always work without a connection.
- **Sync Google Sheets Now** retries queued uploads after setup.

## Project structure

```text
main.go                       Application entry point
internal/
  app/                        Tray and background workers
  nativeui/                   Small GTK3 window and JSON-pipe controller
  config/                     User paths, category loading, embedded defaults
  storage/                    Entries, atomic Excel persistence, durable sync outbox
  googlesync/                 Google sign-in, Sheets API, background retries
  platform/                   Desktop opening and Linux instance lock
cmd/ironman/                  Reminder overlay and embedded sprite
Makefile                      Build, race tests, static checks, formatting
metracker.service             Graphical-session systemd user service
install-service.sh            Binary/service installation
install.desktop.sh            Optional Activities launcher
```

The root entry point is retained so `go build .` still works. Storage and configuration can be tested without launching the desktop UI. Google authorization uses the Go OAuth2 library.

## Build and verify

The module requires Go **1.26 or newer**; Go may select/download a newer toolchain to satisfy dependencies. Ubuntu build dependencies include GTK, Ayatana AppIndicator, OpenGL, and X11 development libraries. Runtime requires a graphical desktop session, `/usr/bin/python3`, `python3-gi`, `gir1.2-gtk-3.0`, `zenity`, `xdg-open`, and LibreOffice to open the workbook.

```bash
sudo apt install libgtk-3-dev libayatana-appindicator3-dev libgl1-mesa-dev xorg-dev zenity python3-gi gir1.2-gtk-3.0 libreoffice
make build
make check
```

`make check` runs the Go race detector and `go vet`. To check the actual GTK form using disposable sample data in a graphical session, run `/usr/bin/python3 tests/native_ui_smoke.py`. The Makefile keeps module and build caches in `.gopath/` and `.gocache/`, both ignored by Git.

Without Make:

```bash
export GOPATH="$PWD/.gopath"
export GOCACHE="$PWD/.gocache"
go build -o metracker .
go build -o ironman-reminder ./cmd/ironman
go test -race ./...
go vet ./...
```

Run `./metracker` from a graphical session. GNOME needs AppIndicator support to display the tray icon. Stop an installed service before running another copy for testing.

## Native Linux interface

A compact native GTK3 popup stays above other windows and opens at the top-right of the primary monitor’s usable desktop area, with a 16-pixel inset. It is about 380 pixels wide, with activity, category, minutes, and Save controls. A small **Today** toggle shows activity details and total logged time. **Ctrl+Enter** saves; **Esc** or **×** closes. The footer shows Google upload status and the reminder countdown. Workbook, folder, reminder, and manual sync actions remain available in the tray. Window placement depends on the desktop compositor; Wayland may control placement itself.

Open it from **Open Me Tracker**, **Log Activity...**, or **View Today** in the tray. Clicking Iron Man opens the form. The window is created on demand; reopening it focuses the existing window. Closing it exits its UI process while the tray, reminders, and background Google sync continue.

The form runs using Ubuntu's system Python and GTK packages. No browser, webview, frontend framework, or local dashboard server is used. The UI exchanges JSON through process pipes with the Go backend. No new Go UI dependencies are needed. **Open Workbook** explicitly uses LibreOffice; **Open Folder** uses your desktop file manager.

## Install

Run from your logged-in graphical session after building:

```bash
./install-service.sh
```

This copies both binaries to `~/.metracker/`, imports the current display/session environment, and enables a systemd user service for graphical login. It removes the legacy duplicate desktop autostart entry. The script does not modify the workbook or category settings.

Optionally install the Activities launcher:

```bash
./install.desktop.sh
```

The launcher points to this checkout. Systemd manages automatic startup.

```bash
systemctl --user status metracker
systemctl --user stop metracker
systemctl --user start metracker
systemctl --user disable metracker
journalctl --user -u metracker -f
```

## Data and configuration

All runtime data stays in `~/.metracker/`:

| File | Purpose |
| --- | --- |
| `me_tracker.xlsx` | Existing workbook, one `YYYY-MM-DD` sheet per day |
| `categories.json` | User-editable category list |
| `add-entry.trigger` | Request sent by clicking the reminder |
| `tracker.lock` | Kernel-backed instance lock; do not delete while running |

Default categories are embedded from `internal/config/categories.json` and written only when the user's category file is missing. Edit `~/.metracker/categories.json` and restart to change the list. Invalid, empty, or unreadable settings produce a startup error in the logs and are preserved for correction.

The reminder interval is defined by `config.ReminderInterval` in `internal/config/config.go`. It remains 30 minutes, measured from application startup. Changing it currently requires rebuilding and reinstalling.

## Excel format

| Time | Category | Minutes | Hours | Activity |
| --- | --- | --- | --- | --- |
| 14:32:10 | Development | 45 | 0.75 | Built login API |
| 15:10:05 | Meetings | 30 | 0.50 | Daily standup |

The timestamp and sheet date come from a single local-time timestamp. Existing entries and sheet names remain compatible. Missing headers from the old first-sheet behavior are restored when that sheet receives a new entry.

Saves are serialized, written to a temporary file beside the workbook, synced, and then renamed over the destination. Failed saves before replacement leave the previous workbook intact. A damaged existing workbook produces an error instead of being overwritten. Temporary files are removed after handled failures; an abrupt process kill may leave a `.workbook-*.xlsx` file.

Avoid saving external edits to the workbook while the tracker is writing: the tracker instance lock does not coordinate with Excel or LibreOffice. Use Excel pivot tables to analyze category totals on a daily sheet; combine daily sheets when comparing dates, since the date is currently the sheet name rather than a column.

## Connect your personal Google Sheet

1. Create a Google Cloud project and enable the **Google Sheets API**.
2. Configure Google Auth Platform for a personal account with **External** audience. While testing, add your Google email as a test user.
3. Create an OAuth client with application type **Desktop app**, then download its JSON file. Keep it on your computer; no Google password is needed.
4. Build and stop the running tracker before setup:

```bash
make build
systemctl --user stop metracker
./metracker --setup-google \
  --sheet "https://docs.google.com/spreadsheets/d/YOUR_SPREADSHEET_ID/edit" \
  --credentials "/path/to/downloaded-client.json" \
  --tab "Activities"
```

Setup prints a Google authorization URL for you to open manually; it does not launch a browser. The app uses a temporary localhost callback, a random authorization state, and PKCE. Grant Sheets access using the account that can edit the destination. The requested `spreadsheets` scope permits spreadsheet access across that account; the app's sync code writes only to the configured spreadsheet and tab.

Setup creates the Activities tab if missing, adds headers to an empty tab, and verifies existing headers before enabling sync. Use a dedicated tab with these columns:

| Date | Time | Category | Minutes | Hours | Activity | Entry ID |
| --- | --- | --- | --- | --- | --- | --- |
| 2026-10-10 | 14:32:10 | Development | 45 | 0.75 | Built login API | Generated stable ID |

Restart the service after successful setup:

```bash
./install-service.sh
journalctl --user -u metracker -f
```

Setup stores `google-sync.json`, `google-credentials.json`, and `google-token.json` in `~/.metracker/`. Credentials and tokens are saved with owner-only permissions and must not be committed or shared. Google authorization may need renewing if revoked or expired. In an External OAuth app left in Testing, refresh tokens for this scope expire after seven days ([Google token expiry rules](https://developers.google.com/identity/protocols/oauth2#expiration)); rerun setup, or configure the consent app for appropriate ongoing use.

### Sync behavior and recovery

- Every **new entry saved by this version** is committed together with a durable upload request in the hidden `_metracker_sync` worksheet. Existing historical entries are not automatically imported.
- Google setup is optional. Entries logged before setup remain queued and sync once configured. A legacy workbook without a queue has no pending uploads.
- Uploads run at startup, after each save, on **Sync Google Sheets Now**, and every minute when entries remain queued. Internet failures never block the native entry form.
- Each entry receives a stable ID. Before upload, its destination and remote row are reserved in the workbook. Retries update that same row using RAW values, preventing a dropped response from causing a second append and keeping activity text from being interpreted as a formula.
- Entries leave the queue only after upload succeeds or their ID is found in the destination. Sync errors are recorded in the service logs; the native window reports local saves separately from cloud status.
- Keep the tab dedicated to this tracker. Do not delete/change Entry IDs, insert/delete/sort rows while sync is pending, or add other writers. Occupied reserved rows cause a safe error, leaving entries queued. Spreadsheet applications are not coordinated by the tracker lock.
- Do not edit/delete the hidden `_metracker_sync` sheet. The visible daily sheets retain their original five-column format.
- Editing existing Excel entries does not update their Google rows: this phase uploads newly logged entries, rather than providing two-way synchronization.
- Entries already reserved for a destination cannot silently move to a different sheet. Restore the original settings to finish those uploads before changing destinations.
- To pause cloud uploads, stop the tracker, rename `~/.metracker/google-sync.json`, and restart. Local entries continue to be queued; restoring the settings resumes uploads.

Google documentation: [Desktop OAuth](https://developers.google.com/identity/protocols/oauth2/native-app), [Sheets setup](https://developers.google.com/workspace/sheets/api/quickstart/go), and [cell updates](https://developers.google.com/workspace/sheets/api/reference/rest/v4/spreadsheets.values/update).

## Possible next phase (not implemented)

- Working hours and quiet hours for reminders.
- Snooze/pause controls for focus sessions and meetings.
- Daily and weekly summaries of time by category.
- Correct or delete previous entries.
- A database as the source of truth with Excel exports as the assistant grows.

## Uninstall

Stop the service and remove program integration:

```bash
systemctl --user disable --now metracker
rm -f "${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/metracker.service"
rm -f "$HOME/.local/share/applications/metracker.desktop"
rm -f "${XDG_CONFIG_HOME:-$HOME/.config}/autostart/metracker.desktop"
systemctl --user daemon-reload
```

Keep `~/.metracker/` to preserve your tracked work. Delete that directory only if you intentionally want to erase the workbook and settings.
