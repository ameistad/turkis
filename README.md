# Turkis
Zero downtime deployment tool on bare metal servers using imperative commands.

## Build
```bash 
GOOS=linux GOARCH=amd64 go build -ldflags="-X 'github.com/ameistad/turkis/cmd.version=1.0.0'" -o turkis .
```

### Todo

Restructure commands:
config
  - validate
  - init
## Documentation

### Prequisites
docker

### Add key pair for github
```bash
ssh-keygen -t ed25519
```

### Add user to docker group
```bash
sudo usermod -aG docker your_username
```

### Run Trefik
```bash
docker compose -f ~/.config/turkis/traefik/docker-compose.yml up -d
```
