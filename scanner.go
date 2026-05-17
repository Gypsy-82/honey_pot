package main

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// commonPorts covers services most relevant to attacker profiling.
// Note: most mobile/cellular IPs are behind carrier-grade NAT — open ports
// there reflect the carrier's infrastructure, not the attacker's device.
var commonPorts = []int{
	21, 22, 23, 25, 53, 80, 110, 135, 139, 143,
	443, 445, 993, 995, 1723, 3306, 3389, 5900, 8080, 8443,
}

var portServices = map[int]string{
	21:   "FTP",
	22:   "SSH",
	23:   "Telnet",
	25:   "SMTP",
	53:   "DNS",
	80:   "HTTP",
	110:  "POP3",
	135:  "MS-RPC",
	139:  "NetBIOS",
	143:  "IMAP",
	443:  "HTTPS",
	445:  "SMB",
	993:  "IMAPS",
	995:  "POP3S",
	1723: "PPTP-VPN",
	3306: "MySQL",
	3389: "RDP",
	5900: "VNC",
	8080: "HTTP-Proxy",
	8443: "HTTPS-Alt",
}

type OpenPort struct {
	Port    int    `json:"port"`
	Service string `json:"service"`
}

// scanPorts does a concurrent TCP connect scan against the attacker's IP.
// Runs in a goroutine — results are written to the DB asynchronously.
func scanPorts(ip string) []OpenPort {
	host := ip
	// Strip port if IP came in as host:port from RemoteAddr
	if h, _, err := net.SplitHostPort(ip); err == nil {
		host = h
	}
	// Skip private/loopback IPs — no point scanning ourselves
	if isPrivateIP(host) {
		return nil
	}

	var mu sync.Mutex
	var open []OpenPort
	var wg sync.WaitGroup
	sem := make(chan struct{}, 50) // 50 concurrent dials max

	for _, port := range commonPorts {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			addr := fmt.Sprintf("%s:%d", host, p)
			conn, err := net.DialTimeout("tcp", addr, 800*time.Millisecond)
			if err == nil {
				conn.Close()
				mu.Lock()
				open = append(open, OpenPort{Port: p, Service: portServices[p]})
				mu.Unlock()
			}
		}(port)
	}
	wg.Wait()

	sort.Slice(open, func(i, j int) bool {
		return open[i].Port < open[j].Port
	})
	return open
}

func isPrivateIP(ip string) bool {
	private := []string{"127.", "10.", "192.168.", "172.16.", "172.17.",
		"172.18.", "172.19.", "172.2", "172.3", "::1", "fc", "fd"}
	for _, prefix := range private {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}
	return false
}
