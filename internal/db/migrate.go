package db

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	mysqlmigrate "github.com/golang-migrate/migrate/v4/database/mysql"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/mysql/*.sql migrations/sqlite/*.sql
var migrationFiles embed.FS

func (d *Database) Migrate() error {
	migrationsDir, databaseDriverName, err := d.migrationConfig()
	if err != nil {
		return err
	}

	source, err := iofs.New(migrationFiles, migrationsDir)
	if err != nil {
		return fmt.Errorf("opening embedded migrations: %w", err)
	}

	driver, err := d.migrationDriver()
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", source, databaseDriverName, driver)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}

	return nil
}

func (d *Database) migrationConfig() (string, string, error) {
	switch d.driver {
	case "mysql":
		return "migrations/mysql", "mysql", nil
	case "sqlite":
		return "migrations/sqlite", "sqlite", nil
	default:
		return "", "", fmt.Errorf("unsupported database driver: %s", d.driver)
	}
}

func (d *Database) migrationDriver() (migratedb.Driver, error) {
	switch d.driver {
	case "mysql":
		driver, err := mysqlmigrate.WithInstance(d.db, &mysqlmigrate.Config{})
		if err != nil {
			return nil, fmt.Errorf("creating mysql migration driver: %w", err)
		}
		return driver, nil
	case "sqlite":
		driver, err := sqlitemigrate.WithInstance(d.db, &sqlitemigrate.Config{})
		if err != nil {
			return nil, fmt.Errorf("creating sqlite migration driver: %w", err)
		}
		return driver, nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", d.driver)
	}
}

func migrationNames(driver string) ([]string, error) {
	dir, _, err := (&Database{driver: driver}).migrationConfig()
	if err != nil {
		return nil, err
	}

	entries, err := fs.ReadDir(migrationFiles, dir)
	if err != nil {
		return nil, fmt.Errorf("reading embedded migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names, nil
}
