#!/bin/sh
# `curl -sSL https://raw.githubusercontent.com/eslusarenko/port-client/master/scripts/install.sh | sh`

set -eu

fail() {
  echo "Error: $*" >&2
  exit 1
}

uname_s=$(uname -s)
uname_m=$(uname -m)

case "$uname_s" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *) fail "Unsupported OS: $uname_s. This installer supports Linux and macOS only." ;;
esac

case "$uname_m" in
  x86_64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) fail "Unsupported architecture: $uname_m. This installer supports amd64 and arm64 only." ;;
esac

tag=$(curl -sSL https://api.github.com/repos/eslusarenko/port-client/releases/latest \
  | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
  | head -n 1)

[ -n "$tag" ] || fail "Could not determine latest release tag from GitHub API."

version=${tag#v}
tarball="port-client_${version}_${os}_${arch}.tar.gz"
base_url="https://github.com/eslusarenko/port-client/releases/download/${tag}"
tarball_url="${base_url}/${tarball}"
checksums_url="${base_url}/checksums.txt"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

echo "Downloading $tarball ..."
curl -sSfL "$tarball_url" -o "$tmpdir/$tarball"
curl -sSfL "$checksums_url" -o "$tmpdir/checksums.txt"

expected_sum=$(awk -v f="$tarball" '$NF == f { print $1; exit }' "$tmpdir/checksums.txt")
[ -n "$expected_sum" ] || fail "No checksum entry found for $tarball."

case "$os" in
  linux)
    actual_sum=$(sha256sum "$tmpdir/$tarball" | awk '{print $1}')
    ;;
  darwin)
    actual_sum=$(shasum -a 256 "$tmpdir/$tarball" | awk '{print $1}')
    ;;
  *)
    fail "Internal error: unsupported OS mapping: $os"
    ;;
esac

[ "$actual_sum" = "$expected_sum" ] || fail "Checksum verification failed for $tarball."

echo "Select install location:"
echo "1) /usr/local/bin (system-wide, default)"
echo "2) $HOME/.local/bin (user-only)"

choice="1"
if [ -r /dev/tty ]; then
  printf "Enter choice [1/2] (default 1): " > /dev/tty
  if read -r choice < /dev/tty; then
    :
  else
    choice="1"
  fi
fi

case "$choice" in
  ""|1)
    install_dir="/usr/local/bin"
    ;;
  2)
    install_dir="$HOME/.local/bin"
    mkdir -p "$install_dir"
    ;;
  *)
    fail "Invalid choice: $choice"
    ;;
esac

tar xzf "$tmpdir/$tarball" -C "$tmpdir"

if [ -f "$tmpdir/port" ]; then
  bin_src="$tmpdir/port"
else
  bin_src=$(find "$tmpdir" -type f -name port | head -n 1)
fi

[ -n "${bin_src:-}" ] || fail "Could not find 'port' binary in archive."

if [ "$install_dir" = "/usr/local/bin" ] && [ ! -w "$install_dir" ]; then
  sudo cp "$bin_src" "$install_dir/port"
  sudo chmod 0755 "$install_dir/port"
else
  cp "$bin_src" "$install_dir/port"
  chmod 0755 "$install_dir/port"
fi

if ! echo "$PATH" | grep -q "$install_dir"; then
  echo "Warning: $install_dir is not in your PATH."
  echo "Add it with: export PATH=\"$install_dir:\$PATH\""
fi

echo "Installed to $install_dir/port"
"$install_dir/port" --version || true
