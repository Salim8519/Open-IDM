#!/usr/bin/env bash
# Fails if a Windows binary imports MinGW runtime DLLs. Those DLLs are not on users'
# machines, so such an exe shows "libstdc++-6.dll was not found" and never starts.
# This happens when a Windows build runs with cgo enabled (see ADR 0014).
set -euo pipefail
status=0
for exe in "$@"; do
  for dll in libstdc++-6.dll libgcc_s_seh-1.dll libwinpthread-1.dll; do
    # LC_ALL=C and no -i: Git Bash's grep aborts on binary files otherwise.
    if LC_ALL=C grep -aqF "$dll" "$exe"; then
      echo "::error::$exe imports $dll (built with cgo?). Build with CGO_ENABLED=0."
      status=1
    fi
  done
done
if [ "$status" -eq 0 ]; then echo "OK: no MinGW runtime DLLs in: $*"; fi
exit "$status"
