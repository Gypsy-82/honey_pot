# trackerd

A compiled Go honeypot proxy that generates **magic tracking links**.

When an attacker clicks the link they see the real target page — but trackerd has already captured their IP address, ISP, physical location, OS fingerprint, open ports, browser cookies, and any form input (including passwords) before they know what hit them.

```
You send magic link via SMS / text
              │
              ▼
     Attacker clicks it
              │
    ┌─────────▼──────────────────────────────────────┐
    │  trackerd logs instantly (server-side):         │
    │    • IP address                                 │
    │    • ISP + organization + ASN                   │
    │    • City, region, country, coordinates         │
    │    • Async port scan of attacker's IP           │
    └─────────┬──────────────────────────────────────┘
              │  proxies real target page
              ▼
    ┌─────────────────────────────────────────────────┐
    │  Injected JS runs silently in attacker browser: │
    │    • OS, platform, screen, GPU                  │
    │    • Network type (WiFi / cellular)             │
    │    • Battery level and charging state           │
    │    • WebRTC IP leak (real IP behind some VPNs)  │
    │    • Canvas fingerprint (unique browser hash)   │
    │    • Cookies visible to the page                │
    │    • All form fields on submit (passwords too)  │
    └─────────────────────────────────────────────────┘
              │
    Attacker sees the real target page (nothing looks wrong)
              │
              ▼
    You run: trackerd logs <token>   — recorded mode
         or: trackerd watch <token>  — live mode (auto-refresh)
```

> **Legal notice:** For authorized security testing, honeypot research, and defensive use on systems you own or have explicit written permission to test. Not for unauthorized surveillance.

---

## What gets captured

| Data | How |
|------|-----|
| IP address | Server-side on first packet — instant, no JS needed |
| ISP / organization / ASN | Server-side geo lookup via ip-api.com |
| City, region, country | Server-side geo lookup |
| GPS coordinates | Server-side geo lookup |
| Timezone | Server-side geo lookup + JS confirmation |
| Open ports | Server-side TCP scan of attacker IP (async) |
| OS and platform | JavaScript beacon |
| Browser and version | User-Agent header + JavaScript |
| Screen resolution + pixel ratio | JavaScript beacon |
| GPU model (real + reported) | WebGL fingerprint |
| RAM and CPU cores | JavaScript beacon |
| Network type | JavaScript (WiFi / 4G / 3G / unknown) |
| Network downlink speed | JavaScript beacon |
| Battery level + charging | JavaScript (Chrome/Edge) |
| Dark mode preference | JavaScript beacon |
| Canvas fingerprint | Unique browser identity hash |
| WebRTC IP leak | Real IP even behind some VPNs + local network IPs |
| Cookies | `document.cookie` via JavaScript |
| Form field values | Submit hook — every input field captured before page navigates, **including passwords** |

---

## Architecture

```
[Attacker's device]
        │  clicks magic link
        ▼
[Nginx or Apache — port 80/443]   ← your Kali or VPS
        │  reverse proxy
        ▼
[trackerd — port 5000, localhost only]
        │                    │
   logs hit to DB     fetches real target page
   runs geo lookup    injects tracking JS
   runs port scan     rewrites links to stay proxied
        │                    │
        └────────────────────┘
                 │
        serves modified page to attacker
        (looks 100% real — proxy is invisible)
```

Nginx/Apache handles port 80/443 as the public face. trackerd runs on localhost:5000 only and is never exposed directly to the internet.

---

## Full setup on Kali Linux — step by step

### Step 1 — Clone the repo

```bash
git clone <your-repo-url> trackerd
cd trackerd
```

### Step 2 — Run the setup script

One command handles everything interactively:

```bash
chmod +x deploy/setup-kali.sh
./deploy/setup-kali.sh
```

The script walks you through:

1. **Installs Go** if not already present (downloads to `~/go`)
2. **Builds the binary** — stripped, no symbols, single file
3. **Configures Nginx or Apache** as the reverse proxy on port 80
4. **Locks the firewall** — blocks port 5000 from outside, allows 80/443
5. **Asks how your server is exposed:**

