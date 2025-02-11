# Turkis
Zero downtime deployment tool on bare metal servers using imperative commands.


### What does turkis mean?
Turkis is cyan in norwegian, which is what you get when you mix blue and green. Blue-green deployments is a common term for zero downtime deployments.

## Build for linux
```bash 
GOOS=linux GOARCH=amd64 go build -ldflags="-X 'github.com/ameistad/turkis/cmd.version=1.0.0'" -o turkis .
```

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
