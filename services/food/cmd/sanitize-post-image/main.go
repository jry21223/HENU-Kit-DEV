package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	food "henukit.dev/food"
)

const maxInputBytes = 4 << 20

type request struct {
	ContentType string `json:"content_type"`
	DataBase64  string `json:"data_base64"`
}

type response struct {
	ContentType string `json:"content_type"`
	ByteSize    int    `json:"byte_size"`
	SHA256      string `json:"sha256"`
	DataBase64  string `json:"data_base64"`
}

func run(input io.Reader, output io.Writer) error {
	limited := io.LimitReader(input, maxInputBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil || len(raw) == 0 || len(raw) > maxInputBytes {
		return errors.New("sanitizer input is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request request
	if err := decoder.Decode(&request); err != nil {
		return errors.New("sanitizer request is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("sanitizer request has trailing content")
	}
	container, err := base64.StdEncoding.Strict().DecodeString(request.DataBase64)
	if err != nil {
		return errors.New("sanitizer image base64 is invalid")
	}
	sanitized, err := food.SanitizePostImage(request.ContentType, container)
	if err != nil {
		return fmt.Errorf("sanitize image: %w", err)
	}
	return json.NewEncoder(output).Encode(response{
		ContentType: sanitized.ContentType,
		ByteSize:    len(sanitized.Bytes),
		SHA256:      sanitized.SHA256,
		DataBase64:  base64.StdEncoding.EncodeToString(sanitized.Bytes),
	})
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "food-sanitize-post-image:", err)
		os.Exit(1)
	}
}
