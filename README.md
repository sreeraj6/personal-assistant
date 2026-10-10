# Me Tracker

A tiny Go system-tray app for Ubuntu to quickly log what you do each day. Each day gets its own sheet in an Excel workbook, structured so you can analyze how much you work and which roles/categories you focus on.

And yes — **Iron Man** flies across your screen every 30 minutes to remind you. 🦾

## Features

- Lives in the system tray (top bar) for one-click access
- Click **Add Entry...** → pick a category, enter minutes spent, describe the activity
- Saves current time, category, duration, and activity
- Creates/updates `~/.metracker/me_tracker.xlsx` with one sheet per day (`YYYY-MM-DD`)
- **Iron Man reminder** pops up every 30 minutes with a funny message
- Click the flying Iron Man and he zooms up and away
- View today's sheet, open the workbook, or open the tracker folder from the tray menu
- Starts automatically on login as a systemd user service

## Files

| File | Purpose |
|------|---------|
| `metracker` | Main app binary |
| `ironman-reminder` | Iron Man fly-by reminder binary |
| `main.go` | Main app source code |
| `cmd/ironman/` | Iron Man reminder source + sprite |
| `icon.go` | Tray icon data |
| `icon.png` | App icon for the desktop launcher |
| `metracker.service` | systemd user service template |
| `install-service.sh` | Installs/enables the systemd service |
| `install.desktop.sh` | Installs the Activities launcher + autostart |
| `categories.json` | Default category list (copied to `~/.metracker/` on first run) |
| `go.mod`, `go.sum` | Go module files |
| `.gopath/`, `.gocache/` | Local Go module/cache directories |

## Build

```bash
cd "/media/sreeraj/UbuntuData/me tracker"
export GOPATH="$PWD/.gopath"
export GOCACHE="$PWD/.gocache"
go build -o metracker .
go build -o ironman-reminder ./cmd/ironman
```

## Run once (for testing)

```bash
./metracker
```

You will see a blue **"Me"** icon in the system tray.

## Install as a service (recommended)

This makes Me Tracker start automatically every time you log in, and restart if it crashes. It also installs the Iron Man reminder binary.

```bash
./install-service.sh
```

Check that it's running:

```bash
systemctl --user status metracker
```

Common service commands:

```bash
systemctl --user stop metracker      # stop now
systemctl --user start metracker     # start now
systemctl --user disable metracker   # disable autostart
journalctl --user -u metracker -f    # watch logs
```

## Install Activities launcher + autostart (optional)

If you also want to launch it from Activities:

```bash
./install.desktop.sh
```

## How to add an entry

1. Click the tray icon
2. Choose **Add Entry...**
3. Select a **category** (e.g. Development, Meetings, Learning)
4. Enter **minutes spent**
5. Describe what you did

That's it — the entry is saved with the current time.

## Iron Man reminder

Every 30 minutes, a small transparent Iron Man window flies diagonally across your screen with a message like:

> "Hey genius, where's your update?"

- Click anywhere on him → he flies straight up and disappears
- Ignore him → he flies off on his own after ~8 seconds
- Want to test it? Click **Iron Man Reminder** in the tray menu

You can change the interval by editing the ticker in `main.go`:

```go
ticker := time.NewTicker(30 * time.Minute)
```

## Excel format (built for analysis)

Workbook: `~/.metracker/me_tracker.xlsx`

Each sheet is named by date. Every entry has these columns:

| Time     | Category     | Minutes | Hours | Activity              |
|----------|--------------|---------|-------|-----------------------|
| 14:32:10 | Development  | 45      | 0.75  | Built login API       |
| 15:10:05 | Meetings     | 30      | 0.50  | Daily standup         |

### Why these columns?

- **Category** — lets you group work by role/type (Development, Meetings, Learning, etc.)
- **Minutes** — easy raw duration
- **Hours** — pre-calculated for charts and pivot tables

You can edit categories in `~/.metracker/categories.json` and restart the app.

## Analysis ideas in Excel

Use Excel's built-in features on your workbook:

1. **Total hours per day**
   - Select any sheet → `Insert → PivotTable`
   - Rows: `Date` (sheet name), Values: `Sum of Hours`

2. **Time spent per category**
   - Combine all sheets, or analyze per sheet
   - PivotTable: Rows = `Category`, Values = `Sum of Hours`

3. **Which role to focus on?**
   - Over a week/month, compare category totals
   - High `Meetings` vs low `Development` might mean you should protect deep-work time
   - High `Learning` could show you're growing toward a new skill/role

4. **Weekly/monthly trends**
   - Copy data from all sheets into one summary sheet
   - Use `SUMIFS` or PivotTables by date range

## Dependencies

Build-time:

- Go 1.21+ (module uses Go 1.25 toolchain automatically)
- `libgtk-3-dev`
- `libayatana-appindicator3-dev`

Runtime:

- GTK+3
- Ayatana AppIndicator
- `zenity` (for dialogs — usually pre-installed on Ubuntu)
- OpenGL / X11 (for the Iron Man Ebiten overlay)

Install build deps with:

```bash
sudo apt update
sudo apt install -y libgtk-3-dev libayatana-appindicator3-dev
```

## Uninstall

```bash
systemctl --user stop metracker
systemctl --user disable metracker
rm -f ~/.config/systemd/user/metracker.service
rm -f ~/.local/share/applications/metracker.desktop
rm -f ~/.config/autostart/metracker.desktop
rm -rf ~/.metracker
systemctl --user daemon-reload
```
