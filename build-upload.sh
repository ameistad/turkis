#!/bin/bash

# Ensure an argument is provided
if [ -z "$1" ]; then
    echo "Usage: $0 <hostname>"
    exit 1
fi

HOSTNAME=$1

# If turkis-cli exists, remove it
if [ -f turkis-cli ]; then
    rm turkis-cli
fi

# Build the CLI binary from cmd/cli
GOOS=linux GOARCH=amd64 go build -ldflags="-X 'github.com/ameistad/turkis/cmd.version=0.1.1'" -o turkis-cli ./cmd/cli
scp turkis-cli andreas@"$HOSTNAME":/home/andreas/turkis-cli

# Remove turkis-cli after copying
if [ -f turkis-cli ]; then
    rm turkis-cli
fi

