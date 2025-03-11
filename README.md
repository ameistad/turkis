# Turkis
Zero downtime deployment tool on bare metal servers using imperative commands.

## Build
```bash 
GOOS=linux GOARCH=amd64 go build -ldflags="-X 'github.com/ameistad/turkis/cmd.version=1.0.0'" -o turkis .
```

## Documentation

### Deployment
1. Check if docker is running
2. Check if HAProxy is running


### Prequisites
- Docker

### Add key pair for github
```bash
ssh-keygen -t ed25519
```

### Add user to docker group
```bash
sudo usermod -aG docker your_username
```

### Example app.yml
```yaml
tls:
  email: "{{ .TLS.Email }}"
apps:
  - name: "example-app"
    domains:
      - domain: "domain.com"
        aliases:
          - "www.domain.com"
          - "m.domain.com"
      - "api.domain.com"
    dockerfile: "/path/to/your/Dockerfile"
    buildContext: "/path/to/your/app"
    env:
      NODE_ENV: "production"
    keepOldContainers: 3
    volumes:
      - "/host/path:/container/path"
      - "/another/host/path:/another/container/path"
  - name: "example-app-two"
    domains:
      - "example-app-two.domain.com"
    dockerfile: "/path/to/your/Dockerfile"
    buildContext: "/path/to/your/app"
    env:
      NODE_ENV: "production"
    keepOldContainers: 3
    volumes:
      - "/host/path:/container/path"
      - "/another/host/path:/another/container/path"
```


## Development
### Monitor

```bash
docker build -t turkis-monitor-dev -f Dockerfile.monitor .
```

```bash
docker run -it --rm \
  --name turkis-monitor-dev \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd):/src \
  --network host \
  turkis-monitor-dev
```
```bash
docker exec -it turkis-monitor-dev bash
```
