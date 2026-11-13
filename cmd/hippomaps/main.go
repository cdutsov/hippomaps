// hippomaps is a map app: a search with its results beside a map. All settings
// come from the environment; see README.md.
//
//	hippomaps [serve]                listen on LISTEN, default :8096
//	hippomaps healthcheck            the server answers
//	hippomaps geoip FILE ADDRESS...  the place of each address in a database file (GEOIP_DB)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gitlab.com/cdutsov/hippomaps/server"
)

// Version of the app.
const Version = "0.1.0"

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "healthcheck":
		err = healthcheck()
	case "geoip":
		err = geoip(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q (serve | healthcheck | geoip)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hippomaps:", err)
		os.Exit(1)
	}
}

// regionCode: a country (CH) or a part of the world by its number of the UN (150 Europe, 001 the world), as browsers
// name them (Intl.DisplayNames)
var regionCode = regexp.MustCompile(`^([A-Z]{2}|[0-9]{3})$`)

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func listen() string { return env("LISTEN", ":8096") }

func config() (server.Config, error) {
	cfg := server.DefaultConfig()
	cfg.Base = env("BASE_PATH", cfg.Base)
	cfg.SearchAPI = env("SEARCH_API", cfg.SearchAPI)
	cfg.SearchUpstream = env("SEARCH_UPSTREAM", "")
	cfg.PublicURL = env("PUBLIC_URL", "")
	cfg.TilesURL = strings.TrimRight(env("TILES_URL", cfg.TilesURL), "/")
	cfg.MapsKey = env("MAPS_KEY", "")
	cfg.UserAgent = env("USER_AGENT", "hippomaps/"+Version+" (+https://gitlab.com/cdutsov/hippomaps)")
	var err error
	num := func(name string, def int) int {
		v := os.Getenv(name)
		if v == "" {
			return def
		}
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			err = errors.Join(err, fmt.Errorf("%s: not a number: %q", name, v))
		}
		return n
	}
	cfg.Pictures = env("PICTURES", "true") == "true"
	cfg.PicturesPerIP = num("PICTURES_PER_IP", cfg.PicturesPerIP)
	cfg.CacheMB = num("PICTURE_CACHE_MB", cfg.CacheMB)
	cfg.RoutingURL = env("ROUTING_URL", "")
	cfg.RoutesPerIP = num("ROUTES_PER_IP", cfg.RoutesPerIP)
	cfg.JourneyURL = env("JOURNEY_URL", "")
	for _, c := range strings.Split(env("JOURNEY_REGIONS", ""), ",") {
		if c = strings.ToUpper(strings.TrimSpace(c)); c == "" {
			continue
		} else if !regionCode.MatchString(c) {
			err = errors.Join(err, fmt.Errorf("JOURNEY_REGIONS: country codes as CH,BG, or a part of the world as 150 (Europe): %q", c))
		} else {
			cfg.JourneyRegions = append(cfg.JourneyRegions, c)
		}
	}
	cfg.ElevationURL = strings.TrimRight(env("ELEVATION_URL", cfg.ElevationURL), "/")
	cfg.ElevationKey = env("ELEVATION_KEY", "")
	cfg.OverpassURL = env("OVERPASS_URL", "")
	cfg.TTSURL = env("TTS_URL", "")
	cfg.TrafficURL = env("TRAFFIC_URL", "")
	cfg.SaysPerIP = num("SAYS_PER_IP", cfg.SaysPerIP)
	cfg.SearchesPerIP = num("SEARCHES_PER_IP", cfg.SearchesPerIP)
	cfg.GeoIPDB = env("GEOIP_DB", "")
	cfg.StripeKey = env("STRIPE_KEY", "")
	switch sw := env("SERVICE_WORKER", "on"); sw {
	case "on", "off":
		cfg.ServiceWorker = sw == "on"
	default:
		err = errors.Join(err, fmt.Errorf("SERVICE_WORKER: on or off: %q", sw))
	}
	if !strings.HasPrefix(cfg.Base, "/") {
		err = errors.Join(err, fmt.Errorf("BASE_PATH: must start with /: %q", cfg.Base))
	}
	cfg.Log = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return cfg, err
}

func serve() error {
	cfg, err := config()
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: listen(), Handler: server.New(cfg), ReadHeaderTimeout: 10 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 90 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	cfg.Log.Info("listening", "addr", srv.Addr, "version", Version, "base", cfg.Base, "pictures", cfg.Pictures, "routes", cfg.RoutingURL != "", "journeys", cfg.JourneyURL != "", "heights", cfg.ElevationKey != "", "voice", cfg.TTSURL != "", "traffic", cfg.TrafficURL != "", "geoip", cfg.GeoIPDB != "", "key", cfg.MapsKey != "")
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// healthcheck asks the running server for /healthz.
func healthcheck() error {
	addr := listen()
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: HTTP %d", resp.StatusCode)
	}
	return nil
}

// geoip prints the place of each address in a database file: latitude, longitude and zoom of the map, or
// "none"; it fails when the file is not a database (a check of a new file before it replaces the old one).
func geoip(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: hippomaps geoip FILE ADDRESS...")
	}
	for _, ip := range args[1:] {
		n, ok, err := server.LookupNear(args[0], ip)
		if err != nil {
			return fmt.Errorf("%s: %w", ip, err)
		}
		if !ok {
			fmt.Println(ip, "none")
			continue
		}
		fmt.Printf("%s %.3f,%.3f %d\n", ip, n.Lat, n.Lon, n.Zoom())
	}
	return nil
}
