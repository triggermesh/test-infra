#!/bin/sh

set -e

# Avoid executing trap command twice on EXIT signal.
# Ref. https://stackoverflow.com/a/14812383 
trap 'excode=$?; trap "" EXIT; echo exit code: $excode' EXIT HUP INT QUIT PIPE TERM

# Bypass ramp-requests.py if any argument was passed.
if [ $# -ne 0 ]; then
    exec "$@"
fi

cegen -u "$TARGET_URL" -t "$CE_TYPE" -s "$CE_SOURCE" -d "$CE_DATA" | ramp-requests.py

echo '-------- results_latency.txt --------'
cat.py results_latency.txt

echo '-------- results_success.txt --------'
cat.py results_success.txt
