# Certificate Manager for Turkis

This package provides TLS certificate management for Turkis using the Let's Encrypt ACME protocol directly from the monitor service.

## Overview

The certificate manager is integrated into the monitor service and automatically:
- Detects domains from container configurations
- Requests TLS certificates from Let's Encrypt
- Handles renewal of certificates
- Configures HAProxy to use the certificates
- Manages HTTP-01 challenges for domain validation

## How It Works

1. **Domain Detection**: When containers with Turkis domain labels are started, their domains are extracted and registered with the certificate manager.

2. **Certificate Issuance**: For each registered domain, the manager checks if a valid certificate exists. If not, it requests a new certificate from Let's Encrypt.

3. **HTTP Challenge**: The manager handles HTTP-01 challenges by running a lightweight HTTP server on port 8080. HAProxy is configured to forward all requests to `/.well-known/acme-challenge/*` to this server.

4. **Certificate Renewal**: The manager periodically checks all certificates and renews any that are nearing expiration (within 30 days).

5. **HAProxy Integration**: When a certificate is issued or renewed, the manager notifies HAProxy to load the new certificate via its runtime API.

## Configuration

The certificate manager accepts the following configuration options:

- `Email`: Email address for Let's Encrypt account (required)
- `CertDir`: Directory where certificates are stored
- `WebRootDir`: Directory for HTTP-01 challenge files
- `HAProxySocket`: Path to HAProxy UNIX socket for runtime API
- `Staging`: Boolean flag to use Let's Encrypt staging environment for testing

## Usage

Certificate management is enabled by default in the monitor service. To disable it, use the `--no-tls` flag or set the `NO_TLS` environment variable to `true`.

### Environment Variables

- `LEGO_EMAIL`: Email address for Let's Encrypt notifications (required)
- `LEGO_STAGING`: Set to `true` to use Let's Encrypt staging environment for testing

### Command Line Flags

- `--no-tls`: Disable TLS certificate management
- `--email`: Email address for Let's Encrypt notifications
- `--staging`: Use Let's Encrypt staging environment for testing

## Notes

- The certificate manager uses the HTTP-01 challenge, which requires port 80 to be accessible from the internet
- HAProxy forwards `/.well-known/acme-challenge/*` requests to the monitor service
- Certificates are stored in PEM format in the `/etc/haproxy/certs` directory

## Error Handling

The certificate manager logs all errors and continues operation. If a certificate request fails, it will automatically retry at the next renewal check.