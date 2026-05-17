#!/usr/bin/env bash
# trackerd — complete self-contained setup for Kali Linux
# Works whether this file is in the repo root or inside deploy/
# All configs are written inline — no external file dependencies
set -euo pipefail

# ── colours ───────────────────────────────────────────────────────────────────
CYAN='\033[36m'; GREEN='\033[32m'; YELLOW='\033[33m'
RED='\033[31;1m'; BOLD='\033[1m'; DIM='\033[2m'; RESET='\033[0m'

info()    { echo -e "${CYAN}[trackerd]${RESET} $*"; }
success() { echo -e "${GREEN}[  OK  ]${RESET} $*"; }
warn()    { echo -e "${YELLOW}[ WARN ]${RESET} $*"; }
die()     { echo -e "${RED}[ FAIL ]${RESET} $*" >&2; exit 1; }
banner()  { echo -e "${CYAN}${BOLD}$*${RESET}"; }

# ── locate project root (works from repo root OR deploy/) ─────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/go.mod" ]; then
    PROJECT_DIR="$SCRIPT_DIR"
else
    PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
fi
[ -f "$PROJECT_DIR/go.mod" ] || die "Cannot find go.mod — clone the full repo first."
BINARY="$PROJECT_DIR/trackerd"

clear
echo ""
banner "╔══════════════════════════════════════════════════════╗"
banner "║           trackerd  v2.0  —  Kali Setup             ║"
banner "╚══════════════════════════════════════════════════════╝"
echo ""

# ── Step 1: Go ────────────────────────────────────────────────────────────────
info "Checking for Go..."
GOBIN=""
for candidate in \
    "$(which go 2>/dev/null || true)" \
    "$HOME/go/bin/go" \
    "/usr/local/go/bin/go" \
    "/usr/bin/go"; do
    if [ -n "$candidate" ] && [ -x "$candidate" ]; then
        GOBIN="$candidate"
        break
    fi
done

if [ -z "$GOBIN" ]; then
    info "Go not found — downloading Go 1.22.4 to $HOME/go ..."
    GO_TAR="/tmp/go1.22.4.linux-amd64.tar.gz"
    if command -v wget &>/dev/null; then
        wget -q --show-progress "https://go.dev/dl/go1.22.4.linux-amd64.tar.gz" -O "$GO_TAR"
    elif command -v curl &>/dev/null; then
        curl -fL "https://go.dev/dl/go1.22.4.linux-amd64.tar.gz" -o "$GO_TAR"
    else
        die "Need wget or curl to download Go."
    fi
    rm -rf "$HOME/go"
    tar -C "$HOME" -xzf "$GO_TAR"
    rm -f "$GO_TAR"
    GOBIN="$HOME/go/bin/go"
    export PATH="$HOME/go/bin:$PATH"
    grep -q 'go/bin' "$HOME/.bashrc" 2>/dev/null \
        || echo 'export PATH="$HOME/go/bin:$PATH"' >> "$HOME/.bashrc"
    grep -q 'go/bin' "$HOME/.zshrc" 2>/dev/null \
        || echo 'export PATH="$HOME/go/bin:$PATH"' >> "$HOME/.zshrc" 2>/dev/null || true
fi
success "Go: $("$GOBIN" version | awk '{print $3}') — $GOBIN"

# ── Step 2: build ─────────────────────────────────────────────────────────────
info "Building trackerd binary..."
cd "$PROJECT_DIR"
# GOROOT intentionally omitted — Go finds its own stdlib.
# GOPROXY=direct skips proxy.golang.org which is unreachable on many Kali installs.
GOPROXY=direct GONOSUMCHECK="*" GONOSUMDB="*" \
    "$GOBIN" build -ldflags="-s -w" -trimpath -o trackerd .
success "Binary ready — $(du -sh "$BINARY" | cut -f1)"

# ── Step 3: web server ────────────────────────────────────────────────────────
echo ""
banner "── Web Server Setup ──────────────────────────────────────────"
echo -e "  Which web server should front trackerd on port 80?"
echo ""
echo "  1) Nginx   (recommended)"
echo "  2) Apache"
echo "  3) Skip    (run trackerd directly — no web server)"
echo ""
read -rp "  Choice [1/2/3]: " WEB_CHOICE
WEB_PORT=5000

