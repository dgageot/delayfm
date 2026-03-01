package radio

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var radioBrowserURL = "https://de1.api.radio-browser.info/json"

type Station struct {
	Name        string `json:"name"`
	URL         string `json:"url_resolved"`
	Bitrate     int    `json:"bitrate"`
	Country     string `json:"country"`
	CountryCode string `json:"countrycode"`
	State       string `json:"state"`
	Language    string `json:"language"`
	Tags        string `json:"tags"`
	Codec       string `json:"codec"`
	Homepage    string `json:"homepage"`
}

// Search finds stations matching the given name.
// If country is empty, it defaults to "FR".
func Search(ctx context.Context, name, country string) ([]Station, error) {
	params := url.Values{
		"name":        {name},
		"countrycode": {strings.ToUpper(cmp.Or(country, "FR"))},
		"codec":       {"MP3"},
		"limit":       {"50"},
		"order":       {"clickcount"},
		"reverse":     {"true"},
		"hidebroken":  {"true"},
	}

	reqURL := radioBrowserURL + "/stations/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "delayfm/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search stations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var stations []Station
	if err := json.NewDecoder(resp.Body).Decode(&stations); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return stations, nil
}
