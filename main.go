package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

const version = "2.0.0"

const (
	cReset  = "\033[0m"
	cRed    = "\033[31;1m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cCyan   = "\033[36m"
	cDim    = "\033[2m"
	cBold   = "\033[1m"
)

func usage() {
	fmt.Printf(`
  trackerd v%s — honeypot proxy tracker

  USAGE:
    trackerd serve                   Start the proxy server
    trackerd create <url> [label]    Generate a magic tracking link
    trackerd logs   <token>          Show all recorded data for a token
    trackerd watch  <token>          Live mode — print new hits as they arrive
    trackerd list                    List all active tokens

  ENV VARS:
    TRACKERD_HOST   Public base URL embedded in magic links
                    (default: http://localhost:5000)
                    Set to your ngrok URL or VPS domain before running serve.
    TRACKERD_PORT   Port trackerd listens on (default: 5000)
    TRACKERD_DB     SQLite database path (default: trackerd.db)

`, version)
}

// apiHost always connects to localhost — TRACKERD_HOST is for the server only.
func apiHost() string {
	port := os.Getenv("TRACKERD_PORT")
	if port == "" {
		port = "5000"
	}
	return "http://localhost:" + port
}

// ── create ────────────────────────────────────────────────────────────────────

func runCreate() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: trackerd create <url> [label]")
		os.Exit(1)
	}
	targetURL := os.Args[2]
	label := ""
	if len(os.Args) > 3 {
		label = strings.Join(os.Args[3:], " ")
	}

	payload, _ := json.Marshal(map[string]string{"url": targetURL, "label": label})
	resp, err := http.Post(apiHost()+"/api/create", "application/json", bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError:%s server not reachable — is trackerd running?\n  %v\n", cRed, cReset, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if errMsg, ok := result["error"]; ok {
		fmt.Fprintf(os.Stderr, "%sError:%s %s\n", cRed, cReset, errMsg)
		os.Exit(1)
	}

	fmt.Printf("\n%s✓ Magic link created%s\n\n", cGreen, cReset)
	fmt.Printf("  %sTarget URL :%s %s\n", cCyan, cReset, result["target"])
	fmt.Printf("  %sToken      :%s %s\n", cCyan, cReset, result["token"])
	if label != "" {
		fmt.Printf("  %sLabel      :%s %s\n", cCyan, cReset, label)
	}
	fmt.Printf("\n  %s%sMagic link : %s%s\n\n", cRed, cBold, result["magic_link"], cReset)
	fmt.Printf("  %sSend this via SMS, WhatsApp, email — however you choose.%s\n", cDim, cReset)
	fmt.Printf("  %sMonitor recorded hits : trackerd logs %s%s\n", cDim, result["token"], cReset)
	fmt.Printf("  %sMonitor live          : trackerd watch %s%s\n\n", cDim, result["token"], cReset)
}

// ── logs (recorded mode) ──────────────────────────────────────────────────────

func runLogs() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: trackerd logs <token>")
		os.Exit(1)
	}
	token := os.Args[2]
	hits := fetchHits(token)
	if len(hits) == 0 {
		fmt.Printf("\n%sNo hits recorded yet for token: %s%s\n\n", cYellow, token, cReset)
		return
	}
	fmt.Printf("\n%sRecorded hits for token: %s%s  (%d hit(s))\n", cCyan, token, cReset, len(hits))
	for i, h := range hits {
		printHit(h, i+1)
	}
}

// ── watch (live mode) ─────────────────────────────────────────────────────────

func runWatch() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: trackerd watch <token>")
		os.Exit(1)
	}
	token := os.Args[2]
	var lastID int64 = -1

	fmt.Printf("\n%s[trackerd] Live watch — token: %s%s\n", cCyan, token, cReset)
	fmt.Printf("%sWaiting for hits... (Ctrl+C to stop)%s\n\n", cDim, cReset)

	for {
		hits := fetchHits(token)
		// Hits are newest-first; walk in reverse to print chronologically
		for i := len(hits) - 1; i >= 0; i-- {
			h := hits[i]
			if h.ID > lastID {
				lastID = h.ID
				printHit(h, int(h.ID))
			}
		}
		time.Sleep(3 * time.Second)
	}
}

// ── list ──────────────────────────────────────────────────────────────────────

func runList() {
	resp, err := http.Get(apiHost() + "/api/tokens")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError:%s %v\n", cRed, cReset, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var tokens []Token
	json.NewDecoder(resp.Body).Decode(&tokens)

	if len(tokens) == 0 {
		fmt.Printf("\n%sNo tokens created yet.%s\n\n", cYellow, cReset)
		return
	}

	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, cCyan+"TOKEN\tLABEL\tTARGET\tCREATED"+cReset)
	fmt.Fprintln(w, strings.Repeat("─", 6)+"\t"+strings.Repeat("─", 5)+"\t"+strings.Repeat("─", 6)+"\t"+strings.Repeat("─", 7))
	for _, t := range tokens {
		label := t.Label
		if label == "" {
			label = cDim + "-" + cReset
		}
		target := t.TargetURL
		if len(target) > 48 {
			target = target[:45] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Token, label, target, t.CreatedAt.Format(time.DateTime))
	}
	w.Flush()
	fmt.Println()
}

// ── helpers ───────────────────────────────────────────────────────────────────

func fetchHits(token string) []Hit {
	resp, err := http.Get(apiHost() + "/api/logs/" + token)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var hits []Hit
	json.NewDecoder(resp.Body).Decode(&hits)
	return hits
}

