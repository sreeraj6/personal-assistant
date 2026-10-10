#!/bin/bash
# Install the Activities launcher; systemd owns autostart.
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
EXEC_PATH="$APP_DIR/metracker"
ICON_PATH="$APP_DIR/icon.png"

mkdir -p "$HOME/.local/share/applications"

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


echo "Me Tracker installed."
echo "  Launcher: $HOME/.local/share/applications/metracker.desktop"
echo ""
echo "You can now:"
echo "  - Search 'Me Tracker' in Activities to launch it"
echo "  - Run ./install-service.sh to enable autostart"
