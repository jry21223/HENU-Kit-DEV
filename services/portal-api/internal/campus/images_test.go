package campus

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestCampusImagesSanitizeFormatsAndJPEGMetadata(t *testing.T) {
	var photo bytes.Buffer
	if err := jpeg.Encode(&photo, image.NewGray(image.Rect(0, 0, 3, 2)), nil); err != nil {
		t.Fatal(err)
	}

	// A legal JPEG marker fill byte precedes APP1. EXIF specifies 90° CW.
	exif := []byte("Exif\x00\x00MM\x00\x2a\x00\x00\x00\x08\x00\x01\x01\x12\x00\x03\x00\x00\x00\x01\x00\x06\x00\x00\x00\x00\x00\x00")
	xmp := []byte("http://ns.adobe.com/xap/1.0/\x00GPSLatitude=34.8")
	rawJPEG := append([]byte{}, photo.Bytes()[:2]...)
	for _, metadata := range [][]byte{xmp, exif} {
		rawJPEG = append(rawJPEG, 0xff, 0xff, 0xe1, 0, byte(len(metadata)+2))
		rawJPEG = append(rawJPEG, metadata...)
	}
	rawJPEG = append(rawJPEG, photo.Bytes()[2:]...)

	// A 1×1 lossy WebP fixture.
	rawWebP, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	inputs := []CampusImageInput{
		{ContentType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(rawJPEG)},
		{ContentType: "image/png", Data: base64.StdEncoding.EncodeToString(testCampusPNG(t, 2, 3))},
		{ContentType: "image/webp", Data: base64.StdEncoding.EncodeToString(rawWebP)},
	}

	images, err := sanitizeCampusImages(t.Context(), inputs)
	if err != nil || len(images) != len(inputs) {
		t.Fatalf("sanitizing images = %d images, err = %v", len(images), err)
	}
	for i, img := range images {
		wantWidth, wantHeight, wantType := 2, 3, "image/jpeg"
		switch i {
		case 1:
			wantType = "image/png"
		case 2:
			wantWidth, wantHeight = 1, 1
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(img.Bytes))
		if err != nil || config.Width != wantWidth || config.Height != wantHeight || "image/"+format != wantType || img.ContentType != wantType {
			t.Errorf("image %d = %s %v %v, want %s %dx%d", i, img.ContentType, config, err, wantType, wantWidth, wantHeight)
		}
		if want := fmt.Sprintf("%x", sha256.Sum256(img.Bytes)); img.SHA256 != want {
			t.Errorf("image %d digest = %q, want %q", i, img.SHA256, want)
		}
	}
	if bytes.Contains(images[0].Bytes, []byte("Exif")) || bytes.Contains(images[0].Bytes, []byte("GPSLatitude")) || bytes.Contains(images[0].Bytes, []byte("ns.adobe.com")) {
		t.Fatal("JPEG retained EXIF or XMP metadata")
	}
}

func TestCampusImagesRejectInvalidUploads(t *testing.T) {
	pngInput := func(raw []byte) CampusImageInput {
		return CampusImageInput{ContentType: "image/png", Data: base64.StdEncoding.EncodeToString(raw)}
	}
	valid := pngInput(testCampusPNG(t, 1, 1))
	// 2 MiB and 2 MiB + 1 have the same base64 length. Check the decoded limit.
	rawPNG := testCampusPNG(t, 1, 1)
	oversize := pngInput(append(rawPNG, make([]byte, (2<<20)+1-len(rawPNG))...))
	tooMany := []CampusImageInput{valid, valid, valid, valid, valid, valid, valid}
	large := pngInput(testCampusPNG(t, 3600, 2500))
	for _, tc := range []struct {
		name   string
		inputs []CampusImageInput
	}{
		{"more than six images", tooMany},
		{"over 2 MiB", []CampusImageInput{oversize}},
		{"includes data prefix", []CampusImageInput{{ContentType: valid.ContentType, Data: "data:image/png;base64," + valid.Data}}},
		{"invalid base64", []CampusImageInput{{ContentType: valid.ContentType, Data: "!!!"}}},
		{"wrong media type", []CampusImageInput{{ContentType: "image/jpeg", Data: valid.Data}}},
		{"unsupported media type", []CampusImageInput{{ContentType: "image/gif", Data: valid.Data}}},
		{"truncated image", []CampusImageInput{pngInput(testCampusPNG(t, 1, 1)[:33])}},
		{"side over 5000", []CampusImageInput{pngInput(testCampusPNG(t, 5001, 1))}},
		{"over 13 million pixels", []CampusImageInput{pngInput(testCampusPNG(t, 4000, 3251))}},
		{"over 26 million pixels in aggregate", []CampusImageInput{large, large, large}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if images, err := sanitizeCampusImages(t.Context(), tc.inputs); !errors.Is(err, ErrInvalidImage) || len(images) != 0 {
				t.Fatalf("got %d images, err = %v, want ErrInvalidImage", len(images), err)
			}
		})
	}
}

func testCampusPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&buf, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
