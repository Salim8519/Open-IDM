#!/usr/bin/env bash
# Copies the shippable files of the browser extension to $1 (no tests, no node_modules).
# Used for the extension zip and for the copy bundled inside the desktop app.
#   stage-extension.sh <dest>          unpacked build: keeps "key", so the ID is fixed
#   stage-extension.sh <dest> --store  store build: drops "key" (the stores reject it and
#                                      assign their own ID)
set -euo pipefail
dest="$1"
mkdir -p "$dest"
tar -C "$(dirname "$0")/../extension" \
  --exclude=./node_modules --exclude=./tests --exclude=./test-results \
  --exclude='./package*.json' --exclude='./playwright*' --exclude=./.gitignore \
  -cf - . | tar -C "$dest" -xf -
if [ "${2:-}" = "--store" ]; then
  node -e '
    const fs = require("fs"); const p = process.argv[1];
    const m = JSON.parse(fs.readFileSync(p, "utf8")); delete m.key;
    fs.writeFileSync(p, JSON.stringify(m, null, 2) + "\n");' "$dest/manifest.json"
fi
test -f "$dest/manifest.json"
