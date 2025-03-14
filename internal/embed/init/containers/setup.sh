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

# Ensure Docker network exists
echo "Setting up Docker network..."
if docker network ls | grep -q turkis-public; then
  echo "Found existing turkis-public network, removing it..."
  # Check if any containers are using this network and disconnect them
  CONTAINERS=$(docker network inspect -f '{{range .Containers}}{{.Name}} {{end}}' turkis-public 2>/dev/null)
  if [ ! -z "$CONTAINERS" ]; then
    echo "Disconnecting containers from network: $CONTAINERS"
    for container in $CONTAINERS; do
      docker network disconnect -f turkis-public "$container" || true
    done
  fi
  # Remove the network
  docker network rm turkis-public || true
fi

# Create the network fresh (without Compose labels)
echo "Creating turkis-public Docker network..."
docker network create turkis-public
echo "Network created."

# Set up certificate directories with correct permissions
echo "Setting up certificate directories..."
mkdir -p cert-storage/accounts
chmod -R 777 cert-storage
echo "Certificate directories created with proper permissions."

# Set up HAProxy socket directory
echo "Setting up HAProxy socket directory..."
mkdir -p haproxy-socket
chmod -R 777 haproxy-socket
echo "Socket directory created with proper permissions."

# Set up HAProxy maps directory for domain mapping
echo "Setting up HAProxy maps directory..."
mkdir -p haproxy-maps
touch haproxy-maps/hosts.map haproxy-maps/redirects.map
chmod -R 777 haproxy-maps
echo "Maps directory created with proper permissions."

# Set up webroot directory for ACME challenges
echo "Setting up ACME challenge directory..."
mkdir -p webroot-storage/.well-known/acme-challenge
chmod -R 777 webroot-storage
echo "Webroot directory created with proper permissions."

# Create initial dummy certificate to help HAProxy start
echo "Creating initial dummy certificate..."
openssl req -x509 -newkey rsa:4096 -keyout cert-storage/default.key -out cert-storage/default.crt -days 365 -nodes -subj "/CN=localhost"
cat cert-storage/default.crt cert-storage/default.key > cert-storage/default.pem
chmod 644 cert-storage/default.pem
echo "Initial certificate created at cert-storage/default.pem"

echo "Setup complete. You can now run 'docker compose up -d' to start the containers."