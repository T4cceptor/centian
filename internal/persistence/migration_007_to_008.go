package persistence

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *Store) migrateV7ToV8(ctx context.Context) error {
	columns := []struct {
		name string
		stmt string
	}{
		{name: "type", stmt: `ALTER TABLE event_annotations ADD COLUMN type TEXT`},
		{name: "category", stmt: `ALTER TABLE event_annotations ADD COLUMN category TEXT`},
	}
	for _, column := range columns {
		exists, err := sqliteColumnExists(ctx, s.db, "event_annotations", column.name)
		if err != nil {
			return fmt.Errorf("inspect event annotation columns: %w", err)
		}
		if exists {
			continue
		}
		if _, err := s.db.ExecContext(ctx, column.stmt); err != nil {
			return fmt.Errorf("migrate event annotations to v8: %w", err)
		}
	}
	return nil
}

type sqlQueryExecutor interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

func sqliteColumnExists(ctx context.Context, db sqlQueryExecutor, tableName, columnName string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return false, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}
