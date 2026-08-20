package campus

import (
	"database/sql"
	"testing"

	"henukit.dev/portal-api/internal/dbtest"
)

func insertItem(t *testing.T, conn *sql.DB, id, itemType, category, title, desc, status string) {
	t.Helper()
	_, err := conn.Exec(`
		INSERT INTO portal_campus_items
			(id, type, category, title, "desc", price, seller, credit,
			 deals_done, wants, place, deadline, status, time, images)
		VALUES ($1, $2, $3, $4, $5, 10, 'seller', 80, 0, 0, 'place', NULL, $6, '', '[]'::jsonb)
	`, id, itemType, category, title, desc, status)
	if err != nil {
		t.Fatalf("insert item %s: %v", id, err)
	}
}

func ids(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func TestGetItemsExcludesHiddenItems(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)

	insertItem(t, conn, "open-1", "sell", "book", "Textbook", "", "open")
	insertItem(t, conn, "ongoing-1", "sell", "book", "Notes", "", "ongoing")
	insertItem(t, conn, "done-1", "sell", "book", "Lamp", "", "done")
	insertItem(t, conn, "hidden-1", "sell", "book", "Removed", "", "hidden")

	items, err := db.GetItems("", "", "")
	if err != nil {
		t.Fatalf("GetItems(): %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("GetItems() = %v, want the 3 non-hidden items", ids(items))
	}
	for _, item := range items {
		if item.Status == "hidden" {
			t.Errorf("GetItems() returned hidden item %s", item.ID)
		}
	}
}

func TestGetItemsFiltersCombineAsConjunction(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)

	insertItem(t, conn, "sell-book", "sell", "book", "Textbook", "", "open")
	insertItem(t, conn, "sell-bike", "sell", "bike", "Bicycle", "", "open")
	insertItem(t, conn, "help-book", "help", "book", "Textbook wanted", "", "open")

	byType, err := db.GetItems("sell", "", "")
	if err != nil {
		t.Fatalf("GetItems(sell): %v", err)
	}
	if len(byType) != 2 {
		t.Fatalf("GetItems(sell) = %v, want both sell items", ids(byType))
	}

	byCategory, err := db.GetItems("", "book", "")
	if err != nil {
		t.Fatalf("GetItems(category=book): %v", err)
	}
	if len(byCategory) != 2 {
		t.Fatalf("GetItems(category=book) = %v, want both book items", ids(byCategory))
	}

	both, err := db.GetItems("sell", "book", "")
	if err != nil {
		t.Fatalf("GetItems(sell, book): %v", err)
	}
	if len(both) != 1 || both[0].ID != "sell-book" {
		t.Fatalf("GetItems(sell, book) = %v, want only sell-book", ids(both))
	}
}

// The search term is bound as a parameter, so ILIKE wildcards a user types are
// matched literally and cannot widen the query.
func TestGetItemsSearchMatchesTitleAndDescriptionCaseInsensitively(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)

	insertItem(t, conn, "by-title", "sell", "book", "Calculus Textbook", "clean copy", "open")
	insertItem(t, conn, "by-desc", "sell", "bike", "Bicycle", "comes with a calculus poster", "open")
	insertItem(t, conn, "no-match", "sell", "misc", "Kettle", "boils water", "open")

	matches, err := db.GetItems("", "", "CALCULUS")
	if err != nil {
		t.Fatalf("GetItems(q=CALCULUS): %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("GetItems(q=CALCULUS) = %v, want the title and description matches", ids(matches))
	}

}

// The search term is bound as a parameter, so it cannot inject SQL — but it is
// not escaped for LIKE, so a typed % or _ is still read as a pattern. This test
// pins that behavior rather than endorsing it: escaping the metacharacters is a
// deliberate product decision about what the search box promises, and whoever
// makes it should have to update this test.
func TestGetItemsSearchTreatsTypedWildcardsAsPatterns(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)

	insertItem(t, conn, "literal", "sell", "book", "note_book", "", "open")
	insertItem(t, conn, "pattern", "sell", "book", "notexbook", "", "open")
	insertItem(t, conn, "unrelated", "sell", "bike", "Bicycle", "", "open")

	// Searching "note_book" matches "notexbook" too, because the underscore
	// reaches ILIKE as a single-character wildcard instead of a literal.
	matches, err := db.GetItems("", "", "note_book")
	if err != nil {
		t.Fatalf("GetItems(q=note_book): %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("GetItems(q=note_book) = %v, want both titles: _ is a wildcard today", ids(matches))
	}

	// A bare %% matches every row for the same reason.
	wildcard, err := db.GetItems("", "", "%")
	if err != nil {
		t.Fatalf("GetItems(q=%%): %v", err)
	}
	if len(wildcard) != 3 {
		t.Fatalf("GetItems(q=%%) = %v, want every row", ids(wildcard))
	}
}

