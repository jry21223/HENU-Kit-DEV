package library

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestDecodeSlidesUsesPortalArrayContract(t *testing.T) {
	slides, err := decodeSlidesJSON([]byte(`[{"title":"二重积分","blocks":["定义","例题"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(slides) != 1 || slides[0].Title != "二重积分" || len(slides[0].Blocks) != 2 {
		t.Fatalf("decoded slides = %+v", slides)
	}

	if _, err := decodeSlidesJSON([]byte(`{"slides":[{"title":"wrapped"}]}`)); err == nil {
		t.Fatal("converter wrapper unexpectedly decoded as Portal []Slide")
	}
}

func TestImporterStorageKeyBecomesBrowserMaterialsURLAtTheAPIBoundary(t *testing.T) {
	const storageKey = "高等数学A（二）/复习讲义/考前 复习#1.pdf"
	material := Material{FilePath: publicMaterialFilePath(storageKey)}
	payload, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		FilePath string `json:"filePath"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}

	origin, err := url.Parse("https://henukit.cn")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := url.Parse(decoded.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	got := origin.ResolveReference(reference).String()
	wantPath := (&url.URL{Path: "/materials/" + storageKey}).EscapedPath()
	want := "https://henukit.cn" + wantPath
	if got != want {
		t.Fatalf("browser material URL = %q, want %q", got, want)
	}
	if decoded.FilePath == storageKey {
		t.Fatalf("API filePath leaked the bare importer storage key %q", storageKey)
	}
}
