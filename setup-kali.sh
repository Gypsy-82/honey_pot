#!/usr/bin/env bash
# trackerd — Kali Linux setup + interactive wizard
# Run once to install, then launches you straight into creating your first trap.
set -euo pipefail

CYAN='\033[36m'
GREEN='\033[32m'
YELLOW='\033[33m'
RED='\033[31;1m'
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'

info()    { echo -e "${CYAN}[trackerd]${RESET} $*"; }
success() { echo -e "${GREEN}[  OK  ]${RESET} $*"; }
warn()    { echo -e "${YELLOW}[ WARN ]${RESET} $*"; }
die()     { echo -e "${RED}[ FAIL ]${RESET} $*" >&2; exit 1; }
banner()  { echo -e "${CYAN}${BOLD}$*${RESET}"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# If the script lives in deploy/, project root is one level up.
# If the script was moved to the repo root, project root is the same directory.
if [ -f "$SCRIPT_DIR/go.mod" ]; then
    PROJECT_DIR="$SCRIPT_DIR"          # script is in repo root
else
    PROJECT_DIR="$(dirname "$SCRIPT_DIR")"  # script is in deploy/
fi

BINARY="$PROJECT_DIR/trackerd"

clear
echo ""
banner "╔══════════════════════════════════════════════════════╗"
banner "║            trackerd  v2.0  —  Kali Setup            ║"
banner "╚══════════════════════════════════════════════════════╝"
echo ""

# ── Step 1: Go ────────────────────────────────────────────────────────────────
info "Checking for Go..."
GOBIN=""
for candidate in "$HOME/go/bin/go" "/usr/local/go/bin/go" "$(which go 2>/dev/null || true)"; do
    if [ -x "$candidate" ] 2>/dev/null; then
        GOBIN="$candidate"
        break
    fi
done

if [ -z "$GOBIN" ]; then
    info "Go not found — downloading Go 1.22.4..."
    GO_TAR="/tmp/go1.22.4.linux-amd64.tar.gz"
    if command -v wget &>/dev/null; then
        wget -q --show-progress "https://go.dev/dl/go1.22.4.linux-amd64.tar.gz" -O "$GO_TAR"
    elif command -v curl &>/dev/null; then
        curl -fL "https://go.dev/dl/go1.22.4.linux-amd64.tar.gz" -o "$GO_TAR"
    else
        die "Need wget or curl to download Go."
    fi
    tar -C "$HOME" -xzf "$GO_TAR" && rm -f "$GO_TAR"
    GOBIN="$HOME/go/bin/go"
    export PATH="$HOME/go/bin:$PATH"
    grep -q 'go/bin' "$HOME/.bashrc" 2>/dev/null || echo 'export PATH="$HOME/go/bin:$PATH"' >> "$HOME/.bashrc"
    grep -q 'go/bin' "$HOME/.zshrc"  2>/dev/null || echo 'export PATH="$HOME/go/bin:$PATH"' >> "$HOME/.zshrc" 2>/dev/null || true
fi
success "Go: $("$GOBIN" version | awk '{print $3}')"

# ── Step 2: build ─────────────────────────────────────────────────────────────
info "Building trackerd binary..."
cd "$PROJECT_DIR"
GOPROXY=direct \
    GONOSUMCHECK="*" \
    GONOSUMDB="*" \
    "$GOBIN" build -ldflags="-s -w" -trimpath -o trackerd .
success "Binary ready: $BINARY ($(du -sh trackerd | cut -f1))"

# ── Step 3: web server ────────────────────────────────────────────────────────
echo ""
banner "── Web Server Setup ─────────────────────────────────────"
echo -e "  Which web server should sit in front of trackerd?"
echo -e "  ${DIM}(Handles port 80, forwards traffic to trackerd on port 5000)${RESET}"
echo ""
echo "  1) Nginx   (recommended)"
echo "  2) Apache"
echo "  3) Skip    (use trackerd directly — no web server)"
echo ""
read -rp "  Choice [1/2/3]: " WEB_CHOICE

case "$WEB_CHOICE" in
1)
    command -v nginx &>/dev/null || { info "Installing Nginx..."; sudo apt-get update -qq && sudo apt-get install -y nginx; }
    NGINX_CONF="$SCRIPT_DIR/nginx/trackerd.conf"
    [ -f "$NGINX_CONF" ] || NGINX_CONF="$SCRIPT_DIR/deploy/nginx/trackerd.conf"
    [ -f "$NGINX_CONF" ] || die "Cannot find nginx/trackerd.conf — make sure deploy/ folder was cloned."
    sudo cp "$NGINX_CONF" /etc/nginx/sites-available/trackerd
    [ -f /etc/nginx/sites-enabled/default ] && sudo rm -f /etc/nginx/sites-enabled/default && warn "Removed Nginx default site"
    sudo ln -sf /etc/nginx/sites-available/trackerd /etc/nginx/sites-enabled/trackerd
    sudo nginx -t && sudo systemctl enable nginx && sudo systemctl restart nginx
    success "Nginx configured and running on port 80"
    WEB_PORT=80
    ;;
