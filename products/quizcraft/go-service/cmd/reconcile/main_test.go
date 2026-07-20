package main

import (
	"errors"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed output") }

func TestPrintJSONPropagatesOutputErrors(t *testing.T) {
	if err := printJSON(failingWriter{}, map[string]bool{"ready": true}); err == nil {
		t.Fatal("stdout write failure was ignored")
	}
}
