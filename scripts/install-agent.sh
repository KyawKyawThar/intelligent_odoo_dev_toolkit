#!/usr/bin/env bash
# install-agent.sh — OdooDevTools Agent installer (Linux + macOS)
#
# Usage (copy from your dashboard):
#   curl -sSL https://YOUR_API_DOMAIN/install | \
#     AGENT_CLOUD_URL=wss://YOUR_API_DOMAIN/api/v1/agent/ws \
#     AGENT_REGISTRATION_TOKEN=reg_YOUR_TOKEN_HERE \
#     bash
#
# Optional environment variables:
#   AGENT_API_URL          — base URL of the OdooDevTools API (derived from AGENT_CLOUD_URL if not set)
#   AGENT_VERSION          — pin a specific release tag (default: latest)
#   INSTALL_DIR            — where to put the binary (default: /usr/local/bin)
#   CONFIG_DIR             — where to put agent.env (default: /etc/odoodevtools)
#   ODOO_URL               — Odoo server URL for the agent to connect to
#   ODOO_DB                — Odoo database name
#   ODOO_ADMIN_USER        — Odoo admin username
#   ODOO_ADMIN_PASSWORD    — Odoo admin password
#
set -euo pipefail

# ─── Config ───────────────────────────────────────────────────────────────────
BINARY_NAME="odoodevtools-agent"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-/etc/odoodevtools}"
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

# Derive the HTTPS API base URL from AGENT_CLOUD_URL when not explicitly set.
# wss://api.example.com/... → https://api.example.com
if [ -z "${AGENT_API_URL:-}" ]; then
  AGENT_API_URL=$(echo "$AGENT_CLOUD_URL" | sed -E 's|^wss?://([^/]+).*|\1|')
  AGENT_API_URL="https://${AGENT_API_URL}"
fi

# ─── Detect OS / architecture ─────────────────────────────────────────────────
step "Detecting platform"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)       ARCH="amd64"  ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l)        ARCH="armv7" ;;
  *) error "Unsupported architecture: $ARCH. Supported: x86_64, aarch64, armv7l" ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) error "Unsupported OS: $OS. For Windows use install-agent.ps1 (see your dashboard)." ;;
esac

PLATFORM="${OS}-${ARCH}"
info "Platform: $PLATFORM"

# ─── Resolve version ──────────────────────────────────────────────────────────
step "Resolving agent version"
if [ -z "${AGENT_VERSION:-}" ]; then
  AGENT_VERSION=$(curl -sSf "${AGENT_API_URL}/api/v1/agent/version" \
    | grep -o '"latest":"[^"]*"' \
    | sed 's/"latest":"//;s/"//')
  [ -z "$AGENT_VERSION" ] && error "Could not fetch latest agent version from ${AGENT_API_URL}. Check your internet connection."
fi
info "Installing version: $AGENT_VERSION"

BINARY_FILENAME="${BINARY_NAME}-${PLATFORM}"
DOWNLOAD_URL="${AGENT_API_URL}/api/v1/agent/download?version=${AGENT_VERSION}&platform=${PLATFORM}"
CHECKSUM_URL="${AGENT_API_URL}/api/v1/agent/checksums?version=${AGENT_VERSION}"

# ─── Download binary ──────────────────────────────────────────────────────────
step "Downloading agent binary"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

info "URL: $DOWNLOAD_URL"
curl -sSfL "$DOWNLOAD_URL" -o "$TMPDIR/$BINARY_FILENAME" \
  || error "Download failed. Is version $AGENT_VERSION available? Check your dashboard."

# ─── Verify checksum ──────────────────────────────────────────────────────────
step "Verifying checksum"
curl -sSfL "$CHECKSUM_URL" -o "$TMPDIR/checksums.txt" 2>/dev/null || {
  warn "checksums.txt not found — skipping verification"
}

if [ -f "$TMPDIR/checksums.txt" ]; then
  EXPECTED=$(grep "$BINARY_FILENAME" "$TMPDIR/checksums.txt" | awk '{print $1}')
  if [ -n "$EXPECTED" ]; then
    if command -v sha256sum &>/dev/null; then
      ACTUAL=$(sha256sum "$TMPDIR/$BINARY_FILENAME" | awk '{print $1}')
    else
      ACTUAL=$(shasum -a 256 "$TMPDIR/$BINARY_FILENAME" | awk '{print $1}')  # macOS
    fi
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
# Print the version to confirm the binary runs. Use a timeout so a binary that
# doesn't support --version (or hangs on startup) never blocks the installer.
timeout 5s "$INSTALL_DIR/$BINARY_NAME" --version 2>/dev/null || true

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

