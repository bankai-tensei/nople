#!/bin/sh
# Builds the installer of Nople for macOS into dist/:
#
#   packaging/macos/build.sh 0.1.0
#
# makes dist/macos/Nople.app and dist/nople-0.1.0-macos-setup.pkg, which
# puts Nople.app into /Applications and links /usr/local/bin/nople to it.
# Only for Apple silicon: gowaku draws with Metal on arm64 alone. Needs Go and
# the tools that come with macOS (sips, iconutil, codesign, pkgbuild,
# productbuild).
#
# The app is signed ad hoc and the package not at all, which is enough on
# the Mac that built them. To hand them out, sign them with a Developer ID
# and notarize them: SIGN_APP="Developer ID Application: ..." signs the app,
# and SIGN_PKG="Developer ID Installer: ..." the package.
set -eu
export COPYFILE_DISABLE=1
version=${1:?usage: build.sh VERSION}
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
dist=$root/dist
app=$dist/macos/Nople.app
pkg=$dist/nople-$version-macos-setup.pkg

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
# Nople needs no cgo.
(cd "$root" && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath \
	-ldflags "-s -w -X main.version=$version" -o "$app/Contents/MacOS/nople" .)
(cd "$root" && go run ./packaging/macos/doctypes -icons "$app/Contents/Resources") >"$work/doctypes.xml"
sed "s/@VERSION@/$version/g" "$here/Info.plist" |
	awk -v f="$work/doctypes.xml" '/@DOCTYPES@/ { while ((getline l <f) > 0) print l; next } { print }' \
	>"$app/Contents/Info.plist"
plutil -lint -s "$app/Contents/Info.plist"
printf 'APPL????' >"$app/Contents/PkgInfo"
cp "$root/LICENSE" "$app/Contents/Resources/LICENSE"

# --- the icon, from the SVG. An icon of macOS leaves a margin round the
# shape (824 of 1024), or it shows larger than those next to it in the Dock.
iconset=$work/nople.iconset
mkdir -p "$iconset"
sips -s format png -Z 824 "$root/assets/icon.svg" --out "$work/shape.png" >/dev/null
sips -p 1024 1024 "$work/shape.png" --out "$work/icon-1024.png" >/dev/null
for size in 16 32 128 256 512; do
	sips -Z $size "$work/icon-1024.png" --out "$iconset/icon_${size}x${size}.png" >/dev/null
	sips -Z $((size * 2)) "$work/icon-1024.png" --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/nople.icns"

# Extended attributes would go into the package as ._ files.
xattr -cr "$app"
codesign --force --sign "${SIGN_APP:--}" ${SIGN_APP:+--options runtime --timestamp} "$app"

# --- the package. Not relocatable, so that the installer puts the app into
# /Applications even when another copy (such as this dist/) is on the disk.
payload=$work/payload
mkdir -p "$payload" "$work/scripts"
ditto --norsrc --noextattr --noqtn "$app" "$payload/Nople.app"
install -m755 "$here/postinstall" "$work/scripts/postinstall"
pkgbuild --analyze --root "$payload" "$work/component.plist" >/dev/null
plutil -replace 0.BundleIsRelocatable -bool NO "$work/component.plist"
pkgbuild --root "$payload" --component-plist "$work/component.plist" \
	--scripts "$work/scripts" --identifier com.github.bankai-tensei.nople \
	--version "$version" --install-location /Applications \
	"$work/nople.pkg" >/dev/null

cat >"$work/distribution.xml" <<EOF
<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="2">
	<title>Nople $version</title>
	<license file="LICENSE"/>
	<options customize="never" require-scripts="false" hostArchitectures="arm64"/>
	<domains enable_localSystem="true"/>
	<volume-check><allowed-os-versions><os-version min="12.0"/></allowed-os-versions></volume-check>
	<choices-outline><line choice="nople"/></choices-outline>
	<choice id="nople" visible="false"><pkg-ref id="com.github.bankai-tensei.nople"/></choice>
	<pkg-ref id="com.github.bankai-tensei.nople" version="$version">nople.pkg</pkg-ref>
</installer-gui-script>
EOF
mkdir -p "$work/resources"
cp "$root/LICENSE" "$work/resources/LICENSE"
productbuild --distribution "$work/distribution.xml" --package-path "$work" \
	--resources "$work/resources" ${SIGN_PKG:+--sign "$SIGN_PKG"} "$pkg" >/dev/null

ls -l "$pkg"