---

**Option A — You have your own domain:**
- Enter your domain name (e.g. `trackerd.yourdomain.com`)
- Script updates Nginx `server_name` to your domain automatically
- Offers to install a free SSL certificate via Let's Encrypt (certbot)
- If certbot fails (DNS not propagated yet), the terminal shows exactly what to fix:

```
╔══════════════════════════════════════════════════════════════╗
║  certbot failed — SSL certificate was NOT issued             ║
╚══════════════════════════════════════════════════════════════╝

  Most common cause: your domain's DNS A record does not yet
  point to this machine's public IP address.

  How to fix:
  1. Log in to your domain registrar (Namecheap, GoDaddy, Cloudflare...)
  2. Set an A record for trackerd.yourdomain.com → your public IP
     (find your public IP: curl -s ifconfig.me)
  3. Wait 5–30 minutes for DNS to propagate
     (check: https://dnschecker.org/#A/trackerd.yourdomain.com)
  4. Then re-run: sudo certbot --nginx -d trackerd.yourdomain.com ...

  Continuing without SSL for now — you can add it later.
```

The script does not crash — it continues with HTTP and lets you finish setup.

---

**Option B — Using ngrok (no domain needed):**
- Script checks if ngrok is installed, shows install commands if not
- Tells you exactly what to run in a second terminal (`ngrok http 80`)
- You paste the ngrok HTTPS URL back and the script continues

---

6. **Asks for your bait URL** — the real page the attacker will see
7. **Starts trackerd** in background automatically
8. **Creates the magic link** and displays it prominently:

```
╔══════════════════════════════════════════════════════════════╗
║  MAGIC LINK — SEND THIS TO YOUR TARGET                       ║
╠══════════════════════════════════════════════════════════════╣
║  https://yourdomain.com/t/Xk9mT2pLqRvN                      ║
╚══════════════════════════════════════════════════════════════╝
```

9. **Offers live watch mode** — stays running, prints hits as they arrive

---

### Step 3 — Send the magic link

Copy the magic link from Step 2 and send it however you choose:
- SMS / text message
- WhatsApp / iMessage / Signal
- Email
- Social media DM

> **Tip:** URL shorteners (bit.ly, etc.) can disguise the domain if needed.

The link looks like any normal HTTPS URL. The attacker has no indication it is a proxy.

### Step 4 — View captured data

**Recorded mode** — shows everything captured so far:
```bash
./trackerd logs Xk9mT2pLqRvN
```

**Live mode** — stays open, prints new hits as they arrive (Ctrl+C to stop):
```bash
./trackerd watch Xk9mT2pLqRvN
```

---

## Example output

```
── Hit #1 ──────────────────────────────────────────────────
  Timestamp:       2025-06-01 14:32:11 UTC
  IP Address:      203.0.113.47
  User-Agent:      Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/124

  Location & ISP:
    Location:      Atlanta, Georgia, 30301 United States
    Coordinates:   33.7490, -84.3880
    Timezone:      America/New_York
    ISP:           Comcast Cable Communications
    Org:           Comcast Atlanta
    ASN:           AS7922 Comcast Cable

  Browser Fingerprint:
    OS/Platform:   Win32
    Language:      en-US
    Screen:        1920x1080
    Pixel Ratio:   1
    Touch Points:  0
    RAM (GB):      8
    CPU Cores:     8
    GPU (real):    NVIDIA GeForce RTX 3070 Direct3D11
    GPU Vendor:    Google Inc. (NVIDIA)
    Network Type:  4g
    Downlink:      10 Mbps
    Battery:       84%
    Charging:      false
    Dark Mode:     true
    Canvas FP:     f4a9c2d1e8b7... (canvas hash)
    Real/Local IPs:[192.168.1.105 10.0.0.1]  (WebRTC leak)
    Cookies:       session=abc123; _ga=GA1.2.xxxxx

  Open Ports (attacker's public IP):
    22     SSH
    80     HTTP
    3389   RDP

  FORM SUBMISSION CAPTURED:
    _action:        https://your-login-page.com/login
    username:       attacker@gmail.com
    password:       Sup3rS3cr3t!
```

