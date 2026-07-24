package tools

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func SQLQuery(driver, dsn, query string) string {
	if driver == "" {
		return "[Error] driver is required (sqlite, postgres, mysql)"
	}
	if dsn == "" {
		return "[Error] dsn (connection string) is required"
	}
	if query == "" {
		return "[Error] query is required"
	}

	switch driver {
	case "sqlite":
	case "postgres":
		return "[Error] PostgreSQL driver not installed yet. Use sqlite instead."
	case "mysql":
		return "[Error] MySQL driver not installed yet. Use sqlite instead."
	default:
		return fmt.Sprintf("[Error] unsupported driver: %s (supported: sqlite)", driver)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return fmt.Sprintf("[Error] failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Sprintf("[Error] failed to connect to database: %v", err)
	}

	query = strings.TrimSpace(query)
	upperQuery := strings.ToUpper(query)

	if strings.HasPrefix(upperQuery, "SELECT") || strings.HasPrefix(upperQuery, "PRAGMA") || strings.HasPrefix(upperQuery, "SHOW") {
		return querySelect(db, query)
	}

	return queryExec(db, query)
}

func querySelect(db *sql.DB, query string) string {
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Sprintf("[Error] query failed: %v", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Sprintf("[Error] failed to get columns: %v", err)
	}

	var results []string
	results = append(results, strings.Join(columns, " | "))
	results = append(results, strings.Repeat("-", len(strings.Join(columns, " | "))))

	count := 0
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Sprintf("[Error] failed to scan row: %v", err)
		}

		var row []string
		for _, v := range values {
			if v.Valid {
				row = append(row, v.String)
			} else {
				row = append(row, "NULL")
			}
		}
		results = append(results, strings.Join(row, " | "))
		count++

		if count >= 100 {
			results = append(results, "... (truncated at 100 rows)")
			break
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Sprintf("[Error] row error: %v", err)
	}

	if count == 0 {
		return "Query returned 0 rows."
	}

	return fmt.Sprintf("%d rows returned:\n%s", count, strings.Join(results, "\n"))
}

func queryExec(db *sql.DB, query string) string {
	result, err := db.Exec(query)
	if err != nil {
		return fmt.Sprintf("[Error] execution failed: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	return fmt.Sprintf("Query executed successfully. Rows affected: %d", rowsAffected)
}
