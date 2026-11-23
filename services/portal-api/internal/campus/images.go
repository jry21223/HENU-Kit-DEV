package campus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"

	_ "golang.org/x/image/webp"
)

const (
	maxUploadImageBytes = 2 << 20
	maxStoredImageBytes = 4 << 20
	maxImageSide        = 5000
	maxImagePixels      = 13_000_000
	maxItemImagePixels  = 26_000_000
)

var imageDecodeSlots = make(chan struct{}, 1)

type CampusImageInput struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
}

type SanitizedCampusImage struct {
	ContentType string
	Bytes       []byte
	SHA256      string
}

func sanitizeCampusImage(raw []byte, format string) (SanitizedCampusImage, error) {
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil || decodedFormat != format {
		return SanitizedCampusImage{}, ErrInvalidImage
	}
	if decodedFormat == "jpeg" {
		decoded = applyImageOrientation(decoded, jpegEXIFOrientation(raw))
	}
	cleanContentType, clean, err := encodeSanitizedCampusImage(decoded, decodedFormat)
	if err != nil || len(clean) == 0 {
		return SanitizedCampusImage{}, ErrInvalidImage
	}
	sum := sha256.Sum256(clean)
	return SanitizedCampusImage{
		ContentType: cleanContentType,
		Bytes:       clean,
		SHA256:      hex.EncodeToString(sum[:]),
	}, nil
}

func addImagePixels(width, height int, current int64) (int64, bool) {
	if width < 1 || height < 1 || width > maxImageSide || height > maxImageSide {
		return current, false
	}
	pixels := int64(width) * int64(height)
	if pixels > maxImagePixels || current+pixels > maxItemImagePixels {
		return current, false
	}
	return current + pixels, true
}

type campusImageBuffer struct {
	bytes.Buffer
}

func (b *campusImageBuffer) Write(p []byte) (int, error) {
	if len(p) > maxStoredImageBytes-b.Len() {
		return 0, ErrInvalidImage
	}
	return b.Buffer.Write(p)
}

func encodeSanitizedCampusImage(decoded image.Image, decodedFormat string) (string, []byte, error) {
	var sanitized campusImageBuffer
	opaque := true
	if value, ok := decoded.(interface{ Opaque() bool }); ok {
		opaque = value.Opaque()
	}
	if decodedFormat == "png" || (decodedFormat == "webp" && !opaque) {
		err := png.Encode(&sanitized, decoded)
		return "image/png", sanitized.Bytes(), err
	}
	err := jpeg.Encode(&sanitized, decoded, &jpeg.Options{Quality: 90})
	return "image/jpeg", sanitized.Bytes(), err
}

func jpegEXIFOrientation(raw []byte) int {
	if len(raw) < 4 || raw[0] != 0xff || raw[1] != 0xd8 {
		return 1
	}
	for offset := 2; offset+4 <= len(raw); {
		if raw[offset] != 0xff {
			break
		}
		marker := raw[offset+1]
		offset += 2
		if marker == 0xff {
			offset-- // Marker fill byte. Keep the next 0xff as the marker prefix.
			continue
		}
		if marker == 0xd9 || marker == 0xda {
			break
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		if offset+2 > len(raw) {
			break
		}
		length := int(binary.BigEndian.Uint16(raw[offset : offset+2]))
		if length < 2 || offset+length > len(raw) {
			break
		}
		segment := raw[offset+2 : offset+length]
		if marker == 0xe1 && len(segment) >= 14 && bytes.Equal(segment[:6], []byte("Exif\x00\x00")) {
			if orientation := tiffOrientation(segment[6:]); orientation >= 1 && orientation <= 8 {
				return orientation
			}
		}
		offset += length
	}
	return 1
}

func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 1
	}
	offset := order.Uint32(tiff[4:8])
	if offset < 8 || offset > uint32(len(tiff)-2) {
		return 1
	}
	ifdOffset := int(offset)
	count := int(order.Uint16(tiff[ifdOffset : ifdOffset+2]))
	if count > (len(tiff)-ifdOffset-2)/12 {
		return 1
	}
	for index := 0; index < count; index++ {
		entry := tiff[ifdOffset+2+index*12 : ifdOffset+2+(index+1)*12]
		if order.Uint16(entry[:2]) == 0x0112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
			return int(order.Uint16(entry[8:10]))
		}
	}
	return 1
}

func applyImageOrientation(source image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return source
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	destinationWidth, destinationHeight := width, height
	if orientation >= 5 {
		destinationWidth, destinationHeight = height, width
	}
	destination := image.NewNRGBA(image.Rect(0, 0, destinationWidth, destinationHeight))
	for sourceY := 0; sourceY < height; sourceY++ {
		for sourceX := 0; sourceX < width; sourceX++ {
			destinationX, destinationY := sourceX, sourceY
			switch orientation {
			case 2:
				destinationX = width - 1 - sourceX
			case 3:
				destinationX, destinationY = width-1-sourceX, height-1-sourceY
			case 4:
				destinationY = height - 1 - sourceY
			case 5:
				destinationX, destinationY = sourceY, sourceX
			case 6:
				destinationX, destinationY = height-1-sourceY, sourceX
			case 7:
				destinationX, destinationY = height-1-sourceY, width-1-sourceX
			case 8:
				destinationX, destinationY = sourceY, width-1-sourceX
			}
			destination.Set(destinationX, destinationY, source.At(bounds.Min.X+sourceX, bounds.Min.Y+sourceY))
		}
	}
	return destination
}

var ErrInvalidImage = errors.New("invalid campus image")

func sanitizeCampusImages(ctx context.Context, inputs []CampusImageInput) ([]SanitizedCampusImage, error) {
	if len(inputs) > 6 {
		return nil, ErrInvalidImage
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	select {
	case imageDecodeSlots <- struct{}{}:
		defer func() { <-imageDecodeSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	images := make([]SanitizedCampusImage, 0, len(inputs))
	var pixels int64
	for _, input := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(input.Data) > base64.StdEncoding.EncodedLen(maxUploadImageBytes) {
			return nil, ErrInvalidImage
		}
		raw, err := base64.StdEncoding.Strict().DecodeString(input.Data)
		if err != nil || len(raw) == 0 || len(raw) > maxUploadImageBytes {
			return nil, ErrInvalidImage
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(raw))
		var valid bool
		pixels, valid = addImagePixels(config.Width, config.Height, pixels)
		if err != nil || !valid || "image/"+format != input.ContentType || (format != "jpeg" && format != "png" && format != "webp") {
			return nil, ErrInvalidImage
		}
		clean, err := sanitizeCampusImage(raw, format)
		if err != nil {
			return nil, err
		}
		images = append(images, clean)
	}
	return images, nil
}
