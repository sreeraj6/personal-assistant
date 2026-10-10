#!/bin/bash
# Install Me Tracker desktop entry and autostart entry
set -e

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
EXEC_PATH="$APP_DIR/metracker"
ICON_PATH="$APP_DIR/icon.png"

mkdir -p "$HOME/.local/share/applications"
mkdir -p "$HOME/.config/autostart"

cat > "$HOME/.local/share/applications/metracker.desktop" <<EOF
[Desktop Entry]
Name=Me Tracker
Comment=Quick daily activity tracker
Exec="$EXEC_PATH"
Icon=$ICON_PATH
Type=Application
Terminal=false
Categories=Utility;Office;
StartupNotify=false
EOF

cp "$HOME/.local/share/applications/metracker.desktop" "$HOME/.config/autostart/metracker.desktop"

echo "Me Tracker installed."
echo "  Launcher: $HOME/.local/share/applications/metracker.desktop"
echo "  Autostart: $HOME/.config/autostart/metracker.desktop"
echo ""
echo "You can now:"
echo "  - Search 'Me Tracker' in Activities to launch it"
echo "  - It will start automatically on next login"
