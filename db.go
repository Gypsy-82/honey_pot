package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

type Token struct {
	Token     string    `json:"token"`
	TargetURL string    `json:"target_url"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

type Hit struct {
	ID          int64     `json:"id"`
	Token       string    `json:"token"`
	Timestamp   time.Time `json:"timestamp"`
	IP          string    `json:"ip"`
	UserAgent   string    `json:"user_agent"`
	Headers     string    `json:"headers"`
	Fingerprint string    `json:"fingerprint"`
	FormData    string    `json:"form_data"`
	GeoData     string    `json:"geo_data"`
	PortScan    string    `json:"port_scan"`
}

func initDB(path string) error {
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tokens (
			token      TEXT PRIMARY KEY,
			target_url TEXT NOT NULL,
			label      TEXT DEFAULT '',
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS hits (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			token       TEXT NOT NULL,
			timestamp   TEXT NOT NULL,
			ip          TEXT,
			user_agent  TEXT,
			headers     TEXT,
			fingerprint TEXT,
			form_data   TEXT,
			geo_data    TEXT,
			port_scan   TEXT
		);
	`)
	if err != nil {
		return err
	}
	// Migrate existing DBs that predate geo_data / port_scan columns
	db.Exec("ALTER TABLE hits ADD COLUMN geo_data TEXT")
	db.Exec("ALTER TABLE hits ADD COLUMN port_scan TEXT")
	return nil
}

func randomToken() string {
	b := make([]byte, 12)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func createToken(targetURL, label string) (string, error) {
	token := randomToken()
	_, err := db.Exec(
		"INSERT INTO tokens (token, target_url, label, created_at) VALUES (?, ?, ?, ?)",
		token, targetURL, label, time.Now().UTC().Format(time.RFC3339),
	)
	return token, err
}

func getToken(token string) (*Token, error) {
	row := db.QueryRow("SELECT token, target_url, label, created_at FROM tokens WHERE token=?", token)
	var t Token
	var createdAt string
	if err := row.Scan(&t.Token, &t.TargetURL, &t.Label, &createdAt); err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &t, nil
}

func listTokens() ([]Token, error) {
	rows, err := db.Query("SELECT token, target_url, label, created_at FROM tokens ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []Token
	for rows.Next() {
		var t Token
		var createdAt string
		rows.Scan(&t.Token, &t.TargetURL, &t.Label, &createdAt)
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func logHit(token, ip, userAgent, headers string) (int64, error) {
	res, err := db.Exec(
		"INSERT INTO hits (token, timestamp, ip, user_agent, headers) VALUES (?, ?, ?, ?, ?)",
		token, time.Now().UTC().Format(time.RFC3339), ip, userAgent, headers,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func getHitFingerprint(id int64) string {
	var fp sql.NullString
	db.QueryRow("SELECT fingerprint FROM hits WHERE id=?", id).Scan(&fp)
	if fp.Valid {
		return fp.String
	}
	return ""
}

func updateHitFingerprint(id int64, fp string) {
	db.Exec("UPDATE hits SET fingerprint=? WHERE id=?", fp, id)
}

func updateHitFormData(id int64, fd string) {
	db.Exec("UPDATE hits SET form_data=? WHERE id=?", fd, id)
}

func updateHitGeo(id int64, geo string) {
	db.Exec("UPDATE hits SET geo_data=? WHERE id=?", geo, id)
}

func updateHitPortScan(id int64, ports string) {
	db.Exec("UPDATE hits SET port_scan=? WHERE id=?", ports, id)
}

func getLatestHitID(token string) (int64, bool) {
	var id int64
	err := db.QueryRow("SELECT id FROM hits WHERE token=? ORDER BY id DESC LIMIT 1", token).Scan(&id)
	return id, err == nil
}

func getHitsByToken(token string) ([]Hit, error) {
	rows, err := db.Query(`
		SELECT id, token, timestamp, ip, user_agent,
		       COALESCE(headers,''), COALESCE(fingerprint,''),
		       COALESCE(form_data,''), COALESCE(geo_data,''), COALESCE(port_scan,'')
		FROM hits WHERE token=? ORDER BY id DESC`, token,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		var ts string
		rows.Scan(&h.ID, &h.Token, &ts, &h.IP, &h.UserAgent,
			&h.Headers, &h.Fingerprint, &h.FormData, &h.GeoData, &h.PortScan)
		h.Timestamp, _ = time.Parse(time.RFC3339, ts)
		hits = append(hits, h)
	}
	return hits, nil
}