2)
    command -v apache2 &>/dev/null || { info "Installing Apache..."; sudo apt-get update -qq && sudo apt-get install -y apache2; }
    sudo a2enmod proxy proxy_http headers remoteip 2>/dev/null || true
    sudo a2dissite 000-default 2>/dev/null || true
    APACHE_CONF="$SCRIPT_DIR/apache/trackerd.conf"
    [ -f "$APACHE_CONF" ] || APACHE_CONF="$SCRIPT_DIR/deploy/apache/trackerd.conf"
    [ -f "$APACHE_CONF" ] || die "Cannot find apache/trackerd.conf — make sure deploy/ folder was cloned."
    sudo cp "$APACHE_CONF" /etc/apache2/sites-available/trackerd.conf
    sudo a2ensite trackerd && sudo systemctl enable apache2 && sudo systemctl restart apache2
    success "Apache configured and running on port 80"
    WEB_PORT=80
    ;;
*)
    warn "Skipping web server — trackerd will serve directly on port 5000"
    WEB_PORT=5000
    ;;
esac

# ── Step 4: firewall ──────────────────────────────────────────────────────────
if command -v ufw &>/dev/null && [ "$WEB_PORT" -eq 80 ]; then
    sudo ufw deny 5000  2>/dev/null && success "Firewall: port 5000 blocked externally" || true
    sudo ufw allow 80   2>/dev/null || true
    sudo ufw allow 443  2>/dev/null || true
fi

# ════════════════════════════════════════════════════════════════════════════════
#  INTERACTIVE WIZARD — Create your first trap
# ════════════════════════════════════════════════════════════════════════════════
echo ""
banner "╔══════════════════════════════════════════════════════╗"
banner "║              Create Your First Honeypot             ║"
banner "╚══════════════════════════════════════════════════════╝"
echo ""

# ── Step 1: How is this server reachable from the internet? ───────────────────
echo -e "  ${CYAN}Step 1 — How is your server exposed to the internet?${RESET}"
echo ""
echo "  1) I have my own domain  (VPS, home server with domain, etc.)"
echo "  2) I'll use ngrok        (tunnel from local Kali — no domain needed)"
echo ""
read -rp "  Choice [1/2]: " EXPOSE_CHOICE

PUBLIC_URL=""

