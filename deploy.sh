#!/bin/bash
set -e

HOST="jerboa@jerboa.dad"
REMOTE_DIR="/var/www/jerboa"

echo ":: building..."
make build

echo ":: stopping service..."
ssh "$HOST" "sudo systemctl stop jerboa"

echo ":: deploying binary..."
scp bin/jerboa "$HOST:$REMOTE_DIR/bin/jerboa"

echo ":: syncing migrations..."
scp migrations/*.sql "$HOST:$REMOTE_DIR/migrations/"

echo ":: starting service..."
ssh "$HOST" "sudo systemctl start jerboa"

echo ":: done"
