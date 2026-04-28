#!/bin/sh
set -e

# Copy static assets into data dir so ResultsAll can serve them (ResultsAll expects dist/ and imgs/ under CWD)
if [ -d /app/dist ] && [ ! -d /data/dist ]; then
	cp -r /app/dist /data/
fi
if [ -d /app/imgs ] && [ ! -d /data/imgs ]; then
	cp -r /app/imgs /data/
fi

# Always overwrite previous results so each run is fresh
rm -rf /data/Results /data/Logs

# Run analysis (config from /config via GOLC_CONFIG_FILE)
/app/webui --internal-run "${GOLC_DEVOPS}"

# Serve results on GOLC_RESULTS_PORT or PORT (default 8090)
exec /app/ResultsAll