---

## Three-terminal layout during operation

```
Terminal 1                Terminal 2                     Terminal 3
──────────────────────    ──────────────────────────     ──────────────────────────
ngrok http 80             TRACKERD_HOST=https://...      ./trackerd create <url>
(if using ngrok)          ./trackerd serve               ./trackerd watch <token>
stay running              stay running                   ./trackerd logs  <token>
                                                         ./trackerd list
```

> If you used `setup-kali.sh`, trackerd is already running in background — you only need Terminal 3.

---

## CLI reference

| Command | Mode | Description |
|---------|------|-------------|
| `./trackerd serve` | Server | Start the proxy. Must be running for all other commands to work. |
| `./trackerd create <url> [label]` | — | Generate a new magic tracking link for a target URL |
| `./trackerd logs <token>` | Recorded | Show all captured data stored for a token |
| `./trackerd watch <token>` | Live | Auto-refresh every 3 seconds — prints new hits as they arrive |
| `./trackerd list` | — | List all tokens and their target URLs |
| `./trackerd version` | — | Print version |

---

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TRACKERD_HOST` | `http://localhost:5000` | Public URL written into magic links. Set to your ngrok URL or domain before running `serve`. |
| `TRACKERD_PORT` | `5000` | Port trackerd listens on. Behind Nginx/Apache this stays on localhost only. |
| `TRACKERD_DB` | `trackerd.db` | Path to the SQLite database. All hits are stored here permanently. |

---

## Using Apache instead of Nginx

```bash
sudo cp deploy/apache/trackerd.conf /etc/apache2/sites-available/trackerd.conf
sudo a2enmod proxy proxy_http headers remoteip
sudo a2dissite 000-default
sudo a2ensite trackerd
sudo systemctl restart apache2
```

---

## Manual build (without setup-kali.sh)

```bash
# Install Go (if not present)
wget -q https://go.dev/dl/go1.22.4.linux-amd64.tar.gz -O /tmp/go.tar.gz
tar -C $HOME -xzf /tmp/go.tar.gz
export PATH=$HOME/go/bin:$PATH

# Build stripped binary
go build -ldflags="-s -w" -trimpath -o trackerd .

# Cross-compile for a remote server
make build-linux   # Linux amd64 — any Linux VPS
make build-arm     # Linux arm64 — Raspberry Pi, AWS Graviton
make build-mac     # macOS arm64
```

---

## Hardening

```bash
# Block direct access to trackerd port (setup-kali.sh does this automatically)
sudo ufw deny 5000
sudo ufw allow 80
sudo ufw allow 443

# Strip binary further with UPX (optional — reduces size and obfuscates)
sudo apt install upx
upx --best ./trackerd
```

trackerd requires no root privileges to run. Always run it as a regular user.

---

## Port scan note

The port scan runs server-side against the attacker's **public IP** immediately after they click the link. Most consumer devices are behind NAT (home router or mobile carrier), so results reflect open ports on their router or carrier infrastructure — not their device directly. Open ports like `22` (SSH), `3389` (RDP), or `8080` still reveal meaningful intel about the network. Mobile attackers on cellular will typically show no open ports.

---

## Project structure

```
trackerd/
├── main.go          CLI — commands, output formatting, live watch loop
├── server.go        HTTP routes, reverse proxy handler, beacon collector
├── proxy.go         HTML rewriting, JS injection, fingerprint payload
├── db.go            SQLite database layer
├── geo.go           IP geolocation — ISP, location, ASN via ip-api.com
├── scanner.go       TCP port scanner — concurrent scan of common ports
├── go.mod           Go module — declares 2 dependencies
├── go.sum           Dependency integrity hashes
├── Makefile         Build shortcuts (build-linux, build-arm, build-mac)
└── deploy/
    ├── nginx/
    │   └── trackerd.conf     Nginx reverse proxy config
    ├── apache/
    │   └── trackerd.conf     Apache reverse proxy config
    └── setup-kali.sh         Interactive setup + wizard for Kali Linux
```
