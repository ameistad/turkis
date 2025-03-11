#!/bin/sh
set -e

# Load updated configuration
. /etc/lego/lego.conf

echo "Starting certificate renewal..."

# Iterate over each app's domain group (env vars starting with APP_)
for var in $(env | grep '^APP_' | cut -d= -f1); do
  app_name=$(echo "$var" | sed 's/^APP_//')
  domains=$(printenv "$var")
  echo "Renewing certificate for $app_name with domains: $domains"
  lego --email="$EMAIL" --domains="$domains" --path="/etc/lego/certificates/$app_name" --webroot="/var/www/lego" run

  # Extract the first domain for certificate naming
  primary_domain=$(echo "$domains" | cut -d',' -f1)

  # Create HAProxy PEM file by concatenating cert and key
  if [ -f "/etc/lego/certificates/$app_name/certificates/$primary_domain.crt" ]; then
    echo "Creating PEM file for HAProxy..."
    cat "/etc/lego/certificates/$app_name/certificates/$primary_domain.crt" \
        "/etc/lego/certificates/$app_name/certificates/$primary_domain.key" \
        > "/etc/lego/certificates/$app_name.pem"
    chmod 644 "/etc/lego/certificates/$app_name.pem"
  fi
done

# Signal HAProxy to reload certificates
if command -v socat >/dev/null 2>&1; then
  echo "Signaling HAProxy to reload certificates..."
  echo "set ssl cert /usr/local/etc/haproxy/certs/" | socat stdio tcp4-connect:127.0.0.1:9999 || echo "Failed to signal HAProxy"
fi
