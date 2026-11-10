# Systemd Deployment

Deploy GenieACS Relay as a systemd service on Linux servers.

## Quick Install

Clone on the target Linux host, then run the installer from the repo root (builds
Go binary + admin UI, installs under `/opt/genieacs-relay`, enables systemd):

```bash
git clone https://github.com/Cepat-Kilat-Teknologi/genieacs-relay.git
cd genieacs-relay
sudo chmod +x install.sh
sudo ./install.sh
```

API-only (no Node/pnpm): `sudo ./install.sh --api-only`

Remote one-liner after copying the repo:

```bash
sudo ./examples/systemd/install.sh
```

## Manual Installation

### 1. Build Binary

```bash
# Build for Linux amd64
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o genieacs-relay .

# Or for ARM64 (Raspberry Pi, AWS Graviton)
GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o genieacs-relay .
```

### 2. Create User

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin genieacs
```

### 3. Install Binary

```bash
sudo cp genieacs-relay /usr/local/bin/
sudo chmod +x /usr/local/bin/genieacs-relay
```

### 4. Create Config Directory

```bash
sudo mkdir -p /etc/genieacs-relay
sudo mkdir -p /var/lib/genieacs-relay
sudo chown genieacs:genieacs /var/lib/genieacs-relay
```

### 5. Configure Environment

```bash
sudo cp env.example /etc/genieacs-relay/env
sudo chmod 600 /etc/genieacs-relay/env
sudo nano /etc/genieacs-relay/env
```

Edit values as needed:
```bash
GENIEACS_BASE_URL=http://your-genieacs:7557
NBI_AUTH=true
NBI_AUTH_KEY=your-key-here
```

### 6. Install Service

```bash
sudo cp genieacs-relay.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable genieacs-relay
sudo systemctl start genieacs-relay
```

## Commands

```bash
# Start service
sudo systemctl start genieacs-relay

# Stop service
sudo systemctl stop genieacs-relay

# Restart service
sudo systemctl restart genieacs-relay

# Check status
sudo systemctl status genieacs-relay

# View logs
sudo journalctl -u genieacs-relay -f

# View logs (last 100 lines)
sudo journalctl -u genieacs-relay -n 100
```

## Update

From the git clone on the server (after `install.sh`):

```bash
cd genieacs-relay
sudo ./update.sh
```

Same flow as GOTICKET: `git pull` → stop → build Go + admin UI → deploy to `/opt/genieacs-relay` → start.
API-only: `sudo ./update.sh --api-only`

## Uninstall

```bash
sudo systemctl stop genieacs-relay
sudo systemctl disable genieacs-relay
sudo rm /etc/systemd/system/genieacs-relay.service
sudo systemctl daemon-reload
sudo rm /usr/local/bin/genieacs-relay
sudo rm -rf /etc/genieacs-relay
sudo rm -rf /var/lib/genieacs-relay
sudo userdel genieacs
```

## Security Notes

The service file includes security hardening:
- Runs as non-root user
- Read-only filesystem
- Private /tmp
- No privilege escalation
- Memory execution protection
