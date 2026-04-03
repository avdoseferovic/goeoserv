package db

import "testing"

func TestCurrentTimestampExpr(t *testing.T) {
	t.Parallel()

	mysqlDB := &Database{driver: "mysql"}
	if got := mysqlDB.CurrentTimestampExpr(); got != "CURRENT_TIMESTAMP" {
		t.Fatalf("mysql current timestamp expr = %q, want %q", got, "CURRENT_TIMESTAMP")
	}

	sqliteDB := &Database{driver: "sqlite"}
	if got := sqliteDB.CurrentTimestampExpr(); got != "datetime('now')" {
		t.Fatalf("sqlite current timestamp expr = %q, want %q", got, "datetime('now')")
	}
}

func TestAddMinutesExpr(t *testing.T) {
	t.Parallel()

	mysqlDB := &Database{driver: "mysql"}
	if got := mysqlDB.AddMinutesExpr("created_at", "duration"); got != "DATE_ADD(created_at, INTERVAL duration MINUTE)" {
		t.Fatalf("mysql add minutes expr = %q", got)
	}

	sqliteDB := &Database{driver: "sqlite"}
	if got := sqliteDB.AddMinutesExpr("created_at", "duration"); got != "datetime(created_at, '+' || duration || ' minutes')" {
		t.Fatalf("sqlite add minutes expr = %q", got)
	}
}

func TestAdditiveUpsertClause(t *testing.T) {
	t.Parallel()

	mysqlDB := &Database{driver: "mysql"}
	if got := mysqlDB.AdditiveUpsertClause([]string{"character_id", "item_id"}, "quantity"); got != "ON DUPLICATE KEY UPDATE quantity = quantity + VALUES(quantity)" {
		t.Fatalf("mysql additive upsert clause = %q", got)
	}

	sqliteDB := &Database{driver: "sqlite"}
	if got := sqliteDB.AdditiveUpsertClause([]string{"character_id", "item_id"}, "quantity"); got != "ON CONFLICT(character_id, item_id) DO UPDATE SET quantity = quantity + excluded.quantity" {
		t.Fatalf("sqlite additive upsert clause = %q", got)
	}
}
