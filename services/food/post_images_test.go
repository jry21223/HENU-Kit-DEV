package food

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestApplyImageOrientation(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	for index, value := range []uint8{10, 20, 30, 40, 50, 60} {
		source.SetNRGBA(index%2, index/2, color.NRGBA{R: value, A: 255})
	}
	tests := []struct {
		orientation int
		rows        [][]uint8
	}{
		{1, [][]uint8{{10, 20}, {30, 40}, {50, 60}}},
		{2, [][]uint8{{20, 10}, {40, 30}, {60, 50}}},
		{3, [][]uint8{{60, 50}, {40, 30}, {20, 10}}},
		{4, [][]uint8{{50, 60}, {30, 40}, {10, 20}}},
		{5, [][]uint8{{10, 30, 50}, {20, 40, 60}}},
		{6, [][]uint8{{50, 30, 10}, {60, 40, 20}}},
		{7, [][]uint8{{60, 40, 20}, {50, 30, 10}}},
		{8, [][]uint8{{20, 40, 60}, {10, 30, 50}}},
	}
	for _, test := range tests {
		t.Run(string(rune('0'+test.orientation)), func(t *testing.T) {
			actual := applyImageOrientation(source, test.orientation)
			bounds := actual.Bounds()
			if bounds.Dx() != len(test.rows[0]) || bounds.Dy() != len(test.rows) {
				t.Fatalf("bounds = %v, want %dx%d", bounds, len(test.rows[0]), len(test.rows))
			}
			for y, row := range test.rows {
				for x, expected := range row {
					actualRed, _, _, _ := actual.At(x, y).RGBA()
					if uint8(actualRed>>8) != expected {
						t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, uint8(actualRed>>8), expected)
					}
				}
			}
		})
	}
}

func TestEncodeSanitizedPostImagePreservesTransparentWebPAsPNG(t *testing.T) {
	decoded := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	decoded.SetNRGBA(0, 0, color.NRGBA{R: 12, G: 34, B: 56, A: 80})
	contentType, clean, err := encodeSanitizedPostImage(decoded, "webp")
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/png" {
		t.Fatalf("content type = %q, want image/png", contentType)
	}
	actual, err := png.Decode(bytes.NewReader(clean))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := actual.At(0, 0).RGBA()
	if alpha>>8 != 80 {
		t.Fatalf("alpha = %d, want 80", alpha>>8)
	}
}

func TestAddPostImagePixelsEnforcesIndividualAndRequestBudgets(t *testing.T) {
	if _, ok := addPostImagePixels(maxPostImageSide+1, 1, 0); ok {
		t.Fatal("oversized image side accepted")
	}
	if _, ok := addPostImagePixels(5000, 3000, 0); ok {
		t.Fatal("oversized individual pixel count accepted")
	}
	total, ok := addPostImagePixels(5000, 2600, 0)
	if !ok || total != maxPostImagePixels {
		t.Fatalf("first image total = %d, ok = %v", total, ok)
	}
	if _, ok := addPostImagePixels(5000, 2600, total); !ok {
		t.Fatal("exact request pixel budget rejected")
	}
	if _, ok := addPostImagePixels(1, 1, maxPostTotalPixels); ok {
		t.Fatal("request pixel budget overflow accepted")
	}
}
