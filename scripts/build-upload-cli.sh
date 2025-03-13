#!/bin/bash
# filepath: /Users/andreas/Projects/turkis/scripts/build-upload-cli.sh
set -e

# Ensure an argument is provided
if [ -z "$1" ]; then
    echo "Usage: $0 <hostname>"
    exit 1
fi

BINARY_NAME=turkis
HOSTNAME=$1

# Use the current username from the shell
USERNAME=$(whoami)

# If turkis-cli exists, remove it
if [ -f turkis-cli ]; then
    rm turkis-cli
fi

# Extract the version from internal/version/version.go (assumes format: var Version = "v0.1.9")
version=$(grep 'var Version' ../internal/version/version.go | sed 's/.*"\(.*\)".*/\1/')
echo "Building version: $version"

# Build the CLI binary from cmd/cli using the extracted version
GOOS=linux GOARCH=amd64 go build -ldflags="-X 'github.com/ameistad/turkis/cmd.version=$version'" -o $BINARY_NAME ../cmd/cli

# Upload the binary via scp using the current username
scp $BINARY_NAME ${USERNAME}@"$HOSTNAME":/home/${USERNAME}/$BINARY_NAME

# Remove binary after copying
if [ -f $BINARY_NAME ]; then
    rm $BINARY_NAME
fi
