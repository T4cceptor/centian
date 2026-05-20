package persistence

import (
	"context"
	"fmt"
)

func (s *Store) migrateV6ToV7(ctx context.Context) error {
	if err := createEventAnnotationTables(ctx, s.db); err != nil {
		return fmt.Errorf("create event annotation schema: %w", err)
	}
	return nil
}
