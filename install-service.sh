#!/bin/bash
# Install Me Tracker as a systemd user service that starts on graphical login.
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="$HOME/.metracker"
USER_SYSTEMD_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

# Check both builds before changing an existing installation.
for binary in metracker ironman-reminder; do
    if [[ ! -x "$APP_DIR/$binary" ]]; then
        echo "Missing executable $binary. Run 'make build' first." >&2
        exit 1
    fi
done
mkdir -p "$INSTALL_DIR" "$USER_SYSTEMD_DIR"

# Stop the previous executable before replacing it (avoids 'text file busy').
systemctl --user stop metracker.service 2>/dev/null || true
# Remove a legacy symlink before copying; this affects binaries only.
if [[ -L "$INSTALL_DIR/metracker" ]]; then
    rm "$INSTALL_DIR/metracker"
fi
install -m 0755 "$APP_DIR/metracker" "$INSTALL_DIR/metracker"
install -m 0755 "$APP_DIR/ironman-reminder" "$INSTALL_DIR/ironman-reminder"
install -m 0644 "$APP_DIR/metracker.service" "$USER_SYSTEMD_DIR/metracker.service"

# Import the actual session instead of assuming DISPLAY=:1 and UID=1000.
session_vars=()
for name in DISPLAY XAUTHORITY WAYLAND_DISPLAY DBUS_SESSION_BUS_ADDRESS; do
    if [[ -n "${!name:-}" ]]; then session_vars+=("$name"); fi
done
if (( ${#session_vars[@]} )); then
    systemctl --user import-environment "${session_vars[@]}"
fi
# The service owns autostart; remove the old duplicate desktop autostart entry.
rm -f "${XDG_CONFIG_HOME:-$HOME/.config}/autostart/metracker.desktop"
systemctl --user daemon-reload
systemctl --user enable metracker.service
systemctl --user restart metracker.service

echo "Me Tracker service installed and started in $INSTALL_DIR."
echo "Status: systemctl --user status metracker"
echo "Logs: journalctl --user -u metracker -f"
