package radio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearch_WithHTTPTestServer(t *testing.T) {
	remoteStations := []Station{
		{Name: "Test FM", URL: "http://testfm/stream", Bitrate: 192, Country: "France"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/json/stations/search", r.URL.Path)
		assert.Equal(t, "test", r.URL.Query().Get("name"))
		assert.Equal(t, "FR", r.URL.Query().Get("countrycode"))

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(remoteStations))
	}))
	defer server.Close()

	origURL := radioBrowserURL
	radioBrowserURL = server.URL + "/json"
	defer func() { radioBrowserURL = origURL }()

	results, err := Search(t.Context(), "test", "")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Test FM", results[0].Name)
	assert.Equal(t, 192, results[0].Bitrate)
}

func TestSearch_ReturnsErrorOnRemoteFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origURL := radioBrowserURL
	radioBrowserURL = server.URL + "/json"
	defer func() { radioBrowserURL = origURL }()

	_, err := Search(t.Context(), "test", "")

	assert.Error(t, err)
}