func printHit(h Hit, n int) {
	fmt.Printf("\n%s── Hit #%d ──────────────────────────────────────────────────%s\n", cCyan, n, cReset)
	fmt.Printf("  %-16s %s\n", "Timestamp:", h.Timestamp.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  %-16s %s%s%s\n", "IP Address:", cRed, h.IP, cReset)
	fmt.Printf("  %-16s %s\n", "User-Agent:", trunc(h.UserAgent, 90))

	// ── Geo / ISP ────────────────────────────────────────────────────
	if h.GeoData != "" {
		var geo GeoData
		if json.Unmarshal([]byte(h.GeoData), &geo) == nil {
			fmt.Printf("\n  %sLocation & ISP:%s\n", cYellow, cReset)
			fmt.Printf("    %-14s %s, %s, %s %s\n", "Location:", geo.City, geo.RegionName, geo.Zip, geo.Country)
			fmt.Printf("    %-14s %.4f, %.4f\n", "Coordinates:", geo.Lat, geo.Lon)
			fmt.Printf("    %-14s %s\n", "Timezone:", geo.Timezone)
			fmt.Printf("    %-14s %s%s%s\n", "ISP:", cBold, geo.ISP, cReset)
			fmt.Printf("    %-14s %s\n", "Org:", geo.Org)
			fmt.Printf("    %-14s %s\n", "ASN:", geo.AS)
		}
	} else {
		fmt.Printf("  %s(geo/ISP lookup pending...)%s\n", cDim, cReset)
	}

	// ── Browser fingerprint ──────────────────────────────────────────
	if h.Fingerprint != "" {
		var fp map[string]interface{}
		if json.Unmarshal([]byte(h.Fingerprint), &fp) == nil {
			fmt.Printf("\n  %sBrowser Fingerprint:%s\n", cYellow, cReset)
			printFPField(fp, "platform", "OS/Platform")
			printFPField(fp, "lang", "Language")
			printFPField(fp, "langs", "All Languages")
			printFPField(fp, "screen", "Screen")
			printFPField(fp, "dpr", "Pixel Ratio")
			printFPField(fp, "touch", "Touch Points")
			printFPField(fp, "dark_mode", "Dark Mode")
			printFPField(fp, "mem", "RAM (GB)")
			printFPField(fp, "cores", "CPU Cores")
			printFPField(fp, "gpu_unmasked", "GPU (real)")
			printFPField(fp, "gpu", "GPU (reported)")
			printFPField(fp, "gpu_vendor_unmasked", "GPU Vendor")
			printFPField(fp, "net_type", "Network Type")
			printFPField(fp, "net_downlink", "Downlink (Mbps)")
			printFPField(fp, "battery", "Battery")
			printFPField(fp, "battery_charging", "Charging")
			printFPField(fp, "plugins", "Plugins")
			if v, ok := fp["canvas"]; ok && fmt.Sprintf("%v", v) != "" {
				fmt.Printf("    %-16s %v %s(canvas hash)%s\n", "Canvas FP:", v, cDim, cReset)
			}
			if v, ok := fp["webrtc_ips"]; ok {
				fmt.Printf("    %-16s %s%v%s %s(WebRTC leak)%s\n", "Real/Local IPs:", cRed, v, cReset, cDim, cReset)
			}
			if v, ok := fp["cookies"]; ok {
				s := fmt.Sprintf("%v", v)
				if s != "" {
					fmt.Printf("    %-16s %s\n", "Cookies:", trunc(s, 120))
				}
			}
		}
	}

	// ── Open ports ───────────────────────────────────────────────────
	if h.PortScan != "" {
		var ports []OpenPort
		if json.Unmarshal([]byte(h.PortScan), &ports) == nil {
			if len(ports) > 0 {
				fmt.Printf("\n  %sOpen Ports (attacker's public IP):%s\n", cYellow, cReset)
				for _, p := range ports {
					fmt.Printf("    %s%-6d%s %s\n", cRed, p.Port, cReset, p.Service)
				}
			} else {
				fmt.Printf("\n  %sOpen Ports:%s none detected (common for mobile/NAT)\n", cYellow, cReset)
			}
		}
	} else {
		fmt.Printf("  %s(port scan pending...)%s\n", cDim, cReset)
	}

	// ── Form capture ─────────────────────────────────────────────────
	if h.FormData != "" {
		var fd map[string]interface{}
		if json.Unmarshal([]byte(h.FormData), &fd) == nil {
			fmt.Printf("\n  %s%sFORM SUBMISSION CAPTURED:%s\n", cRed, cBold, cReset)
			for k, v := range fd {
				fmt.Printf("    %-20s %v\n", k+":", v)
			}
		}
	}
	fmt.Println()
}

func printFPField(m map[string]interface{}, key, label string) {
	v, ok := m[key]
	if !ok || v == nil {
		return
	}
	s := fmt.Sprintf("%v", v)
	if s == "" || s == "0" || s == "<nil>" || s == "false" {
		return
	}
	fmt.Printf("    %-16s %s\n", label+":", trunc(s, 80))
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ── entry point ───────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "serve":
		runServer()
	case "create":
		runCreate()
	case "logs":
		runLogs()
	case "watch":
		runWatch()
	case "list":
		runList()
	case "version", "-v", "--version":
		fmt.Println("trackerd v" + version)
	default:
		usage()
		os.Exit(1)
	}
}
