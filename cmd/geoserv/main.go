package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/avdoseferovic/geoserv/internal/config"
	"github.com/avdoseferovic/geoserv/internal/content"
	"github.com/avdoseferovic/geoserv/internal/db"
	pubdata "github.com/avdoseferovic/geoserv/internal/pub"
	"github.com/avdoseferovic/geoserv/internal/quest"
	"github.com/avdoseferovic/geoserv/internal/server"
	"github.com/avdoseferovic/geoserv/internal/sln"
	"github.com/avdoseferovic/geoserv/internal/world"

	// Register packet handlers
	_ "github.com/avdoseferovic/geoserv/internal/player/handlers"
)

const banner = `
 ██████╗ ███████╗ ██████╗ ███████╗███████╗██████╗ ██╗   ██╗
██╔════╝ ██╔════╝██╔═══██╗██╔════╝██╔════╝██╔══██╗██║   ██║
██║  ███╗█████╗  ██║   ██║███████╗█████╗  ██████╔╝██║   ██║
██║   ██║██╔══╝  ██║   ██║╚════██║██╔══╝  ██╔══██╗╚██╗ ██╔╝
╚██████╔╝███████╗╚██████╔╝███████║███████╗██║  ██║ ╚████╔╝
 ╚═════╝ ╚══════╝ ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝  ╚═══╝
The Go-powered Endless Online server`

const version = "0.1.0"

func main() {
	fmt.Printf("%s v%s\n\n", banner, version)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	cfg, err := config.Load("config")
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.Info("config loaded")

	database, err := db.New(cfg.Database)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer database.Close() //nolint:errcheck
	slog.Info("database connected", "driver", cfg.Database.Driver)

	if err := database.Migrate(); err != nil {
		slog.Error("failed to apply database migrations", "err", err)
		os.Exit(1)
	}
	slog.Info("database migrations applied")

	// Load pub files (EIF/ENF/ESF/ECF)
	if err := pubdata.LoadAll(); err != nil {
		slog.Warn("failed to load pub files", "err", err)
	}

	// Load quests
	if err := quest.LoadQuests("data/quests"); err != nil {
		slog.Warn("failed to load quests", "err", err)
	}

	if _, err := content.Load(cfg); err != nil {
		slog.Warn("failed to load service content", "err", err)
	} else {
		slog.Info("service content loaded")
	}

	// Initialize world and load maps
	w := world.New(cfg, database)
	if err := w.LoadMaps(); err != nil {
		slog.Warn("failed to load maps", "err", err)
	}

	// Set NPC HP from ENF data
	w.InitNpcStats()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start world tick loop
	go w.RunTickLoop(ctx)

	srv := server.New(cfg, database, w)
	if err := srv.Start(ctx); err != nil {
		slog.Error("failed to start server", "err", err)
		os.Exit(1)
	}

	go srv.RunPingLoop(ctx)
	go srv.RunSaveLoop(ctx)

	// SLN heartbeat
	go sln.Run(ctx, cfg.SLN, cfg.Server.Port, w.OnlinePlayerCount)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	slog.Info("shutting down server", "signal", sig)
	cancel()
	srv.Shutdown()
	slog.Info("server stopped")
}
