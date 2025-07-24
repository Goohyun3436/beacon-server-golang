#!/bin/sh
set -e

echo "[ENTRYPOINT] Checking InfluxDB bucket..."
./init_bucket

echo "[ENTRYPOINT] Starting Go TCP Server..."
exec ./beacon