func TestGetItemsOnEmptyTableReturnsEmptyList(t *testing.T) {
	db := NewPortalDB(dbtest.Open(t))

	items, err := db.GetItems("", "", "")
	if err != nil {
		t.Fatalf("GetItems(): %v", err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("GetItems() = %+v, want an empty slice", items)
	}
}

func TestGetItemReturnsNilForUnknownID(t *testing.T) {
	db := NewPortalDB(dbtest.Open(t))

	item, err := db.GetItem("does-not-exist")
	if err != nil {
		t.Fatalf("GetItem(): unexpected error %v", err)
	}
	if item != nil {
		t.Fatalf("GetItem(unknown) = %+v, want nil", item)
	}
}

// A hidden item is filtered from the list but still readable by ID.
func TestGetItemReturnsHiddenItemByID(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)
	insertItem(t, conn, "hidden-1", "sell", "book", "Removed", "", "hidden")

	item, err := db.GetItem("hidden-1")
	if err != nil {
		t.Fatalf("GetItem(): %v", err)
	}
	if item == nil || item.Status != "hidden" {
		t.Fatalf("GetItem(hidden-1) = %+v, want the hidden item", item)
	}
}

func TestGetMessagesIsScopedToItsItem(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)
	insertItem(t, conn, "item-1", "sell", "book", "Textbook", "", "open")
	insertItem(t, conn, "item-2", "sell", "book", "Notes", "", "open")

	if _, err := conn.Exec(`
		INSERT INTO portal_campus_messages (id, item_id, author, time, text)
		VALUES ('m-1', 'item-1', 'alice', '', 'is it available'),
		       ('m-2', 'item-2', 'bob', '', 'other item')
	`); err != nil {
		t.Fatalf("insert messages: %v", err)
	}

	messages, err := db.GetMessages("item-1")
	if err != nil {
		t.Fatalf("GetMessages(): %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "m-1" {
		t.Fatalf("GetMessages(item-1) = %+v, want only m-1", messages)
	}
}

func TestGetMessagesOnEmptyItemReturnsEmptyList(t *testing.T) {
	db := NewPortalDB(dbtest.Open(t))

	messages, err := db.GetMessages("item-1")
	if err != nil {
		t.Fatalf("GetMessages(): %v", err)
	}
	if messages == nil || len(messages) != 0 {
		t.Fatalf("GetMessages() = %+v, want an empty slice", messages)
	}
}

// A dead pool must surface as an error, never as an empty-but-successful read —
// the same "never serve incomplete data as success" rule the empty-list tests
// above cover from the other direction.
//
// Note on scope: this exercises the Query() failure path, not the mid-iteration
// one that rows.Err() guards. A fault landing *between* rows (backend
// termination or statement_timeout while the result streams) is not reachable
// from a test today: these methods take no context.Context, and SET
// statement_timeout does not follow a pooled *sql.DB to the connection the next
// Query borrows. The rows.Err() checks are therefore held by the rowserrcheck
// linter rather than by a test — see .golangci.yml.
func TestGetItemsReportsADeadPoolInsteadOfAnEmptyRead(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)

	insertItem(t, conn, "item-1", "sell", "book", "Textbook", "", "open")

	// Confirm the healthy read first, so the failure below is unambiguously the
	// injected one rather than an empty table.
	healthy, err := db.GetItems("", "", "")
	if err != nil {
		t.Fatalf("GetItems(): %v", err)
	}
	if len(healthy) != 1 {
		t.Fatalf("GetItems() = %d items, want 1 before the fault is injected", len(healthy))
	}

	conn.Close()

	items, err := db.GetItems("", "", "")
	if err == nil {
		t.Fatalf("GetItems() on a closed pool = %d items, nil error; "+
			"an unreadable table must not be reported as an empty success", len(items))
	}
	if items != nil {
		t.Errorf("GetItems() returned %d items alongside an error, want nil", len(items))
	}
}
