package db

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteSQLMySQL(t *testing.T) {
	t.Parallel()

	query := `SELECT
    CAST(strftime('%w', created_at, 'unixepoch') AS INTEGER) as day_of_week,
    CAST(strftime('%H', created_at, 'unixepoch') AS INTEGER) as hour
FROM sessions
WHERE created_at >= strftime('%s', 'now', '-30 days')`

	rewritten := rewriteSQL(query, dialectMySQL)

	require.Contains(t, rewritten, "DAYOFWEEK(FROM_UNIXTIME(created_at))")
	require.Contains(t, rewritten, "HOUR(FROM_UNIXTIME(created_at))")
	require.Contains(t, rewritten, "DATE_SUB(NOW(), INTERVAL 30 DAY)")
	require.NotContains(t, rewritten, "strftime")
}

func TestRewriteSQLMySQLRemovesReturning(t *testing.T) {
	t.Parallel()

	query := `INSERT INTO sessions (id) VALUES (?) RETURNING id, parent_session_id, title, message_count, prompt_tokens, completion_tokens, cost, updated_at, created_at, summary_message_id, todos`

	rewritten := rewriteSQL(query, dialectMySQL)

	require.NotContains(t, rewritten, "RETURNING")
	require.True(t, strings.Contains(rewritten, "INSERT INTO sessions"))
}

func TestRewriteSQLMySQLJSONTable(t *testing.T) {
	t.Parallel()

	query := `SELECT
    json_extract(value, '$.data.name') as tool_name,
    COUNT(*) as call_count
FROM messages, json_each(parts)
WHERE json_extract(value, '$.type') = 'tool_call'
  AND json_extract(value, '$.data.name') IS NOT NULL
GROUP BY tool_name`

	rewritten := rewriteSQL(query, dialectMySQL)

	require.Contains(t, rewritten, "JSON_TABLE")
	require.Contains(t, rewritten, "jt.tool_name")
	require.NotContains(t, rewritten, "json_each")
}
