#!/bin/bash
# Setup script for Turkis containers

# Get Docker group ID
DOCKER_GID=$(getent group docker | cut -d: -f3)

# If the command failed or DOCKER_GID is empty, use a default value
if [ -z "$DOCKER_GID" ]; then
  echo "Could not detect Docker group ID, using default value 999"
  DOCKER_GID=999
else
  echo "Detected Docker group ID: $DOCKER_GID"
fi

# Create .env file for docker-compose
cat > .env << EOF
# Docker group ID for giving the monitor container access to the Docker socket
DOCKER_GID=$DOCKER_GID

# Set to true to use staging server for testing
LEGO_STAGING=false
EOF

echo "Created .env file with Docker group ID"
echo "You can now run 'docker compose up -d' to start the containers"

# Create initial dummy certificate to help HAProxy start (optional)
if [ ! -d "cert-storage" ]; then
  echo "Creating initial dummy certificate..."
  mkdir -p cert-storage
  openssl req -x509 -newkey rsa:4096 -keyout cert-storage/localhost.key -out cert-storage/localhost.crt -days 365 -nodes -subj "/CN=localhost"
  cat cert-storage/localhost.crt cert-storage/localhost.key > cert-storage/localhost.pem
  echo "Initial certificate created at cert-storage/localhost.pem"
fi