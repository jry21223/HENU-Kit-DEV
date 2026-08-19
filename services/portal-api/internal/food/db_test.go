package food

import (
	"database/sql"
	"testing"

	"henukit.dev/portal-api/internal/dbtest"
)

func insertPost(t *testing.T, conn *sql.DB, id, campus, shop string, stars int, hidden bool) {
	t.Helper()
	_, err := conn.Exec(`
		INSERT INTO portal_food_posts
			(id, campus, title, excerpt, blocks, author, likes, stars, tags,
			 shop_name, shop_lat, shop_lng, time, hidden, images)
		VALUES ($1, $2, $3, '', '[]'::jsonb, 'author', 0, $4, '["spicy"]'::jsonb,
		        $5, 0, 0, '', $6, '["/legacy.jpg"]'::jsonb)
	`, id, campus, "title-"+id, stars, shop, hidden)
	if err != nil {
		t.Fatalf("insert post %s: %v", id, err)
	}
}

func TestGetPostsExcludesHiddenAndFiltersByCampus(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)

	insertPost(t, conn, "p-minglun", "minglun", "Shop A", 4, false)
	insertPost(t, conn, "p-jinming", "jinming", "Shop B", 4, false)
	insertPost(t, conn, "p-hidden", "minglun", "Shop C", 4, true)

	all, err := db.GetPosts("")
	if err != nil {
		t.Fatalf("GetPosts(): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("GetPosts() returned %d posts, want the 2 visible ones", len(all))
	}
	for _, post := range all {
		if post.ID == "p-hidden" {
			t.Error("GetPosts() returned a hidden post")
		}
	}

	filtered, err := db.GetPosts("minglun")
	if err != nil {
		t.Fatalf("GetPosts(minglun): %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "p-minglun" {
		t.Fatalf("GetPosts(minglun) = %+v, want only p-minglun", filtered)
	}
}

// An empty table must read as an empty list rather than a nil slice, so the
// handler serves 200 with [] and never falls back to mock posts.
func TestGetPostsOnEmptyTableReturnsEmptyList(t *testing.T) {
	db := NewPortalDB(dbtest.Open(t))

	posts, err := db.GetPosts("")
	if err != nil {
		t.Fatalf("GetPosts(): %v", err)
	}
	if posts == nil {
		t.Fatal("GetPosts() = nil, want an empty slice")
	}
	if len(posts) != 0 {
		t.Fatalf("GetPosts() = %+v, want empty", posts)
	}
}

func TestGetPostReturnsNilForUnknownID(t *testing.T) {
	db := NewPortalDB(dbtest.Open(t))

	post, err := db.GetPost("does-not-exist")
	if err != nil {
		t.Fatalf("GetPost(): unexpected error %v", err)
	}
	if post != nil {
		t.Fatalf("GetPost(unknown) = %+v, want nil", post)
	}
}

func TestGetPostDecodesJSONColumns(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)
	insertPost(t, conn, "p-1", "longzihu", "Shop A", 5, false)

	post, err := db.GetPost("p-1")
	if err != nil {
		t.Fatalf("GetPost(): %v", err)
	}
	if post == nil {
		t.Fatal("GetPost(p-1) = nil, want the inserted post")
	}
	if post.Shop.Name != "Shop A" || post.Campus != "longzihu" {
		t.Errorf("GetPost() shop/campus = %q/%q", post.Shop.Name, post.Campus)
	}
	if len(post.Tags) != 1 || post.Tags[0] != "spicy" {
		t.Errorf("GetPost() tags = %+v, want [spicy]", post.Tags)
	}
}

// A hidden post is still addressable by ID; only the list view filters it.
func TestGetPostReturnsHiddenPostByID(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)
	insertPost(t, conn, "p-hidden", "minglun", "Shop C", 3, true)

	post, err := db.GetPost("p-hidden")
	if err != nil {
		t.Fatalf("GetPost(): %v", err)
	}
	if post == nil || !post.Hidden {
		t.Fatalf("GetPost(p-hidden) = %+v, want the hidden post", post)
	}
}

