package config

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/require"
)

func openConfigTestDB(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	_, err = conn.Exec(`
CREATE TABLE data_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    working_dir TEXT NOT NULL UNIQUE,
    config TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
`)
	require.NoError(t, err)

	return conn
}

func TestLoadFromDBMergesGlobalAndWorkspace(t *testing.T) {
	t.Parallel()

	conn := openConfigTestDB(t)
	workingDir := t.TempDir()
	workspaceKey, err := workspaceConfigKeyForDir(workingDir)
	require.NoError(t, err)

	_, err = AddDataConfig(conn, DataConfig{
		WorkingDir: globalConfigKey,
		Config:     `{"options":{"debug":false,"disabled_tools":["bash"]}}`,
		CreatedAt:  10,
		UpdatedAt:  10,
	})
	require.NoError(t, err)

	_, err = AddDataConfig(conn, DataConfig{
		WorkingDir: workspaceKey,
		Config:     `{"options":{"debug":true}}`,
		CreatedAt:  20,
		UpdatedAt:  20,
	})
	require.NoError(t, err)

	store, err := Load(workingDir, "", conn, false)
	require.NoError(t, err)

	require.True(t, store.Config().Options.Debug)
	require.Equal(t, []string{"bash"}, store.Config().Options.DisabledTools)
	require.Equal(t, []string{globalConfigKey, workspaceKey}, store.LoadedPaths())
}

func TestSetConfigFieldPersistsWorkspaceRecord(t *testing.T) {
	t.Parallel()

	conn := openConfigTestDB(t)
	workingDir := t.TempDir()

	store, err := Load(workingDir, "", conn, false)
	require.NoError(t, err)

	err = store.SetConfigField(ScopeWorkspace, "options.debug", true)
	require.NoError(t, err)

	workspaceKey, err := workspaceConfigKeyForDir(workingDir)
	require.NoError(t, err)
	record, err := GetDataConfigByWorkingDir(conn, workspaceKey)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.JSONEq(t, `{"options":{"debug":true}}`, record.Config)
}

func TestConfigStalenessUsesDBUpdatedAt(t *testing.T) {
	t.Parallel()

	conn := openConfigTestDB(t)
	workingDir := t.TempDir()

	_, err := AddDataConfig(conn, DataConfig{
		WorkingDir: globalConfigKey,
		Config:     `{"options":{"debug":false}}`,
		CreatedAt:  100,
		UpdatedAt:  100,
	})
	require.NoError(t, err)

	store, err := Load(workingDir, "", conn, false)
	require.NoError(t, err)

	clean := store.ConfigStaleness()
	require.False(t, clean.Dirty)

	_, err = AddDataConfig(conn, DataConfig{
		WorkingDir: globalConfigKey,
		Config:     `{"options":{"debug":true}}`,
		CreatedAt:  100,
		UpdatedAt:  200,
	})
	require.NoError(t, err)

	dirty := store.ConfigStaleness()
	require.True(t, dirty.Dirty)
	require.Contains(t, dirty.Changed, globalConfigKey)
}
