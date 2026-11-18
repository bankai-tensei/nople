#!/bin/sh
# Takes out what install.sh put in; pass --system if it was installed so.
# The settings and the session in ~/.config/Nople stay.
set -eu

if [ "${1:-}" = "--system" ]; then
	prefix=/usr/local
	data=$prefix/share
else
	prefix=$HOME/.local
	data=${XDG_DATA_HOME:-$HOME/.local/share}
fi

rm -f "$prefix/bin/nople" \
	"$data/applications/nople.desktop" \
	"$data/icons/hicolor/256x256/apps/nople.png" \
	"$data/icons/hicolor/scalable/apps/nople.svg"

command -v update-desktop-database >/dev/null && update-desktop-database -q "$data/applications" || true
command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -qtf "$data/icons/hicolor" 2>/dev/null || true

echo "Nople is uninstalled."
