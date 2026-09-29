#!/bin/sh
# Install the ondx CLI from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/openndx/openndx-core/main/scripts/install.sh | sh
#
# Environment:
#   ONDX_VERSION      Release tag to install (e.g. v0.3.0). Default: latest.
#   ONDX_REPO         GitHub repo to download releases from, for forks/mirrors.
#                     Default: openndx/openndx-core.
#   ONDX_INSTALL_DIR  Where to put the binary. Default: /usr/local/bin if
#                     writable, otherwise $HOME/.local/bin.
set -eu

REPO="${ONDX_REPO:-openndx/openndx-core}"
VERSION="${ONDX_VERSION:-latest}"

err() {
	echo "install.sh: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || err "required command not found: $1"
}

need uname
need tar
need mktemp

if command -v curl >/dev/null 2>&1; then
	download() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -qO "$2" "$1"; }
else
	err "curl or wget is required"
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
linux | darwin) ;;
mingw* | msys* | cygwin*) err "on Windows, use PowerShell: irm https://raw.githubusercontent.com/$REPO/main/scripts/install.ps1 | iex" ;;
*) err "unsupported OS: $os (download a build manually from https://github.com/$REPO/releases)" ;;
esac

arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) err "unsupported architecture: $arch" ;;
esac

if [ "$VERSION" = "latest" ]; then
	base="https://github.com/$REPO/releases/latest/download"
else
	base="https://github.com/$REPO/releases/download/$VERSION"
fi

archive="ondx_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading $archive ($VERSION)..."
download "$base/$archive" "$tmp/$archive" || err "failed to download $base/$archive"
download "$base/checksums.txt" "$tmp/checksums.txt" || err "failed to download $base/checksums.txt"

expected=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$expected" ] || err "$archive not listed in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
else
	err "sha256sum or shasum is required to verify the download"
fi
[ "$expected" = "$actual" ] || err "checksum mismatch for $archive (expected $expected, got $actual)"

tar -xzf "$tmp/$archive" -C "$tmp" ondx

if [ -n "${ONDX_INSTALL_DIR:-}" ]; then
	dir="$ONDX_INSTALL_DIR"
elif [ -w /usr/local/bin ]; then
	dir=/usr/local/bin
else
	dir="$HOME/.local/bin"
fi
mkdir -p "$dir"
install -m 0755 "$tmp/ondx" "$dir/ondx" 2>/dev/null || {
	cp "$tmp/ondx" "$dir/ondx"
	chmod 0755 "$dir/ondx"
}

echo "Installed ondx to $dir/ondx"
case ":$PATH:" in
*":$dir:"*) ;;
*) echo "Note: $dir is not on your PATH; add it, e.g. export PATH=\"$dir:\$PATH\"" ;;
esac
"$dir/ondx" version
