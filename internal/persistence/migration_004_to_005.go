package persistence

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func (s *Store) migrateV4ToV5(ctx context.Context) error {
	steps := []struct {
		name string
		run  func(context.Context, bun.IDB) error
	}{
		{name: "create task run snapshots", run: createTaskRunSnapshotTables},
		{name: "create task run stats", run: createTaskRunStatsTables},
		{name: "create benchmark runs", run: createLegacyBenchmarkRunTablesV5},
		{name: "create benchmark run scores", run: createBenchmarkRunScoreTables},
	}
	for _, step := range steps {
		if err := step.run(ctx, s.db); err != nil {
			return fmt.Errorf("failed to migrate event store schema from v4 to v5: %s: %w", step.name, err)
		}
	}
	return nil
}