# ─── Service install ──────────────────────────────────────────────────────────

if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
  # ── Linux: systemd ────────────────────────────────────────────────────────
  step "Installing systemd service"

  if ! id "$SERVICE_USER" &>/dev/null; then
    sudo useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
    info "Created system user: $SERVICE_USER"
  fi

  sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" > /dev/null <<EOF
[Unit]
Description=OdooDevTools Agent
Documentation=${AGENT_API_URL}/docs
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
EnvironmentFile=${CONFIG_DIR}/agent.env
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
Restart=on-failure
RestartSec=5s
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
    info "systemd service is running"
  else
    warn "Service may have failed to start. Check with:"
    warn "  sudo journalctl -u $SERVICE_NAME -n 50 --no-pager"
  fi

elif [ "$OS" = "darwin" ]; then
  # ── macOS: launchd ────────────────────────────────────────────────────────
  step "Installing launchd service (macOS)"

  PLIST_LABEL="com.odoodevtools.agent"
  PLIST_DIR="/Library/LaunchDaemons"
  PLIST_FILE="${PLIST_DIR}/${PLIST_LABEL}.plist"
  LOG_FILE="/var/log/odoodevtools-agent.log"

  # The binary reads /etc/odoodevtools/agent.env itself — no EnvironmentFile
  # equivalent needed in the plist.
  sudo tee "$PLIST_FILE" > /dev/null <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${PLIST_LABEL}</string>

  <key>ProgramArguments</key>
  <array>
    <string>${INSTALL_DIR}/${BINARY_NAME}</string>
  </array>

  <!-- Restart automatically if it crashes -->
  <key>KeepAlive</key>
  <true/>

  <!-- Start immediately when loaded -->
  <key>RunAtLoad</key>
  <true/>

  <!-- Throttle rapid restart loops (seconds) -->
  <key>ThrottleInterval</key>
  <integer>5</integer>

  <key>StandardOutPath</key>
  <string>${LOG_FILE}</string>
  <key>StandardErrorPath</key>
  <string>${LOG_FILE}</string>
</dict>
</plist>
EOF

  sudo chmod 644 "$PLIST_FILE"
  sudo chown root:wheel "$PLIST_FILE"

  # Unload any old version first, then load the new plist.
  sudo launchctl bootout system "$PLIST_FILE" 2>/dev/null || true
  sudo launchctl bootstrap system "$PLIST_FILE"

  sleep 2
  if sudo launchctl print "system/${PLIST_LABEL}" 2>/dev/null | grep -q "state = running"; then
    info "launchd service is running"
  else
    warn "Service may still be starting. Check with:"
    warn "  sudo launchctl print system/${PLIST_LABEL}"
    warn "  tail -f ${LOG_FILE}"
  fi

  info "Service management commands:"
  echo "  Stop  : sudo launchctl bootout system ${PLIST_FILE}"
  echo "  Start : sudo launchctl bootstrap system ${PLIST_FILE}"
  echo "  Logs  : tail -f ${LOG_FILE}"
fi

# ─── Done ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}✓ OdooDevTools Agent $AGENT_VERSION installed successfully!${RESET}"
echo ""
echo "  Next steps:"
echo "  1. Edit $CONFIG_DIR/agent.env — set ODOO_URL, PG_ODOO_DB, ODOO_ADMIN_USER, ODOO_ADMIN_PASSWORD"
if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
  echo "  2. sudo systemctl restart $SERVICE_NAME"
  echo "  3. sudo journalctl -u $SERVICE_NAME -f"
elif [ "$OS" = "darwin" ]; then
  echo "  2. sudo launchctl bootout system /Library/LaunchDaemons/com.odoodevtools.agent.plist"
  echo "     sudo launchctl bootstrap system /Library/LaunchDaemons/com.odoodevtools.agent.plist"
  echo "  3. tail -f /var/log/odoodevtools-agent.log"
fi
echo "  4. Check your dashboard — the agent will appear as 'online' within ~30 seconds"
echo ""
