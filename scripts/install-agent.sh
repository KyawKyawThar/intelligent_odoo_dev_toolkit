#!/usr/bin/env bash
# install-agent.sh — OdooDevTools Agent one-liner installer
#
# Usage (copy from your dashboard):
#   curl -sSL https://raw.githubusercontent.com/KyawKyawThar/intelligent_odoo_dev_toolkit/main/scripts/install-agent.sh | \
#     AGENT_CLOUD_URL=wss://api.yourdomain.com/api/v1/agent/ws \
#     AGENT_REGISTRATION_TOKEN=reg_YOUR_TOKEN_HERE \
#     bash
#
# Optional environment variables:
#   AGENT_VERSION          — pin a specific release tag (default: latest)
#   INSTALL_DIR            — where to put the binary (default: /usr/local/bin)
#   CONFIG_DIR             — where to put agent.env (default: /etc/odoodevtools)
#   INSTALL_SYSTEMD        — set to "false" to skip systemd service (default: true)
#   ODOO_URL               — Odoo server URL for the agent to connect to
#   ODOO_DB                — Odoo database name
#   ODOO_ADMIN_USER        — Odoo admin username
#   ODOO_ADMIN_PASSWORD    — Odoo admin password
#
set -euo pipefail

# ─── Config ───────────────────────────────────────────────────────────────────
REPO="KyawKyawThar/intelligent_odoo_dev_toolkit"
BINARY_NAME="odoodevtools-agent"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-/etc/odoodevtools}"
INSTALL_SYSTEMD="${INSTALL_SYSTEMD:-true}"
SERVICE_NAME="odoodevtools-agent"
SERVICE_USER="odoodt"

# ─── Colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; RESET='\033[0m'
info()  { echo -e "${GREEN}[INFO]${RESET}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${RESET}  $*"; }
error() { echo -e "${RED}[ERROR]${RESET} $*" >&2; exit 1; }
step()  { echo -e "\n${BOLD}▶ $*${RESET}"; }

# ─── Required env vars ────────────────────────────────────────────────────────
: "${AGENT_CLOUD_URL:?'AGENT_CLOUD_URL is required (e.g. wss://api.yourdomain.com/api/v1/agent/ws)'}"
: "${AGENT_REGISTRATION_TOKEN:?'AGENT_REGISTRATION_TOKEN is required (copy from the dashboard)'}"

# ─── Detect OS / architecture ─────────────────────────────────────────────────
step "Detecting platform"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l)  ARCH="armv7" ;;
  *) error "Unsupported architecture: $ARCH. Supported: x86_64, aarch64, armv7l" ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) error "Unsupported OS: $OS. Supported: linux, darwin" ;;
esac

PLATFORM="${OS}-${ARCH}"
info "Platform: $PLATFORM"

# ─── Resolve version ──────────────────────────────────────────────────────────
step "Resolving agent version"
if [ -z "${AGENT_VERSION:-}" ]; then
  AGENT_VERSION=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -z "$AGENT_VERSION" ] && error "Could not fetch latest release tag. Check your internet connection."
fi
info "Installing version: $AGENT_VERSION"

BINARY_FILENAME="${BINARY_NAME}-${PLATFORM}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${AGENT_VERSION}/${BINARY_FILENAME}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${AGENT_VERSION}/checksums.txt"

# ─── Download binary ──────────────────────────────────────────────────────────
step "Downloading agent binary"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

info "URL: $DOWNLOAD_URL"
curl -sSfL "$DOWNLOAD_URL" -o "$TMPDIR/$BINARY_FILENAME" \
  || error "Download failed. Is version $AGENT_VERSION published? Check: https://github.com/${REPO}/releases"

# ─── Verify checksum ──────────────────────────────────────────────────────────
step "Verifying checksum"
curl -sSfL "$CHECKSUM_URL" -o "$TMPDIR/checksums.txt" 2>/dev/null || {
  warn "checksums.txt not found — skipping verification"
}

if [ -f "$TMPDIR/checksums.txt" ]; then
  # Extract only the line for this binary and verify
  EXPECTED=$(grep "$BINARY_FILENAME" "$TMPDIR/checksums.txt" | awk '{print $1}')
  if [ -n "$EXPECTED" ]; then
    ACTUAL=$(sha256sum "$TMPDIR/$BINARY_FILENAME" | awk '{print $1}')
    if [ "$EXPECTED" = "$ACTUAL" ]; then
      info "Checksum OK: $ACTUAL"
    else
      error "Checksum mismatch!\n  Expected: $EXPECTED\n  Actual:   $ACTUAL\nDownload may be corrupted."
    fi
  else
    warn "No checksum entry found for $BINARY_FILENAME — skipping"
  fi
