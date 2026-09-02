package migrations

import (
	_ "embed"
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
}

//go:embed 001_initial.sql
var initialSQL string

//go:embed 002_reminder_delivery_retries.sql
var reminderDeliveryRetriesSQL string

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
