package sln

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/avdo/goeoserv/internal/config"
)

const version = "0.1.0"

// Run starts the SLN heartbeat loop. Blocks until ctx is cancelled.
func Run(ctx context.Context, cfg config.SLN, serverPort string, playerCountFn func() int) {
	if !cfg.Enabled {
		return
	}

	rate := cfg.Rate
	if rate <= 0 {
		rate = 5
	}

	ticker := time.NewTicker(time.Duration(rate) * time.Minute)
	defer ticker.Stop()

	// Send initial heartbeat immediately
	ping(cfg, serverPort, playerCountFn)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ping(cfg, serverPort, playerCountFn)
		}
	}
}

func ping(cfg config.SLN, serverPort string, playerCountFn func() int) {
	params := url.Values{
		"software": {"GOEOSERV"},
		"v":        {version},
		"retry":    {fmt.Sprintf("%d", cfg.Rate*60)},
		"host":     {cfg.Hostname},
		"port":     {serverPort},
		"name":     {cfg.ServerName},
		"url":      {cfg.Site},
		"zone":     {cfg.Zone},
		"players":  {fmt.Sprintf("%d", playerCountFn())},
	}

	reqURL := cfg.URL + "check?" + params.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		slog.Warn("sln request build failed", "err", err)
		return
	}
	req.Header.Set("User-Agent", "EOSERV")

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("sln heartbeat failed", "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("sln heartbeat non-200", "status", resp.StatusCode)
	} else {
		slog.Debug("sln heartbeat sent")
	}
}
