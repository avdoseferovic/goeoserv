package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/db"
	"github.com/avdo/goeoserv/internal/quest"
	"github.com/avdo/goeoserv/internal/server"
	"github.com/avdo/goeoserv/internal/world"

	// Register packet handlers
	_ "github.com/avdo/goeoserv/internal/player/handlers"
)

const banner = `
 ██████╗  ██████╗ ███████╗ ██████╗ ███████╗███████╗██████╗ ██╗   ██╗
██╔════╝ ██╔═══██╗██╔════╝██╔═══██╗██╔════╝██╔════╝██╔══██╗██║   ██║
██║  ███╗██║   ██║█████╗  ██║   ██║███████╗█████╗  ██████╔╝██║   ██║
██║   ██║██║   ██║██╔══╝  ██║   ██║╚════██║██╔══╝  ██╔══██╗╚██╗ ██╔╝
╚██████╔╝╚██████╔╝███████╗╚██████╔╝███████║███████╗██║  ██║ ╚████╔╝
 ╚═════╝  ╚═════╝ ╚══════╝ ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝  ╚═══╝
The Go-powered Endless Online server`

const version = "0.1.0"

func main() {
	fmt.Printf("%s v%s\n\n", banner, version)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load("config/Config.toml")
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

	// Handle --install flag
	for _, arg := range os.Args[1:] {
		if arg == "--install" {
			if err := installDB(database, cfg.Database.Driver); err != nil {
				slog.Error("failed to install database", "err", err)
				os.Exit(1)
			}
			slog.Info("database installed successfully")
		}
	}

	// Load quests
	if err := quest.LoadQuests("data/quests"); err != nil {
		slog.Warn("failed to load quests", "err", err)
	}

	// Initialize world and load maps
	w := world.New(cfg, database)
	if err := w.LoadMaps(); err != nil {
		slog.Warn("failed to load maps", "err", err)
	}

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

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	slog.Info("shutting down server", "signal", sig)
	cancel()
	srv.Shutdown()
	slog.Info("server stopped")
}

func installDB(database *db.Database, driver string) error {
	var filename string
	switch driver {
	case "mysql":
		filename = "sql/install_mysql.sql"
	case "sqlite":
		filename = "sql/install_sqlite.sql"
	default:
		return fmt.Errorf("unsupported driver: %s", driver)
	}

	script, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filename, err)
	}

	return database.Execute(context.Background(), string(script))
}