case "$EXPOSE_CHOICE" in
1)
    # ── Own domain path ───────────────────────────────────────────────────────
    echo ""
    echo -e "  ${CYAN}Enter your domain name${RESET} ${DIM}(e.g. trackerd.yourdomain.com or yourdomain.com)${RESET}"
    read -rp "  Domain: " DOMAIN
    DOMAIN="${DOMAIN%/}"
    DOMAIN="${DOMAIN#http://}"
    DOMAIN="${DOMAIN#https://}"

    if [ -z "$DOMAIN" ]; then
        die "Domain cannot be empty."
    fi

    # Update Nginx server_name if Nginx was configured
    if [ "$WEB_PORT" -eq 80 ] && command -v nginx &>/dev/null; then
        sudo sed -i "s/server_name _;/server_name ${DOMAIN};/" \
            /etc/nginx/sites-available/trackerd 2>/dev/null || true
        sudo nginx -t && sudo systemctl reload nginx
        success "Nginx server_name set to: $DOMAIN"
    fi

    # SSL via certbot
    echo ""
    echo -e "  ${CYAN}Do you want HTTPS via Let's Encrypt (certbot)?${RESET}"
    echo -e "  ${DIM}Required: your domain's DNS A record must already point to this machine's IP.${RESET}"
    echo -e "  ${DIM}Strongly recommended — SMS links over plain HTTP may be flagged as unsafe.${RESET}"
    echo ""
    echo "  1) Yes — install certbot and get a free SSL certificate now"
    echo "  2) No  — use HTTP for now (you can add SSL later)"
    echo ""
    read -rp "  Choice [1/2]: " SSL_CHOICE

    if [ "$SSL_CHOICE" = "1" ]; then
        echo ""
        read -rp "  Email address for Let's Encrypt renewal notices: " LE_EMAIL
        info "Installing certbot..."
        sudo apt-get install -y certbot python3-certbot-nginx -qq

        echo ""
        info "Requesting SSL certificate for $DOMAIN ..."
        echo -e "  ${DIM}(certbot will contact Let's Encrypt — your domain must already point to this IP)${RESET}"
        echo ""

        # Run certbot but catch failure instead of letting set -e kill the script
        if sudo certbot --nginx -d "$DOMAIN" \
               --non-interactive --agree-tos --email "$LE_EMAIL" \
               --redirect; then
            success "HTTPS certificate installed — $DOMAIN is now HTTPS"
            PUBLIC_URL="https://${DOMAIN}"
        else
            echo ""
            echo -e "${RED}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
            echo -e "${RED}${BOLD}║  certbot failed — SSL certificate was NOT issued             ║${RESET}"
            echo -e "${RED}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
            echo ""
            echo -e "  ${YELLOW}Most common cause:${RESET} your domain's DNS A record does not yet"
            echo -e "  point to this machine's public IP address."
            echo ""
            echo -e "  ${CYAN}How to fix:${RESET}"
            echo -e "  1. Log in to your domain registrar (Namecheap, GoDaddy, Cloudflare, etc.)"
            echo -e "  2. Find the DNS settings for ${BOLD}$DOMAIN${RESET}"
            echo -e "  3. Set an ${BOLD}A record${RESET} pointing to this machine's public IP"
            echo -e "     ${DIM}(find your public IP: curl -s ifconfig.me)${RESET}"
            echo -e "  4. Wait for DNS to propagate — usually 5 to 30 minutes"
            echo -e "     ${DIM}(check propagation: https://dnschecker.org/#A/$DOMAIN)${RESET}"
            echo -e "  5. Then re-run certbot manually with this exact command:"
            echo ""
            echo -e "     ${BOLD}sudo certbot --nginx -d $DOMAIN --non-interactive --agree-tos --email $LE_EMAIL --redirect${RESET}"
            echo ""
            echo -e "  ${YELLOW}Continuing without SSL for now.${RESET}"
            echo -e "  Magic links will use ${BOLD}http://${DOMAIN}${RESET} until you add SSL."
            echo ""
            PUBLIC_URL="http://${DOMAIN}"
        fi
    else
        warn "Running without SSL — magic links will be http://"
        PUBLIC_URL="http://${DOMAIN}"
    fi
    ;;

2)
    # ── ngrok path ────────────────────────────────────────────────────────────
    echo ""
    if ! command -v ngrok &>/dev/null; then
        warn "ngrok not found. Install it:"
        echo ""
        echo -e "  ${DIM}wget -q https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-amd64.tgz -O /tmp/ngrok.tgz${RESET}"
        echo -e "  ${DIM}sudo tar -C /usr/local/bin -xzf /tmp/ngrok.tgz${RESET}"
        echo -e "  ${DIM}ngrok config add-authtoken YOUR_TOKEN   # free at ngrok.com${RESET}"
        echo ""
        echo -e "  Then in a separate terminal run:  ${CYAN}ngrok http 80${RESET}"
        echo -e "  Copy the HTTPS URL it gives you, then re-run this script."
        echo ""
        exit 0
    fi

    echo -e "  ${CYAN}Open a NEW terminal and run:${RESET}"
    echo ""
    echo -e "    ${BOLD}ngrok http ${WEB_PORT}${RESET}"
    echo ""
    echo -e "  ${DIM}ngrok will print a Forwarding line like:${RESET}"
    echo -e "  ${DIM}  https://abc123.ngrok-free.app -> http://localhost:${WEB_PORT}${RESET}"
    echo ""
    read -rp "  Paste your ngrok HTTPS URL here: " NGROK_URL
    PUBLIC_URL="${NGROK_URL%/}"

    if [ -z "$PUBLIC_URL" ]; then
        die "ngrok URL cannot be empty."
    fi
    ;;

