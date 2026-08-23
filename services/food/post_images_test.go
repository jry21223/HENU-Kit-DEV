package food

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func jpegWithOrientationAndGPSMarker(t *testing.T, source image.Image, orientation uint16) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	tiff := make([]byte, 8+2+12+4)
	copy(tiff[:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], orientation)
	payload := append(append([]byte("Exif\x00\x00"), tiff...), []byte("GPSSECRET")...)
	segment := make([]byte, 4+len(payload))
	segment[0], segment[1] = 0xff, 0xe1
	binary.BigEndian.PutUint16(segment[2:4], uint16(len(payload)+2))
	copy(segment[4:], payload)
	original := encoded.Bytes()
	return append(append(append([]byte{}, original[:2]...), segment...), original[2:]...)
}

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

func TestSanitizePostImageAppliesOrientationAndStripsEXIFGPS(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x*50 + y*10), A: 255})
		}
	}
	original := jpegWithOrientationAndGPSMarker(t, source, 6)
	if !bytes.Contains(original, []byte("Exif")) || !bytes.Contains(original, []byte("GPSSECRET")) {
		t.Fatal("test fixture does not contain EXIF/GPS marker")
	}
	sanitized, err := SanitizePostImage("image/jpeg", original)
	if err != nil {
		t.Fatal(err)
	}
	if sanitized.ContentType != "image/jpeg" || sanitized.Width != 3 || sanitized.Height != 2 {
		t.Fatalf("sanitized image = %s %dx%d", sanitized.ContentType, sanitized.Width, sanitized.Height)
	}
	if bytes.Contains(sanitized.Bytes, []byte("Exif")) || bytes.Contains(sanitized.Bytes, []byte("GPSSECRET")) {
		t.Fatal("sanitized image retained EXIF/GPS metadata")
	}
	decoded, err := jpeg.Decode(bytes.NewReader(sanitized.Bytes))
	if err != nil || decoded.Bounds().Dx() != 3 || decoded.Bounds().Dy() != 2 {
		t.Fatalf("sanitized JPEG is not the oriented 3x2 image: %v", err)
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
