package food

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"

	_ "golang.org/x/image/webp"
)

const (
	maxPostImageBytes   = 2 << 20
	maxStoredImageBytes = 4 << 20
	maxPostImageSide    = 5000
	maxPostImagePixels  = 13_000_000
	maxPostTotalPixels  = 26_000_000
)

var imageDecodeSlots = make(chan struct{}, 1)

type postImageInput struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
}

type decodedPostImage struct {
	ContentType string
	Bytes       []byte
	SHA256      string
}

func decodePostImages(w http.ResponseWriter, r *http.Request, inputs []postImageInput) ([]decodedPostImage, bool) {
	if len(inputs) > maxPostImages {
		writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image count exceeds the limit")
		return nil, false
	}
	if len(inputs) > 0 {
		select {
		case imageDecodeSlots <- struct{}{}:
			defer func() { <-imageDecodeSlots }()
		case <-r.Context().Done():
			writeError(w, r, http.StatusRequestTimeout, "REQUEST_CANCELLED", "Food post image processing was cancelled")
			return nil, false
		}
	}
	images := make([]decodedPostImage, 0, len(inputs))
	var totalPixels int64
	for _, item := range inputs {
		if item.ContentType != "image/jpeg" && item.ContentType != "image/png" && item.ContentType != "image/webp" {
			writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image content type is invalid")
			return nil, false
		}
		raw, err := base64.StdEncoding.DecodeString(item.Data)
		if err != nil || len(raw) == 0 || len(raw) > maxPostImageBytes {
			writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image bytes are invalid")
			return nil, false
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(raw))
		expectedFormat := map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/webp": "webp"}[item.ContentType]
		var validDimensions bool
		if err == nil && format == expectedFormat {
			totalPixels, validDimensions = addPostImagePixels(config.Width, config.Height, totalPixels)
		}
		if !validDimensions {
			writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image format or dimensions are invalid")
			return nil, false
		}
		decoded, decodedFormat, err := image.Decode(bytes.NewReader(raw))
		if err != nil || decodedFormat != expectedFormat {
			writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image cannot be decoded")
			return nil, false
		}
		// Never publish the uploader's original container bytes. Re-encoding the
		// decoded pixels strips EXIF/XMP/text chunks, including embedded GPS.
		if decodedFormat == "jpeg" {
			decoded = applyImageOrientation(decoded, jpegEXIFOrientation(raw))
		}
		contentType, clean, err := encodeSanitizedPostImage(decoded, decodedFormat)
		if err != nil || len(clean) == 0 || len(clean) > maxStoredImageBytes {
			writeError(w, r, http.StatusBadRequest, "INVALID_POST", "Food post image cannot be sanitized")
			return nil, false
		}
		sum := sha256.Sum256(clean)
		images = append(images, decodedPostImage{ContentType: contentType, Bytes: clean, SHA256: hex.EncodeToString(sum[:])})
	}
	return images, true
}

func addPostImagePixels(width, height int, current int64) (int64, bool) {
	if width < 1 || height < 1 || width > maxPostImageSide || height > maxPostImageSide {
		return current, false
	}
	pixels := int64(width) * int64(height)
	if pixels > maxPostImagePixels || current+pixels > maxPostTotalPixels {
		return current, false
	}
	return current + pixels, true
}

func encodeSanitizedPostImage(decoded image.Image, decodedFormat string) (string, []byte, error) {
	var sanitized bytes.Buffer
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
	ifdOffset := int(order.Uint32(tiff[4:8]))
	if ifdOffset < 8 || ifdOffset+2 > len(tiff) {
		return 1
	}
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
