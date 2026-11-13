#!/usr/bin/env bash
# Copies the shippable files of the browser extension to $1 (no tests, no node_modules).
# Used for the extension zip and for the copy bundled inside the desktop app.
set -euo pipefail
dest="$1"
mkdir -p "$dest"
tar -C "$(dirname "$0")/../extension" \
  --exclude=./node_modules --exclude=./tests --exclude=./test-results \
  --exclude='./package*.json' --exclude='./playwright*' --exclude=./.gitignore \
  -cf - . | tar -C "$dest" -xf -
test -f "$dest/manifest.json"
