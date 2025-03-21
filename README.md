# Turkis

Zero downtime deployment tool for bare metal servers using Docker containers and HAProxy.

## Features

- Zero downtime deployments with Blue-Green deployment strategy
- Automatic TLS certificate provisioning via Let's Encrypt
- Easy app configuration with a simple YAML file
- Built-in health checks for deployed containers
- Domain routing and HTTPS redirection

## Installation

### Option 1: Download the binary

Download the latest release from [GitHub Releases](https://github.com/ameistad/turkis/releases).

```bash
# Linux (AMD64)
curl -L https://github.com/ameistad/turkis/releases/latest/download/turkis-linux-amd64 -o turkis
chmod +x turkis
sudo mv turkis /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/ameistad/turkis/releases/latest/download/turkis-darwin-arm64 -o turkis
chmod +x turkis
sudo mv turkis /usr/local/bin/
```

### Option 2: Build from source

```bash
git clone https://github.com/ameistad/turkis.git
cd turkis
go build -o turkis ./cmd/cli
sudo mv turkis /usr/local/bin/
```

## Getting Started

### Prerequisites

- Docker installed and running
- User added to the docker group: `sudo usermod -aG docker your_username`
- Log out and log back in for group changes to take effect, or run `newgrp docker`

### Initialize Turkis

```bash
turkis init
```

This will:
- Set up the directory structure at `~/.config/turkis/`
- Create a sample configuration file
- Set up the HAProxy and manager containers

### Configure Your Apps

Edit the configuration file at `~/.config/turkis/apps.yml`:

```yaml
apps:
  - name: "example-app"
    domains:
      - canonical: "example.com"
        aliases:
          - "www.example.com"
      - "api.example.com"
    port: 8080 # Optional: Default is 80
    dockerfile: "/path/to/your/Dockerfile"
    buildContext: "/path/to/your/app"
    env:
      NODE_ENV: "production"
    keepOldContainers: 5 # Optional: Default is 3
    volumes: # Optional
      - "/host/path:/container/path"
    healthCheckPath: "/health" # Optional: Default is "/"
```

### Deploy Your Apps

```bash
# Deploy a single app
turkis deploy example-app

# Deploy all apps
turkis deploy-all

# Check the status of your deployments
turkis status

# List all deployed containers
turkis list

# Roll back to a previous deployment
turkis rollback example-app
```

## Configuration Reference

### TLS Configuration

The `tls` section configures TLS certificate provisioning:

```yaml
tls:
  email: "your-email@example.com"  # Required for Let's Encrypt notifications
```

### App Configuration

Each app in the `apps` array can have the following properties:

- `name`: Unique name for the app (required)
- `domains`: List of domains for the app (required)
  - Simple format: `"example.com"`
  - With aliases: `{ domain: "example.com", aliases: ["www.example.com"] }`
- `dockerfile`: Path to your Dockerfile (required)
- `buildContext`: Build context directory for Docker (required)
- `env`: Environment variables for the container
- `keepOldContainers`: Number of old containers to keep after deployment (default: 3)
- `volumes`: Docker volumes to mount
- `healthCheckPath`: HTTP path for health checks (default: "/")

## Development

### Building the CLI

```bash
go build -o turkis ./cmd/cli
```

## Releasing

Turkis uses GitHub Actions for automated builds and releases.

### Automated Release Process

1. Make sure all code changes are committed and pushed to main:
   - Any workflow or CI/CD changes should be made separately from the release process
   - Push these changes and wait for the workflow to complete before proceeding

2. Update the version number in relevant files:
   ```bash
   # Edit the version constant in internal/version/version.go
   git add internal/version/version.go
   git commit -m "Bump version to v1.0.0"
   git push origin main
   ```

3. Create an annotated tag for the release:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0: Brief description of changes"
   git push origin v1.0.0
   ```

4. GitHub Actions will automatically:
   - Run all tests
   - Build the manager Docker image and push it to GitHub Container Registry with version tags
   - Build the CLI binaries for all supported platforms
   - Create a GitHub Release with the binaries attached

5. Verify the release at:
   ```
   https://github.com/ameistad/turkis/releases
   ```

**Important Note**: The workflow is optimized to:
- Only run tests for pushes to branches and pull requests
- Run the full build process (including release) only when pushing a tag
- This separation prevents duplicate builds and conserves GitHub Actions minutes

### Manual Release Process

If you need to build releases manually:

1. Build the CLI for multiple platforms:
   ```bash
   # Linux (AMD64)
   GOOS=linux GOARCH=amd64 go build -o turkis-linux-amd64 ./cmd/cli
   
   # Linux (ARM64)
   GOOS=linux GOARCH=arm64 go build -o turkis-linux-arm64 ./cmd/cli
   
   # macOS (AMD64)
   GOOS=darwin GOARCH=amd64 go build -o turkis-darwin-amd64 ./cmd/cli
   
   # macOS (ARM64)
   GOOS=darwin GOARCH=arm64 go build -o turkis-darwin-arm64 ./cmd/cli
   
   # Windows
   GOOS=windows GOARCH=amd64 go build -o turkis-windows-amd64.exe ./cmd/cli
   ```

2. Build and push the manager Docker image:
   ```bash
   docker build -t ghcr.io/ameistad/turkis-manager:latest -f build/manager/Dockerfile .
   docker push ghcr.io/ameistad/turkis-manager:latest
   ```


### List of labels that turkis will use to configure HAProxy.

Turkis uses the following Docker container labels to configure HAProxy:
- `turkis.ignore` - If set to true turkis will ignore the container (default: false) 
- `turkis.appName` - Identifies the application name
- `turkis.deployment` - Identifies the deployment ID
- `turkis.domains.all` - A comma-separated list of all domains
- `turkis.domain.<index>` - The canonical domain name for the specified index
- `turkis.domain.<index>.alias.<alias_index>` - Domain aliases that should redirect to the canonical domain
- `turkis.health-check-path` - The path to the health check endpoint
- `turkis.drain-time` - The time in seconds to wait before draining connections (default: 10) 


## License

[MIT License](LICENSE)
