#!/bin/sh
# Builds the packages of Nople for Linux into dist/:
#
#   packaging/linux/build.sh 0.1.0
#
# makes nople_0.1.0_amd64.deb and nople-0.1.0-linux-x64.tar.gz (with
# install.sh). It builds dist/linux/nople with Go if Go is there; without
# it, it packs the dist/linux/nople that is there, which Go on another OS
# builds without cgo:
#
#   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
#     -ldflags "-s -w -X main.version=0.1.0" -o dist/linux/nople .
#
# Needs dpkg-deb, and fakeroot when not run as root. MAINTAINER sets the
# Maintainer of the package.
set -eu
version=${1:?usage: build.sh VERSION}
maintainer=${MAINTAINER:-Nople contributors <nople@users.noreply.github.com>}
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
dist=$root/dist
bin=$dist/linux/nople

if command -v go >/dev/null; then
	(cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$bin" .)
fi
[ -f "$bin" ] || { echo "build.sh: no $bin, and no Go to build it" >&2; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# --- the .deb, which installs into /usr
pkg=$work/deb
install -Dm755 "$bin" "$pkg/usr/bin/nople"
install -Dm644 "$here/nople.desktop" "$pkg/usr/share/applications/nople.desktop"
install -Dm644 "$root/assets/icon.png" "$pkg/usr/share/icons/hicolor/256x256/apps/nople.png"
install -Dm644 "$root/assets/icon.svg" "$pkg/usr/share/icons/hicolor/scalable/apps/nople.svg"
install -Dm644 "$root/LICENSE" "$pkg/usr/share/doc/nople/copyright"
size=$(du -sk "$pkg/usr" | cut -f1)
mkdir -p "$pkg/DEBIAN"
# nople loads these with dlopen, so no tool finds them by itself: Vulkan
# draws, and the window is Wayland's or X11's.
cat >"$pkg/DEBIAN/control" <<EOF
Package: nople
Version: $version
Section: editors
Priority: optional
Architecture: amd64
Maintainer: $maintainer
Installed-Size: $size
Depends: libc6, libvulkan1, libxkbcommon0, libwayland-client0 | libx11-6
Recommends: mesa-vulkan-drivers | vulkan-icd, libwayland-cursor0, libxi6, git
Homepage: https://github.com/bankai-tensei/nople
Description: simple editor for the age of AI
 Nople opens a folder and shows its file tree, tabs, highlighted code,
 rendered Markdown, Git diffs and a terminal, for reading and checking code
 that AI wrote, and editing it in place. No extensions, no sign-in.
EOF
# The triggers of desktop-file-utils and hicolor-icon-theme update the
# caches, so the package needs no scripts.
deb=$dist/nople_${version}_amd64.deb
if [ "$(id -u)" = 0 ]; then
	dpkg-deb --build --root-owner-group "$pkg" "$deb" >/dev/null
else
	fakeroot dpkg-deb --build --root-owner-group "$pkg" "$deb" >/dev/null
fi

# --- the tarball, with install.sh for ~/.local or /usr/local
name=nople-$version-linux-x64
tree=$work/$name
install -Dm755 "$bin" "$tree/nople"
install -Dm644 "$here/nople.desktop" "$tree/nople.desktop"
install -Dm644 "$root/assets/icon.png" "$tree/icons/nople.png"
install -Dm644 "$root/assets/icon.svg" "$tree/icons/nople.svg"
install -Dm755 "$here/install.sh" "$tree/install.sh"
install -Dm755 "$here/uninstall.sh" "$tree/uninstall.sh"
install -Dm644 "$root/LICENSE" "$tree/LICENSE"
tar -C "$work" --owner=0 --group=0 -czf "$dist/$name.tar.gz" "$name"

ls -l "$deb" "$dist/$name.tar.gz"
