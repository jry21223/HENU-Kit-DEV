package trainingcontract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

//go:embed owner.yaml
var ownerSchema []byte
var loadSchema = sync.OnceValues(func() (*openapi3.T, error) { return openapi3.NewLoader().LoadFromData(ownerSchema) })

// Validate enforces the frozen closed schema, including nested union variants.
// Generated union UnmarshalJSON methods alone intentionally do not validate.
func Validate(name string, raw []byte) error {
	doc, err := loadSchema()
	if err != nil {
		return err
	}
	schema, ok := doc.Components.Schemas[name]
	if !ok || schema.Value == nil {
		return errors.New("unknown training schema")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := decodeUnique(dec, &value); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return schema.Value.VisitJSON(value, openapi3.EnableFormatValidation(), openapi3.WithStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC4122)), openapi3.WithStringFormatValidator("date-time", openapi3.NewCallbackValidator(func(value string) error { _, err := time.Parse(time.RFC3339Nano, value); return err })))
}

// decodeUnique rejects duplicate object keys before a decoder can silently
// replace an earlier value. The exact original bytes remain the signed body.
func decodeUnique(dec *json.Decoder, target *any) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		*target = token
		return nil
	}
	switch delim {
	case '{':
		object := map[string]any{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errors.New("invalid JSON key")
			}
			if _, exists := object[name]; exists {
				return fmt.Errorf("duplicate JSON key %q", name)
			}
			var value any
			if err := decodeUnique(dec, &value); err != nil {
				return err
			}
			object[name] = value
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
		*target = object
	case '[':
		values := []any{}
		for dec.More() {
			var value any
			if err := decodeUnique(dec, &value); err != nil {
				return err
			}
			values = append(values, value)
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
		*target = values
	default:
		return errors.New("invalid JSON delimiter")
	}
	return nil
}
