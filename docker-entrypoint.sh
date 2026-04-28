#!/bin/sh
set -e

# Copy static assets into /data so webui and ResultsAll can serve them from CWD
if [ -d /app/dist ] && [ ! -d /data/dist ]; then
	cp -r /app/dist /data/
fi
if [ -d /app/imgs ] && [ ! -d /data/imgs ]; then
	cp -r /app/imgs /data/
fi

exec /app/webui