// Stored photos replace the images column; posts without stored photos keep
// whatever the column held, so externally hosted photos survive.
func TestStoredImagesReplaceOnlyPostsThatHaveThem(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)
	insertPost(t, conn, "p-stored", "minglun", "Shop A", 4, false)
	insertPost(t, conn, "p-external", "minglun", "Shop B", 4, false)

	if _, err := conn.Exec(`
		INSERT INTO portal_food_post_images
			(id, post_id, position, content_type, byte_size, sha256, bytes)
		VALUES ('img-0', 'p-stored', 0, 'image/jpeg', 1, repeat('a', 64), '\x00'::bytea),
		       ('img-1', 'p-stored', 1, 'image/jpeg', 1, repeat('b', 64), '\x01'::bytea)
	`); err != nil {
		t.Fatalf("insert stored images: %v", err)
	}

	stored, err := db.GetPost("p-stored")
	if err != nil {
		t.Fatalf("GetPost(p-stored): %v", err)
	}
	want := []string{ImagePath("p-stored", 0), ImagePath("p-stored", 1)}
	if len(stored.Images) != len(want) {
		t.Fatalf("stored post images = %+v, want %+v", stored.Images, want)
	}
	for i, path := range want {
		if stored.Images[i] != path {
			t.Errorf("image %d = %q, want %q", i, stored.Images[i], path)
		}
	}

	external, err := db.GetPost("p-external")
	if err != nil {
		t.Fatalf("GetPost(p-external): %v", err)
	}
	if len(external.Images) != 1 || external.Images[0] != "/legacy.jpg" {
		t.Errorf("external post images = %+v, want the untouched column value", external.Images)
	}
}

func TestGetVenuesGroupsByShopAndSkipsHiddenOrUnnamed(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)

	insertPost(t, conn, "a-1", "minglun", "Shop A", 4, false)
	insertPost(t, conn, "a-2", "minglun", "Shop A", 6, false)
	insertPost(t, conn, "b-1", "minglun", "Shop B", 3, false)
	// A hidden post and a post with no shop name must not produce a venue.
	insertPost(t, conn, "c-1", "minglun", "Shop C", 5, true)
	insertPost(t, conn, "d-1", "minglun", "", 5, false)

	venues, err := db.GetVenues("")
	if err != nil {
		t.Fatalf("GetVenues(): %v", err)
	}
	if len(venues) != 2 {
		t.Fatalf("GetVenues() = %+v, want only Shop A and Shop B", venues)
	}

	rating := map[string]float64{}
	for _, venue := range venues {
		rating[venue.Name] = venue.Rating
	}
	// Two posts at 4 and 6 average to 5: the query aggregates rather than
	// returning one venue per post.
	if got := rating["Shop A"]; got != 5 {
		t.Errorf("Shop A rating = %v, want the 5 average of its two posts", got)
	}
	if got := rating["Shop B"]; got != 3 {
		t.Errorf("Shop B rating = %v, want 3", got)
	}
}

func TestGetVenuesFiltersByCampus(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)
	insertPost(t, conn, "a-1", "minglun", "Shop A", 4, false)
	insertPost(t, conn, "b-1", "jinming", "Shop B", 4, false)

	venues, err := db.GetVenues("jinming")
	if err != nil {
		t.Fatalf("GetVenues(jinming): %v", err)
	}
	if len(venues) != 1 || venues[0].Name != "Shop B" {
		t.Fatalf("GetVenues(jinming) = %+v, want only Shop B", venues)
	}
}

func TestGetVenuesOnEmptyTableReturnsEmptyList(t *testing.T) {
	db := NewPortalDB(dbtest.Open(t))

	venues, err := db.GetVenues("")
	if err != nil {
		t.Fatalf("GetVenues(): %v", err)
	}
	if venues == nil || len(venues) != 0 {
		t.Fatalf("GetVenues() = %+v, want an empty slice", venues)
	}
}

func TestGetCommentsIsScopedToItsPost(t *testing.T) {
	conn := dbtest.Open(t)
	db := NewPortalDB(conn)
	insertPost(t, conn, "p-1", "minglun", "Shop A", 4, false)
	insertPost(t, conn, "p-2", "minglun", "Shop B", 4, false)

	if _, err := conn.Exec(`
		INSERT INTO portal_food_comments (id, post_id, author, time, text)
		VALUES ('c-1', 'p-1', 'alice', '', 'first'),
		       ('c-2', 'p-2', 'bob', '', 'other post')
	`); err != nil {
		t.Fatalf("insert comments: %v", err)
	}

	comments, err := db.GetComments("p-1")
	if err != nil {
		t.Fatalf("GetComments(): %v", err)
	}
	if len(comments) != 1 || comments[0].ID != "c-1" {
		t.Fatalf("GetComments(p-1) = %+v, want only c-1", comments)
	}
}
