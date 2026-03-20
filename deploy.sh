#!/bin/bash
set -e

HOST="jerboa@jerboa.dad"
REMOTE_DIR="/var/www/jerboa"

echo ":: building..."
make build

echo ":: staging binary..."
scp bin/jerboa "$HOST:$REMOTE_DIR/bin/jerboa.new"

echo ":: syncing migrations..."
scp migrations/*.sql "$HOST:$REMOTE_DIR/migrations/"

echo ":: swapping (stop → rename → start)..."
ssh "$HOST" "sudo systemctl stop jerboa && mv $REMOTE_DIR/bin/jerboa.new $REMOTE_DIR/bin/jerboa && sudo systemctl start jerboa"

echo ":: done"
