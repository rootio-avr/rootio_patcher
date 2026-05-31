package rootio_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rootio_patcher/pkg/rootio"
)

func TestClient_AnalyzePackages_SendsIgnoreList(t *testing.T) {
	var received AnalyzePackagesRequestCapture

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rootio.AnalyzePackagesResponse{})
	}))
	defer srv.Close()

	client := rootio.NewClient(srv.URL, "test-key")
	ignore := []rootio.Package{{Name: "lodash", Version: "4.17.20"}}
	_, err := client.AnalyzePackages(context.Background(), []rootio.Package{{Name: "express", Version: "4.17.1"}}, ignore, "npm")
	require.NoError(t, err)

	assert.Equal(t, 1, len(received.Ignore))
	assert.Equal(t, "lodash", received.Ignore[0].Name)
	assert.Equal(t, "4.17.20", received.Ignore[0].Version)
}

func TestClient_AnalyzePackages_EmptyIgnoreList(t *testing.T) {
	var received AnalyzePackagesRequestCapture

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rootio.AnalyzePackagesResponse{})
	}))
	defer srv.Close()

	client := rootio.NewClient(srv.URL, "test-key")
	_, err := client.AnalyzePackages(context.Background(), []rootio.Package{{Name: "express", Version: "4.17.1"}}, nil, "npm")
	require.NoError(t, err)

	assert.Empty(t, received.Ignore)
}

// AnalyzePackagesRequestCapture mirrors the request shape for test decoding.
type AnalyzePackagesRequestCapture struct {
	Packages []rootio.Package `json:"packages"`
	Ignore   []rootio.Package `json:"ignore"`
}