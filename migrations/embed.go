package migrations

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"sort"
)

type Migration struct {
	Version string
	SQL     string
}

//go:embed 000_schema_migrations.sql
var schemaMigrationsSQL string

var migrations = []Migration{
	{
		Version: "001_initial.sql",
		SQL:     initialSQL,
	},
	{
		Version: "002_reminder_delivery_retries.sql",
		SQL:     reminderDeliveryRetriesSQL,
	},
	{
		Version: "003_runtime_hardening.sql",
		SQL:     runtimeHardeningSQL,
	},
	{
		Version: "004_delivery_queue_retention.sql",
		SQL:     deliveryQueueRetentionSQL,
	},
}

//go:embed 001_initial.sql
var initialSQL string

//go:embed 002_reminder_delivery_retries.sql
var reminderDeliveryRetriesSQL string

//go:embed 003_runtime_hardening.sql
var runtimeHardeningSQL string

//go:embed 004_delivery_queue_retention.sql
var deliveryQueueRetentionSQL string

func (m Migration) Checksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(m.SQL)))
}

func All() []Migration {
	ret := make([]Migration, len(migrations))
	copy(ret, migrations)
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Version < ret[j].Version
	})
	return ret
}

func SchemaMigrationsSQL() string {
	return schemaMigrationsSQL
}
