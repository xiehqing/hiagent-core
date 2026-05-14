package db

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type dialect string

const (
	dialectUnknown dialect = ""
	dialectSQLite  dialect = "sqlite"
	dialectMySQL   dialect = "mysql"
)

func detectDialect(db DBTX) dialect {
	driverProvider, ok := db.(interface{ Driver() driver.Driver })
	if !ok {
		return dialectUnknown
	}

	name := strings.ToLower(fmt.Sprintf("%T", driverProvider.Driver()))
	switch {
	case strings.Contains(name, "mysql"):
		return dialectMySQL
	case strings.Contains(name, "sqlite"):
		return dialectSQLite
	default:
		return dialectUnknown
	}
}

func rewriteSQL(query string, dialect dialect) string {
	if dialect != dialectMySQL {
		return query
	}

	replacer := strings.NewReplacer(
		"strftime('%s', 'now')", "UNIX_TIMESTAMP()",
		"date(created_at, 'unixepoch')", "DATE(FROM_UNIXTIME(created_at))",
		"CAST(strftime('%H', created_at, 'unixepoch') AS INTEGER)", "HOUR(FROM_UNIXTIME(created_at))",
		"CAST(strftime('%w', created_at, 'unixepoch') AS INTEGER)", "((DAYOFWEEK(FROM_UNIXTIME(created_at)) + 5) % 7)",
		"created_at >= strftime('%s', 'now', '-30 days')", "created_at >= UNIX_TIMESTAMP(DATE_SUB(NOW(), INTERVAL 30 DAY))",
		"ON CONFLICT(path, session_id) DO UPDATE SET\n    read_at = excluded.read_at", "ON DUPLICATE KEY UPDATE read_at = VALUES(read_at)",
		"json_extract(value, '$.data.name') as tool_name", "jt.tool_name as tool_name",
		"FROM messages, json_each(parts)\nWHERE json_extract(value, '$.type') = 'tool_call'\n  AND json_extract(value, '$.data.name') IS NOT NULL", "FROM messages,\nJSON_TABLE(CAST(parts AS JSON), '$[*]' COLUMNS (\n    part_type VARCHAR(64) PATH '$.type',\n    tool_name VARCHAR(255) PATH '$.data.name'\n)) jt\nWHERE jt.part_type = 'tool_call'\n  AND jt.tool_name IS NOT NULL",
	)
	query = replacer.Replace(query)

	query = strings.ReplaceAll(query, "\nRETURNING id, session_id, path, content, version, created_at, updated_at", "")
	query = strings.ReplaceAll(query, " RETURNING id, session_id, path, content, version, created_at, updated_at", "")
	query = strings.ReplaceAll(query, "\nRETURNING id, session_id, role, parts, model, created_at, updated_at, finished_at, provider, is_summary_message", "")
	query = strings.ReplaceAll(query, " RETURNING id, session_id, role, parts, model, created_at, updated_at, finished_at, provider, is_summary_message", "")
	query = strings.ReplaceAll(query, "\nRETURNING id, parent_session_id, title, message_count, prompt_tokens, completion_tokens, cost, updated_at, created_at, summary_message_id, todos", "")
	query = strings.ReplaceAll(query, " RETURNING id, parent_session_id, title, message_count, prompt_tokens, completion_tokens, cost, updated_at, created_at, summary_message_id, todos", "")

	return query
}