fi

# ─── Install binary ───────────────────────────────────────────────────────────
step "Installing binary to $INSTALL_DIR/$BINARY_NAME"
chmod +x "$TMPDIR/$BINARY_FILENAME"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMPDIR/$BINARY_FILENAME" "$INSTALL_DIR/$BINARY_NAME"
else
  sudo mv "$TMPDIR/$BINARY_FILENAME" "$INSTALL_DIR/$BINARY_NAME"
fi

info "Binary installed: $INSTALL_DIR/$BINARY_NAME"
"$INSTALL_DIR/$BINARY_NAME" --version 2>/dev/null || true

# ─── Write config file ────────────────────────────────────────────────────────
step "Writing config to $CONFIG_DIR/agent.env"
[ -d "$CONFIG_DIR" ] || sudo mkdir -p "$CONFIG_DIR"

sudo tee "$CONFIG_DIR/agent.env" > /dev/null <<EOF
# OdooDevTools Agent configuration
# Generated by install-agent.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ)

# Cloud server — copy from your dashboard
AGENT_CLOUD_URL=${AGENT_CLOUD_URL}
AGENT_REGISTRATION_TOKEN=${AGENT_REGISTRATION_TOKEN}

# Odoo server — edit these to match your Odoo instance
ODOO_URL=${ODOO_URL:-http://localhost:8069}
PG_ODOO_DB=${ODOO_DB:-odoo}
ODOO_ADMIN_USER=${ODOO_ADMIN_USER:-admin}
ODOO_ADMIN_PASSWORD=${ODOO_ADMIN_PASSWORD:-admin}

# Optional tuning
APP_ENV=production
AGENT_SAMPLER_MODE=sampled
AGENT_SAMPLER_RATE=0.1
AGENT_SLOW_THRESHOLD_MS=200
EOF

sudo chmod 600 "$CONFIG_DIR/agent.env"
info "Config written. Edit $CONFIG_DIR/agent.env to set your Odoo credentials."

# ─── Systemd service (Linux only) ────────────────────────────────────────────
if [ "$OS" = "linux" ] && [ "$INSTALL_SYSTEMD" = "true" ] && command -v systemctl &>/dev/null; then
  step "Installing systemd service"

  # Create dedicated service user if it doesn't exist
  if ! id "$SERVICE_USER" &>/dev/null; then
    sudo useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
    info "Created system user: $SERVICE_USER"
  fi

  sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" > /dev/null <<EOF
[Unit]
Description=OdooDevTools Agent
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
EnvironmentFile=${CONFIG_DIR}/agent.env
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
Restart=on-failure
RestartSec=5s
# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=${CONFIG_DIR}

[Install]
WantedBy=multi-user.target
EOF

  sudo systemctl daemon-reload
  sudo systemctl enable "$SERVICE_NAME"
  sudo systemctl restart "$SERVICE_NAME"

  sleep 2
  if systemctl is-active --quiet "$SERVICE_NAME"; then
    info "Service is running"
  else
    warn "Service may have failed to start. Check logs with:"
    warn "  sudo journalctl -u $SERVICE_NAME -n 50 --no-pager"
  fi

elif [ "$OS" = "darwin" ]; then
  step "macOS: no systemd — run the agent manually"
  info "Start the agent with:"
  echo ""
  echo "  $INSTALL_DIR/$BINARY_NAME"
  echo ""
  info "Or add it to launchd for auto-start (optional, not configured by this script)."
fi

# ─── Done ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}✓ OdooDevTools Agent $AGENT_VERSION installed successfully!${RESET}"
echo ""
echo "  Next steps:"
echo "  1. Edit $CONFIG_DIR/agent.env — set ODOO_URL, PG_ODOO_DB, ODOO_ADMIN_USER, ODOO_ADMIN_PASSWORD"
if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
  echo "  2. sudo systemctl restart $SERVICE_NAME"
  echo "  3. sudo journalctl -u $SERVICE_NAME -f   (watch logs)"
else
  echo "  2. Run: $INSTALL_DIR/$BINARY_NAME"
fi
echo "  4. Check your dashboard — the agent will appear as 'online' within ~30 seconds"
echo ""
