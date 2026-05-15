package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
)

var (
	pragmas = map[string]string{
		"foreign_keys":  "ON",
		"journal_mode":  "WAL",
		"page_size":     "4096",
		"cache_size":    "-8000",
		"synchronous":   "NORMAL",
		"secure_delete": "ON",
		"busy_timeout":  "30000",
	}
	gooseInitOnce sync.Once
	gooseInitErr  error
)

//go:embed migrations/*.sql
var FS embed.FS

//go:embed migrations_mysql/*.sql
var mysqlFS embed.FS

func init() {
	goose.SetBaseFS(FS)

	if testing.Testing() {
		goose.SetLogger(goose.NopLogger())
	}
}

// Connect opens a SQLite database connection and runs migrations.
func Connect(ctx context.Context, dataDir string) (*sql.DB, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("data.dir is not set")
	}
	dbPath := filepath.Join(dataDir, "hi_agent.db")

	db, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}

	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := InitMigrations(db); err != nil {
		slog.Error("Failed to initialize goose", "error", err)
		return nil, err
	}

	return db, nil
}

func initGoose() error {
	gooseInitOnce.Do(func() {
		goose.SetBaseFS(FS)
		gooseInitErr = goose.SetDialect("sqlite3")
	})

	return gooseInitErr
}

type migrationConfig struct {
	baseFS  embed.FS
	dialect string
	dir     string
}

// ConnectWithOption opens the configured database backend.
//
// When no database configuration is provided, SQLite remains the default.
func ConnectWithOption(ctx context.Context, driver, dataDir, dsn string) (*sql.DB, error) {
	if driver == "" || driver == "sqlite" {
		return Connect(ctx, dataDir)
	}

	switch driver {
	case "mysql":
		return ConnectMySQL(ctx, dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

// ConnectMySQL opens a MySQL database connection and runs MySQL migrations.
func ConnectMySQL(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("mysql dsn is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to mysql database: %w", err)
	}

	if err := InitMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func InitMigrations(db *sql.DB) error {
	cfg, err := migrationConfigForDB(db)
	if err != nil {
		return err
	}
	if cfg.dialect == "sqlite3" {
		if err := initGoose(); err != nil {
			return fmt.Errorf("failed to initialize goose: %w", err)
		}
	} else {
		goose.SetBaseFS(cfg.baseFS)
		if err := goose.SetDialect(cfg.dialect); err != nil {
			return fmt.Errorf("failed to set %s dialect: %w", cfg.dialect, err)
		}
	}
	if err := goose.Up(db, cfg.dir); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	return nil
}

func migrationConfigForDB(db *sql.DB) (migrationConfig, error) {
	switch detectDialect(db) {
	case dialectSQLite:
		return migrationConfig{
			baseFS:  FS,
			dialect: "sqlite3",
			dir:     "migrations",
		}, nil
	case dialectMySQL:
		return migrationConfig{
			baseFS:  mysqlFS,
			dialect: "mysql",
			dir:     "migrations_mysql",
		}, nil
	default:
		return migrationConfig{}, fmt.Errorf("unsupported database dialect for migrations")
	}
}
