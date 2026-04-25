#!/bin/sh
# Swaves installer – Linux only.
# Usage: curl -fsSL https://swaves.io/install.sh | sh

set -e

REPO="keelii/swaves"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"

# ── OS check ────────────────────────────────────────────────────────────────
os="$(uname -s)"
if [ "$os" != "Linux" ]; then
    echo "Error: swaves installation is only supported on Linux (got: $os)." >&2
    exit 1
fi

# ── Architecture ─────────────────────────────────────────────────────────────
arch="$(uname -m)"
case "$arch" in
    x86_64)  arch="amd64" ;;
    aarch64) arch="arm64" ;;
    *)
        echo "Error: unsupported architecture: $arch." >&2
        exit 1
        ;;
esac

# ── Dependencies ─────────────────────────────────────────────────────────────
for cmd in curl tar; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Error: '$cmd' is required but not found in PATH." >&2
        exit 1
    fi
done

# ── Latest version ───────────────────────────────────────────────────────────
echo "Fetching latest swaves release..."
version="$(curl -fsSL "$GITHUB_API" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
if [ -z "$version" ]; then
    echo "Error: could not determine latest swaves version." >&2
    exit 1
fi
echo "Latest version: $version"

# ── Download URLs ─────────────────────────────────────────────────────────────
base_name="swaves_${version}_linux_${arch}"
archive="${base_name}.tar.gz"
download_url="https://github.com/${REPO}/releases/download/${version}/${archive}"
checksum_url="${download_url}.sha256"

# ── Install directory ─────────────────────────────────────────────────────────
install_dir="${SWAVES_INSTALL:-$HOME/.local/bin}"
exe="${install_dir}/swaves"

mkdir -p "$install_dir"

# ── Download ──────────────────────────────────────────────────────────────────
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "Downloading $archive..."
curl -fsSL --progress-bar -o "$tmp_dir/$archive" "$download_url"

# ── Checksum verification ─────────────────────────────────────────────────────
echo "Verifying checksum..."
curl -fsSL -o "$tmp_dir/$archive.sha256" "$checksum_url"
expected="$(awk '{print $1}' "$tmp_dir/$archive.sha256")"
if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$tmp_dir/$archive" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$tmp_dir/$archive" | awk '{print $1}')"
else
    echo "Warning: sha256sum/shasum not found, skipping checksum verification." >&2
    actual="$expected"
fi
if [ "$actual" != "$expected" ]; then
    echo "Error: checksum mismatch (expected $expected, got $actual)." >&2
    exit 1
fi

# ── Extract ───────────────────────────────────────────────────────────────────
tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"
mv "$tmp_dir/$base_name" "$exe"
chmod +x "$exe"

echo "swaves $version was installed successfully to $exe"

# ── PATH hint ─────────────────────────────────────────────────────────────────
case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *)
        echo ""
        echo "Note: $install_dir is not in your PATH."
        echo "Add the following line to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
        echo "  export PATH=\"\$PATH:$install_dir\""
        ;;
esac

echo ""
echo "Run 'swaves --help' to get started."
