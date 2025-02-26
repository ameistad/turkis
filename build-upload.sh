#!/bin/bash

# Ensure an argument is provided
if [ -z "$1" ]; then
    echo "Usage: $0 <hostname>"
    exit 1
fi

HOSTNAME=$1

# If turkis exists, remove it
if [ -f turkis ]; then
    rm turkis
fi

GOOS=linux GOARCH=amd64 go build -ldflags="-X 'github.com/ameistad/turkis/cmd.version=0.1.1'" -o turkis .
scp turkis andreas@"$HOSTNAME":/home/andreas/turkis

# Remove turkis after copying
if [ -f turkis ]; then
    rm turkis
fi

