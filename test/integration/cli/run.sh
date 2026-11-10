#!/usr/bin/env bash
# Runs every test script in this directory and exits non-zero on any failure.
# lib.sh and spike.sh are helpers, not tests.
set -uo pipefail
cd /tests
fail=0
for t in *.sh; do
  case "$t" in run.sh | lib.sh | spike.sh) continue ;; esac
  if bash -euo pipefail "$t"; then echo "PASS $t"; else echo "FAIL $t"; fail=1; fi
done
exit "$fail"