case "$WEB_CHOICE" in
1)
    if ! command -v nginx &>/dev/null; then
        info "Installing Nginx..."
        sudo apt-get update -qq && sudo apt-get install -y nginx
    fi

    info "Writing Nginx config..."
    sudo tee /etc/nginx/sites-available/trackerd > /dev/null <<'NGINXCONF'
server {
    listen 80;
    listen [::]:80;
    server_name _;

    proxy_connect_timeout  30s;
    proxy_send_timeout     30s;
    proxy_read_timeout     30s;

    location / {
        proxy_pass          http://127.0.0.1:5000;
        proxy_http_version  1.1;
        proxy_set_header    X-Real-IP        $remote_addr;
        proxy_set_header    X-Forwarded-For  $proxy_add_x_forwarded_for;
        proxy_set_header    X-Forwarded-Proto $scheme;
        proxy_set_header    Host             $host;
        proxy_set_header    Connection       "";
        proxy_buffering     off;
    }

    server_tokens off;
    proxy_hide_header X-Powered-By;
}
NGINXCONF

    [ -L /etc/nginx/sites-enabled/default ] && \
        sudo rm -f /etc/nginx/sites-enabled/default && \
        warn "Removed Nginx default site"

    sudo ln -sf /etc/nginx/sites-available/trackerd \
                /etc/nginx/sites-enabled/trackerd
    sudo nginx -t
    sudo systemctl enable nginx
    sudo systemctl restart nginx
    success "Nginx configured and running on port 80"
    WEB_PORT=80
    ;;

2)
    if ! command -v apache2 &>/dev/null; then
        info "Installing Apache..."
        sudo apt-get update -qq && sudo apt-get install -y apache2
    fi

    info "Writing Apache config..."
    sudo a2enmod proxy proxy_http headers remoteip 2>/dev/null || true
    sudo a2dissite 000-default 2>/dev/null || true

    sudo tee /etc/apache2/sites-available/trackerd.conf > /dev/null <<'APACHECONF'
<VirtualHost *:80>
    ServerName localhost
    ProxyPreserveHost On
    ProxyPass        / http://127.0.0.1:5000/
    ProxyPassReverse / http://127.0.0.1:5000/
    RequestHeader set X-Real-IP         "%{REMOTE_ADDR}s"
    RequestHeader set X-Forwarded-For   "%{REMOTE_ADDR}s"
    RequestHeader set X-Forwarded-Proto "http"
    ProxyBadHeader  Ignore
    ServerTokens    Prod
    ServerSignature Off
    Header          unset X-Powered-By
    ErrorLog  /var/log/apache2/trackerd_error.log
    CustomLog /var/log/apache2/trackerd_access.log combined
</VirtualHost>
APACHECONF

    sudo a2ensite trackerd
    sudo systemctl enable apache2
    sudo systemctl restart apache2
    success "Apache configured and running on port 80"
    WEB_PORT=80
    ;;

*)
    warn "Skipping web server — trackerd will run directly on port 5000"
    ;;
esac

# ── Step 4: firewall ──────────────────────────────────────────────────────────
if command -v ufw &>/dev/null && [ "$WEB_PORT" -eq 80 ]; then
    sudo ufw deny  5000 2>/dev/null && success "Firewall: port 5000 blocked from outside" || true
    sudo ufw allow 80   2>/dev/null || true
    sudo ufw allow 443  2>/dev/null || true
fi

# ══════════════════════════════════════════════════════════════════════════════
#  WIZARD — public URL then first honeypot trap
# ══════════════════════════════════════════════════════════════════════════════
echo ""
banner "╔══════════════════════════════════════════════════════╗"
banner "║             Create Your First Honeypot              ║"
banner "╚══════════════════════════════════════════════════════╝"
echo ""

# ── How is this machine reachable? ────────────────────────────────────────────
echo -e "  ${CYAN}How is this machine reachable from the internet?${RESET}"
echo ""
echo "  1) I have my own domain  (VPS or home server with a domain)"
echo "  2) I'll use ngrok        (tunnel from Kali — no domain needed)"
echo ""
read -rp "  Choice [1/2]: " EXPOSE_CHOICE
PUBLIC_URL=""

