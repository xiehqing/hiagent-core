package config

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// table_name: data_config
type DataConfig struct {
	ID         int64  `json:"id"`
	WorkingDir string `json:"workingDir"`
	Config     string `json:"config"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

const dataConfigQuery = `
	SELECT
		id,
		working_dir,
		COALESCE(config, ''),
		created_at,
		updated_at
	FROM data_config
`

// AddDataConfig creates or updates the config for a working directory.
func AddDataConfig(db *sql.DB, data DataConfig) (*DataConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return addDataConfig(ctx, db, data)
}

// GetDataConfig returns a data_config record by id.
func GetDataConfig(db *sql.DB, id int64) (*DataConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return getDataConfig(ctx, db, id)
}

// GetDataConfigByWorkingDir returns the data_config record for a working directory.
func GetDataConfigByWorkingDir(db *sql.DB, workingDir string) (*DataConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return getDataConfigByWorkingDir(ctx, db, workingDir)
}

// GetConfigByWorkingDir returns only the config value for a working directory.
func GetConfigByWorkingDir(db *sql.DB, workingDir string) (string, error) {
	data, err := GetDataConfigByWorkingDir(db, workingDir)
	if err != nil || data == nil {
		return "", err
	}
	return data.Config, nil
}

// DeleteDataConfig deletes a data_config record by id.
func DeleteDataConfig(db *sql.DB, id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return deleteDataConfig(ctx, db, id)
}

// DeleteDataConfigByWorkingDir deletes the data_config record for a working directory.
func DeleteDataConfigByWorkingDir(db *sql.DB, workingDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return deleteDataConfigByWorkingDir(ctx, db, workingDir)
}

func addDataConfig(ctx context.Context, db *sql.DB, data DataConfig) (*DataConfig, error) {
	if db == nil {
		return nil, errors.New("database connection is nil")
	}
	if data.WorkingDir == "" {
		return nil, errors.New("workingDir is required")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	if data.UpdatedAt == 0 {
		data.UpdatedAt = now
	}

	var id int64
	var createdAt int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT id, created_at FROM data_config WHERE working_dir = ? LIMIT 1`,
		data.WorkingDir,
	).Scan(&id, &createdAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if errors.Is(err, sql.ErrNoRows) {
		if data.CreatedAt == 0 {
			data.CreatedAt = now
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO data_config (working_dir, config, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			data.WorkingDir,
			data.Config,
			data.CreatedAt,
			data.UpdatedAt,
		); err != nil {
			return nil, err
		}
	} else {
		if data.CreatedAt == 0 {
			data.CreatedAt = createdAt
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE data_config SET config = ?, created_at = ?, updated_at = ? WHERE id = ?`,
			data.Config,
			data.CreatedAt,
			data.UpdatedAt,
			id,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getDataConfigByWorkingDir(ctx, db, data.WorkingDir)
}

func getDataConfig(ctx context.Context, db *sql.DB, id int64) (*DataConfig, error) {
	if db == nil {
		return nil, errors.New("database connection is nil")
	}
	if id <= 0 {
		return nil, errors.New("id is required")
	}
	row := db.QueryRowContext(ctx, dataConfigQuery+` WHERE id = ? LIMIT 1`, id)
	return scanDataConfig(row)
}

func getDataConfigByWorkingDir(ctx context.Context, db *sql.DB, workingDir string) (*DataConfig, error) {
	if db == nil {
		return nil, errors.New("database connection is nil")
	}
	if workingDir == "" {
		return nil, errors.New("workingDir is required")
	}
	row := db.QueryRowContext(ctx, dataConfigQuery+` WHERE working_dir = ? LIMIT 1`, workingDir)
	return scanDataConfig(row)
}

func deleteDataConfig(ctx context.Context, db *sql.DB, id int64) error {
	if db == nil {
		return errors.New("database connection is nil")
	}
	if id <= 0 {
		return errors.New("id is required")
	}
	_, err := db.ExecContext(ctx, `DELETE FROM data_config WHERE id = ?`, id)
	return err
}

func deleteDataConfigByWorkingDir(ctx context.Context, db *sql.DB, workingDir string) error {
	if db == nil {
		return errors.New("database connection is nil")
	}
	if workingDir == "" {
		return errors.New("workingDir is required")
	}
	_, err := db.ExecContext(ctx, `DELETE FROM data_config WHERE working_dir = ?`, workingDir)
	return err
}

type dataConfigScanner interface {
	Scan(dest ...any) error
}

func scanDataConfig(row dataConfigScanner) (*DataConfig, error) {
	var data DataConfig
	if err := row.Scan(
		&data.ID,
		&data.WorkingDir,
		&data.Config,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan data_config: %w", err)
	}
	return &data, nil
}
