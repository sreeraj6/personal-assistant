#!/bin/bash
# Install Me Tracker as a systemd user service that starts on login
set -e

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="$HOME/.metracker"
SERVICE_FILE="$APP_DIR/metracker.service"
USER_SYSTEMD_DIR="$HOME/.config/systemd/user"

mkdir -p "$INSTALL_DIR"
mkdir -p "$USER_SYSTEMD_DIR"

# Remove old symlink if present, then copy binaries to a path without spaces.
rm -f "$INSTALL_DIR/metracker"
cp "$APP_DIR/metracker" "$INSTALL_DIR/metracker"
cp "$APP_DIR/ironman-reminder" "$INSTALL_DIR/ironman-reminder"

cp "$SERVICE_FILE" "$USER_SYSTEMD_DIR/metracker.service"

# Reload, enable, start
systemctl --user daemon-reload
systemctl --user enable metracker.service
systemctl --user restart metracker.service

echo "Me Tracker service installed and started."
echo "  Install dir: $INSTALL_DIR"
echo ""
echo "Useful commands:"
echo "  systemctl --user status metracker   # check status"
echo "  systemctl --user stop metracker     # stop now"
echo "  systemctl --user start metracker    # start now"
echo "  systemctl --user disable metracker  # disable autostart"
echo ""
echo "Logs:"
echo "  journalctl --user -u metracker -f"
