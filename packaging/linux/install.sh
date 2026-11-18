#!/bin/sh
# Installs Nople from this folder, the one the tarball unpacks to:
#
#   ./install.sh            for the current user, into ~/.local
#   sudo ./install.sh --system   for all users, into /usr/local
#
# It puts nople into bin, nople.desktop among the applications, so that the
# file manager offers "Open with Nople" for folders and text files, and the
# icon into the hicolor theme. ./uninstall.sh (with the same flag) takes them
# out again.
set -eu
here=$(cd "$(dirname "$0")" && pwd)

if [ "${1:-}" = "--system" ]; then
	prefix=/usr/local
	data=$prefix/share
else
	prefix=$HOME/.local
	data=${XDG_DATA_HOME:-$HOME/.local/share}
fi
bin=$prefix/bin

install -Dm755 "$here/nople" "$bin/nople"
# The desktop file names nople by its path, as ~/.local/bin is not on the
# PATH of every desktop session.
mkdir -p "$data/applications"
sed "s|^Exec=nople |Exec=$bin/nople |" "$here/nople.desktop" >"$data/applications/nople.desktop"
chmod 644 "$data/applications/nople.desktop"
install -Dm644 "$here/icons/nople.png" "$data/icons/hicolor/256x256/apps/nople.png"
install -Dm644 "$here/icons/nople.svg" "$data/icons/hicolor/scalable/apps/nople.svg"

command -v update-desktop-database >/dev/null && update-desktop-database -q "$data/applications" || true
command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -qtf "$data/icons/hicolor" 2>/dev/null || true

echo "Nople is installed: $bin/nople"
case ":$PATH:" in
*":$bin:"*) ;;
*) echo "$bin is not on PATH; add it to run nople from a terminal." ;;
esac