case "$EXPOSE_CHOICE" in
1)
    echo ""
    echo -e "  ${CYAN}Enter your domain${RESET} ${DIM}(e.g. trap.yourdomain.com)${RESET}"
    read -rp "  Domain: " DOMAIN
    DOMAIN="${DOMAIN%/}"; DOMAIN="${DOMAIN#http://}"; DOMAIN="${DOMAIN#https://}"
    [ -z "$DOMAIN" ] && die "Domain cannot be empty."

    # Update server_name in Nginx if it was configured
    if [ "$WEB_PORT" -eq 80 ] && command -v nginx &>/dev/null; then
        sudo sed -i "s/server_name _;/server_name ${DOMAIN};/" \
            /etc/nginx/sites-available/trackerd 2>/dev/null || true
        sudo nginx -t && sudo systemctl reload nginx
        success "Nginx server_name → $DOMAIN"
    fi

    # SSL
    echo ""
    echo -e "  ${CYAN}Set up HTTPS with Let's Encrypt (certbot)?${RESET}"
    echo -e "  ${DIM}DNS A record for $DOMAIN must already point to this machine's IP.${RESET}"
    echo -e "  ${DIM}Check: curl -s ifconfig.me   |   Propagation: dnschecker.org/#A/$DOMAIN${RESET}"
    echo ""
    echo "  1) Yes — get a free SSL certificate now"
    echo "  2) No  — use HTTP for now"
    echo ""
    read -rp "  Choice [1/2]: " SSL_CHOICE

    if [ "$SSL_CHOICE" = "1" ]; then
        read -rp "  Email for renewal notices: " LE_EMAIL
        sudo apt-get install -y certbot python3-certbot-nginx -qq
        echo ""
        if sudo certbot --nginx -d "$DOMAIN" \
               --non-interactive --agree-tos \
               --email "$LE_EMAIL" --redirect; then
            success "HTTPS certificate installed for $DOMAIN"
            PUBLIC_URL="https://${DOMAIN}"
        else
            echo ""
            echo -e "${RED}${BOLD}╔═══════════════════════════════════════════════════════════╗${RESET}"
            echo -e "${RED}${BOLD}║  certbot failed — SSL certificate was NOT issued          ║${RESET}"
            echo -e "${RED}${BOLD}╚═══════════════════════════════════════════════════════════╝${RESET}"
            echo ""
            echo -e "  ${YELLOW}Most likely cause:${RESET} DNS A record for ${BOLD}$DOMAIN${RESET} does not"
            echo -e "  point to this machine's public IP yet."
            echo ""
            echo -e "  ${CYAN}Fix:${RESET}"
            echo -e "  1. Log into your registrar (Namecheap, Cloudflare, GoDaddy…)"
            echo -e "  2. Add A record: ${BOLD}$DOMAIN${RESET} → $(curl -s ifconfig.me 2>/dev/null || echo 'your-public-ip')"
            echo -e "  3. Wait 5–30 min then re-run:"
            echo -e "     ${BOLD}sudo certbot --nginx -d $DOMAIN --non-interactive --agree-tos --email $LE_EMAIL --redirect${RESET}"
            echo ""
            warn "Continuing with HTTP — add SSL later."
            PUBLIC_URL="http://${DOMAIN}"
        fi
    else
        warn "Using HTTP — magic links will be http://"
        PUBLIC_URL="http://${DOMAIN}"
    fi
    ;;

2)
    if ! command -v ngrok &>/dev/null; then
        warn "ngrok not found. Install it first:"
        echo ""
        echo -e "  ${DIM}wget -q https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-amd64.tgz -O /tmp/ngrok.tgz${RESET}"
        echo -e "  ${DIM}sudo tar -C /usr/local/bin -xzf /tmp/ngrok.tgz${RESET}"
        echo -e "  ${DIM}ngrok config add-authtoken YOUR_TOKEN   # free account at ngrok.com${RESET}"
        echo ""
        echo -e "  Then open a NEW terminal and run: ${CYAN}ngrok http ${WEB_PORT}${RESET}"
        echo -e "  Then re-run this script."
        exit 0
    fi
    echo ""
    echo -e "  ${CYAN}Open a NEW terminal and run:${RESET}  ${BOLD}ngrok http ${WEB_PORT}${RESET}"
    echo -e "  ${DIM}ngrok will show:  Forwarding  https://xxxx.ngrok-free.app -> localhost:${WEB_PORT}${RESET}"
    echo ""
    read -rp "  Paste the ngrok HTTPS URL here: " NGROK_URL
    PUBLIC_URL="${NGROK_URL%/}"
    [ -z "$PUBLIC_URL" ] && die "ngrok URL cannot be empty."
    ;;

