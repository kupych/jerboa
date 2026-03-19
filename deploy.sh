#!/bin/bash
set -e

HOST="jerboa@jerboa.dad"
REMOTE_DIR="/var/www/jerboa"

echo ":: building..."
make build

echo ":: deploying binary..."
scp bin/jerboa "$HOST:$REMOTE_DIR/bin/jerboa"

echo ":: syncing migrations..."
scp -r migrations/ "$HOST:$REMOTE_DIR/migrations/"

echo ":: restarting service..."
ssh "$HOST" "sudo systemctl restart jerboa"

echo ":: done"
