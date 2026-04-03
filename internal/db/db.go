package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/avdoseferovic/geoserv/internal/config"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

type Database struct {
	db     *sql.DB
	driver string
}

func New(cfg config.Database) (*Database, error) {
	var dsn string
	var driverName string

	switch cfg.Driver {
	case "mysql":
		driverName = "mysql"
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
	case "sqlite":
		driverName = "sqlite"
		dsn = fmt.Sprintf("%s.db", cfg.Name)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Configure connection pool to prevent resource exhaustion
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	return &Database{db: db, driver: cfg.Driver}, nil
}

func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) Driver() string {
	return d.driver
}

func (d *Database) Execute(ctx context.Context, query string, args ...any) error {
	_, err := d.db.ExecContext(ctx, query, args...)
	return err
}

func (d *Database) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return d.db.QueryRowContext(ctx, query, args...)
}

func (d *Database) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

func (d *Database) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return d.db.BeginTx(ctx, nil)
}

func (d *Database) DB() *sql.DB {
	return d.db
}

func (d *Database) CurrentTimestampExpr() string {
	switch d.driver {
	case "mysql":
		return "CURRENT_TIMESTAMP"
	default:
		return "datetime('now')"
	}
}

func (d *Database) AddMinutesExpr(timestampExpr, minutesExpr string) string {
	switch d.driver {
	case "mysql":
		return fmt.Sprintf("DATE_ADD(%s, INTERVAL %s MINUTE)", timestampExpr, minutesExpr)
	default:
		return fmt.Sprintf("datetime(%s, '+' || %s || ' minutes')", timestampExpr, minutesExpr)
	}
}

func (d *Database) AdditiveUpsertClause(conflictColumns []string, targetColumn string) string {
	switch d.driver {
	case "mysql":
		return fmt.Sprintf("ON DUPLICATE KEY UPDATE %s = %s + VALUES(%s)", targetColumn, targetColumn, targetColumn)
	default:
		return fmt.Sprintf("ON CONFLICT(%s) DO UPDATE SET %s = %s + excluded.%s",
			strings.Join(conflictColumns, ", "),
			targetColumn,
			targetColumn,
			targetColumn,
		)
	}
}
