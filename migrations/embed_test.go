package migrations

import "testing"

func TestAllReturnsSortedCopy(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("All() returned no migrations")
	}

	for index := 1; index < len(all); index++ {
		if all[index-1].Version > all[index].Version {
			t.Fatalf("migrations are not sorted: %q before %q", all[index-1].Version, all[index].Version)
		}
	}

	originalVersion := all[0].Version
	all[0].Version = "mutated"
	if got := All()[0].Version; got != originalVersion {
		t.Fatalf("All() exposed mutable migration slice: got %q, want %q", got, originalVersion)
	}
}
