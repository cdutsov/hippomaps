package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/cdutsov/hippomaps/server"
)

// the settings from the environment: the defaults without it, each value read, and what is no number, no
// "on" or "off" and no path refused with the name of the setting
func TestConfig(t *testing.T) {
	for _, k := range []string{"BASE_PATH", "SEARCH_API", "SEARCH_UPSTREAM", "PUBLIC_URL", "TILES_URL", "MAPS_KEY", "USER_AGENT", "PICTURES",
		"PICTURES_PER_IP", "PICTURE_CACHE_MB", "ROUTING_URL", "ROUTES_PER_IP", "ELEVATION_URL", "ELEVATION_KEY", "OVERPASS_URL", "TTS_URL",
		"TRAFFIC_URL", "SAYS_PER_IP", "SEARCHES_PER_IP", "GEOIP_DB", "STRIPE_KEY", "SERVICE_WORKER", "LISTEN"} {
		t.Setenv(k, "")
	}
	cfg, err := config()
	def := server.DefaultConfig()
	if err != nil || cfg.Base != def.Base || cfg.PicturesPerIP != def.PicturesPerIP || !cfg.Pictures || !cfg.ServiceWorker || cfg.RoutingURL != "" ||
		cfg.StripeKey != "" || !strings.HasPrefix(cfg.UserAgent, "hippomaps/"+Version) || listen() != ":8096" {
		t.Errorf("without the environment: %v %+v", err, cfg)
	}
	for k, v := range map[string]string{"BASE_PATH": "/", "TILES_URL": "https://tiles.example/", "ELEVATION_URL": "https://api.example//", "PICTURES": "false",
		"PICTURES_PER_IP": "7", "PICTURE_CACHE_MB": "0", "ROUTES_PER_IP": "8", "SAYS_PER_IP": "9", "SEARCHES_PER_IP": "10", "SERVICE_WORKER": "off",
		"ROUTING_URL": "http://valhalla:8002", "PUBLIC_URL": "https://hippomaps.example", "LISTEN": "127.0.0.1:9000"} {
		t.Setenv(k, v)
	}
	cfg, err = config()
	if err != nil || cfg.Base != "/" || cfg.TilesURL != "https://tiles.example" || cfg.ElevationURL != "https://api.example" || cfg.Pictures ||
		cfg.PicturesPerIP != 7 || cfg.CacheMB != 0 || cfg.RoutesPerIP != 8 || cfg.SaysPerIP != 9 || cfg.SearchesPerIP != 10 || cfg.ServiceWorker ||
		cfg.RoutingURL != "http://valhalla:8002" || cfg.PublicURL != "https://hippomaps.example" || listen() != "127.0.0.1:9000" {
		t.Errorf("from the environment: %v %+v", err, cfg)
	}
	for k, v := range map[string]string{"PICTURES_PER_IP": "many", "ROUTES_PER_IP": "-1", "SEARCHES_PER_IP": "1.5", "SERVICE_WORKER": "maybe", "BASE_PATH": "maps"} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := config(); err == nil || !strings.Contains(err.Error(), k) {
				t.Errorf("%s=%s: %v", k, v, err)
			}
		})
	}
}

// the healthcheck of the container: the server at LISTEN answers /healthz
func TestHealthcheck(t *testing.T) {
	cfg := server.DefaultConfig()
	srv := httptest.NewServer(server.New(cfg))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	for _, addr := range []string{":" + port, "127.0.0.1:" + port} {
		t.Setenv("LISTEN", addr)
		if err := healthcheck(); err != nil {
			t.Errorf("a running server at %s: %v", addr, err)
		}
	}
	sick := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer sick.Close()
	t.Setenv("LISTEN", sick.Listener.Addr().String())
	if err := healthcheck(); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("a server that answers 500: %v", err)
	}
	sick.Close()
	if err := healthcheck(); err == nil {
		t.Errorf("no server: no error")
	}
}

// hippomaps geoip: its arguments, and a file that is no database fails (the check of a new file before it
// replaces the old one)
func TestGeoIPCommand(t *testing.T) {
	if err := geoip([]string{"only-a-file"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("without an address: %v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.mmdb")
	if err := os.WriteFile(bad, []byte(strings.Repeat("not a database ", 100)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := geoip([]string{bad, "192.0.2.1"}); err == nil {
		t.Errorf("a file that is no database: no error")
	}
	if err := geoip([]string{filepath.Join(t.TempDir(), "none.mmdb"), "192.0.2.1"}); err == nil {
		t.Errorf("no file: no error")
	}
}