*)
    die "Invalid choice."
    ;;
esac

# Target URL (bait page)
echo ""
echo -e "  ${CYAN}Step 2 — Bait URL${RESET}"
echo -e "  ${DIM}The real page the attacker will see when they click.${RESET}"
echo -e "  ${DIM}Use your own login page, a test page, or any URL you control.${RESET}"
echo ""
read -rp "  Bait URL: " TARGET_URL

if [ -z "$TARGET_URL" ]; then
    die "Bait URL cannot be empty."
fi

# Optional label
echo ""
read -rp "  Label for this trap (optional, press Enter to skip): " TRAP_LABEL

# Start trackerd in background
echo ""
info "Starting trackerd server..."
TRACKERD_HOST="$PUBLIC_URL" TRACKERD_PORT=5000 \
    "$BINARY" serve > /tmp/trackerd_run.log 2>&1 &
TD_PID=$!
sleep 1

if ! kill -0 "$TD_PID" 2>/dev/null; then
    echo ""
    echo -e "${RED}trackerd failed to start. Server log:${RESET}"
    cat /tmp/trackerd_run.log
    exit 1
fi
success "trackerd running (PID $TD_PID)"

# Create the magic link
echo ""
info "Creating magic link..."
CREATE_OUTPUT=$(TRACKERD_PORT=5000 "$BINARY" create "$TARGET_URL" "$TRAP_LABEL" 2>&1)
echo "$CREATE_OUTPUT"

MAGIC_LINK=$(echo "$CREATE_OUTPUT" | grep -oP 'https?://[^\s]+/t/[^\s]+' | head -1)
TOKEN=$(echo "$CREATE_OUTPUT"     | grep "Token" | awk '{print $NF}')

if [ -z "$MAGIC_LINK" ]; then
    warn "Could not parse magic link from output above."
    exit 1
fi

# Display prominently
echo ""
echo -e "${RED}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
echo -e "${RED}${BOLD}║  MAGIC LINK — SEND THIS TO YOUR TARGET                       ║${RESET}"
echo -e "${RED}${BOLD}╠══════════════════════════════════════════════════════════════╣${RESET}"
echo -e "${RED}${BOLD}║  ${MAGIC_LINK}${RESET}"
echo -e "${RED}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
echo ""
echo -e "  ${DIM}Copy the link above and send it via SMS, WhatsApp, email, or DM.${RESET}"
echo -e "  ${DIM}The attacker will see the real bait page — your proxy is invisible.${RESET}"
echo ""

# Live watch or exit
echo -e "  ${CYAN}Watch mode options:${RESET}"
echo "  1) Live watch — stay running, print hits as they arrive"
echo "  2) Exit now   — come back with: trackerd logs $TOKEN"
echo ""
read -rp "  Choice [1/2]: " WATCH_CHOICE

if [ "${WATCH_CHOICE}" = "1" ]; then
    echo ""
    banner "── Live Watch — waiting for hits on: $TOKEN ─────────"
    echo -e "  ${DIM}Ctrl+C to stop watching. trackerd server keeps running in background.${RESET}"
    echo ""
    LAST_COUNT=0
    while true; do
        CURRENT=$(TRACKERD_PORT=5000 "$BINARY" logs "$TOKEN" 2>/dev/null)
        HIT_COUNT=$(echo "$CURRENT" | grep -c "Hit #" || true)
        if [ "$HIT_COUNT" -gt "$LAST_COUNT" ]; then
            clear
            echo "$CURRENT"
            LAST_COUNT=$HIT_COUNT
        fi
        sleep 4
    done
else
    echo ""
    success "Setup complete."
    echo ""
    echo -e "  ${CYAN}Commands:${RESET}"
    echo -e "    Recorded hits : ${BOLD}./trackerd logs $TOKEN${RESET}"
    echo -e "    Live watch    : ${BOLD}./trackerd watch $TOKEN${RESET}"
    echo -e "    All tokens    : ${BOLD}./trackerd list${RESET}"
    echo -e "    New trap      : ${BOLD}./trackerd create <url> [label]${RESET}"
    echo ""
    echo -e "  ${DIM}trackerd server is running in background (PID $TD_PID).${RESET}"
    echo -e "  ${DIM}To stop it: kill $TD_PID${RESET}"
    echo ""
fi
