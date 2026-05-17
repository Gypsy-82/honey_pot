package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type GeoData struct {
	Country    string  `json:"country"`
	RegionName string  `json:"regionName"`
	City       string  `json:"city"`
	Zip        string  `json:"zip"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Timezone   string  `json:"timezone"`
	ISP        string  `json:"isp"`
	Org        string  `json:"org"`
	AS         string  `json:"as"`
	Status     string  `json:"status"`
}

var geoHTTP = &http.Client{Timeout: 6 * time.Second}

func geoLookup(ip string) (*GeoData, error) {
	url := fmt.Sprintf(
		"http://ip-api.com/json/%s?fields=status,country,regionName,city,zip,lat,lon,timezone,isp,org,as",
		ip,
	)
	resp, err := geoHTTP.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var g GeoData
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return nil, err
	}
	if g.Status != "success" {
		return nil, fmt.Errorf("geo lookup failed for %s", ip)
	}
	return &g, nil
}