*)
    die "Invalid choice."
    ;;
esac

# ── bait URL ──────────────────────────────────────────────────────────────────
echo ""
echo -e "  ${CYAN}Enter the bait URL${RESET} ${DIM}(the real page the attacker will see)${RESET}"
read -rp "  Bait URL: " TARGET_URL
[ -z "$TARGET_URL" ] && die "Bait URL cannot be empty."

echo ""
read -rp "  Label for this trap (optional, press Enter to skip): " TRAP_LABEL

# ── start trackerd ────────────────────────────────────────────────────────────
echo ""
info "Starting trackerd..."
TRACKERD_HOST="$PUBLIC_URL" TRACKERD_PORT=5000 \
    "$BINARY" serve > /tmp/trackerd.log 2>&1 &
TD_PID=$!
sleep 1

if ! kill -0 "$TD_PID" 2>/dev/null; then
    echo -e "${RED}trackerd failed to start. Log:${RESET}"
    cat /tmp/trackerd.log
    exit 1
fi
success "trackerd running (PID $TD_PID)"

# ── create magic link ─────────────────────────────────────────────────────────
echo ""
info "Creating magic link..."
CREATE_OUT=$(TRACKERD_PORT=5000 "$BINARY" create "$TARGET_URL" "$TRAP_LABEL" 2>&1)
echo "$CREATE_OUT"

MAGIC_LINK=$(echo "$CREATE_OUT" | grep -oE 'https?://[^ ]+/t/[^ ]+' | head -1)
TOKEN=$(echo "$CREATE_OUT" | grep "Token" | awk '{print $NF}')

if [ -z "$MAGIC_LINK" ]; then
    warn "Could not parse magic link — check output above."
    exit 1
fi

echo ""
echo -e "${RED}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
echo -e "${RED}${BOLD}║  MAGIC LINK — SEND THIS TO YOUR TARGET VIA SMS / TEXT        ║${RESET}"
echo -e "${RED}${BOLD}╠══════════════════════════════════════════════════════════════╣${RESET}"
echo -e "${RED}${BOLD}║  $MAGIC_LINK${RESET}"
echo -e "${RED}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
echo ""
echo -e "  ${DIM}Attacker clicks → sees the real bait page → you capture everything.${RESET}"
echo ""

# ── watch mode ────────────────────────────────────────────────────────────────
echo -e "  ${CYAN}What do you want to do now?${RESET}"
echo "  1) Live watch — print hits as they arrive (Ctrl+C to stop)"
echo "  2) Exit — come back with: ./trackerd logs $TOKEN"
echo ""
read -rp "  Choice [1/2]: " WATCH_CHOICE

if [ "$WATCH_CHOICE" = "1" ]; then
    echo ""
    banner "── Live Watch — token: $TOKEN ────────────────────────────────"
    echo -e "  ${DIM}Ctrl+C to stop. trackerd keeps running in background.${RESET}"
    echo ""
    LAST=0
    while true; do
        CUR=$(TRACKERD_PORT=5000 "$BINARY" logs "$TOKEN" 2>/dev/null || true)
        N=$(echo "$CUR" | grep -c "Hit #" 2>/dev/null || echo 0)
        if [ "$N" -gt "$LAST" ]; then
            clear
            echo "$CUR"
            LAST=$N
        fi
        sleep 4
    done
else
    echo ""
    success "All done."
    echo ""
    echo -e "  ${CYAN}Commands to use from here:${RESET}"
    echo -e "    ${BOLD}./trackerd logs  $TOKEN${RESET}     — recorded hits"
    echo -e "    ${BOLD}./trackerd watch $TOKEN${RESET}     — live mode"
    echo -e "    ${BOLD}./trackerd create <url> [label]${RESET}  — new trap"
    echo -e "    ${BOLD}./trackerd list${RESET}              — all tokens"
    echo ""
    echo -e "  ${DIM}trackerd is running in background — PID $TD_PID${RESET}"
    echo -e "  ${DIM}To stop it: kill $TD_PID${RESET}"
    echo ""
fi
