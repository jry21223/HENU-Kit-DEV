package career

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"image/jpeg"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func testDOCX(t *testing.T, text string) []byte {
	t.Helper()
	var raw bytes.Buffer
	archive := zip.NewWriter(&raw)
	contentTypes, err := archive.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = contentTypes.Write([]byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`))
	document, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = document.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func testPDF(text string) []byte {
	stream := "BT /F1 12 Tf 72 720 Td (" + strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(text) + ") Tj ET"
	return testPDFStream([]byte(stream), "")
}

func testPDFStream(stream []byte, streamOptions string) []byte {
	return testPDFStreamWithMediaBox(stream, streamOptions, "612 792")
}

func testPDFStreamWithMediaBox(stream []byte, streamOptions, mediaBox string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 " + mediaBox + "] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Length " + strconv.Itoa(len(stream)) + streamOptions + " >>\nstream\n" + string(stream) + "\nendstream",
	}
	var raw bytes.Buffer
	raw.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = raw.Len()
		_, _ = fmt.Fprintf(&raw, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := raw.Len()
	_, _ = fmt.Fprintf(&raw, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index < len(offsets); index++ {
		_, _ = fmt.Fprintf(&raw, "%010d 00000 n \n", offsets[index])
	}
	_, _ = fmt.Fprintf(&raw, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return raw.Bytes()
}

func TestPDFResumeImagesBoundsOversizedMediaBoxBeforeRasterOutput(t *testing.T) {
	stream := []byte("BT /F1 12 Tf 72 720 Td (Backend Developer) Tj ET")
	images, err := pdfResumeImages(context.Background(), testPDFStreamWithMediaBox(stream, "", "100000 100000"))
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("rendered pages = %d, want 1", len(images))
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(images[0]))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width > maxPDFPageSide || config.Height > maxPDFPageSide {
		t.Fatalf("rendered page = %dx%d, maximum side %d", config.Width, config.Height, maxPDFPageSide)
	}
}

func zlibBytes(t *testing.T, content []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func TestResumeDocumentTextSupportsEveryAdvertisedFormat(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
		content  []byte
	}{
		{name: "txt", fileName: "resume.txt", content: []byte("Backend Developer\nGo PostgreSQL")},
		{name: "docx", fileName: "resume.docx", content: testDOCX(t, "Backend Developer Go PostgreSQL")},
		{name: "pdf", fileName: "resume.pdf", content: testPDF("Backend Developer Go PostgreSQL")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			text, err := resumeDocumentText(context.Background(), test.fileName, test.content)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text, "Backend Developer") || !strings.Contains(text, "PostgreSQL") {
				t.Fatalf("extracted text = %q", text)
			}
		})
	}
}

func TestResumeDocumentTextRejectsMalformedAndEmptyDocuments(t *testing.T) {
	for _, test := range []struct {
		fileName string
		content  []byte
	}{
		{fileName: "resume.pdf", content: []byte("%PDF-not-a-document")},
		{fileName: "resume.docx", content: []byte("PK\x03\x04not-a-document")},
		{fileName: "resume.txt", content: []byte(" \n\t")},
	} {
		if _, err := resumeDocumentText(context.Background(), test.fileName, test.content); err == nil {
			t.Fatalf("accepted malformed %s", test.fileName)
		}
	}
}

func TestResumePDFParsingIsConcurrentAndTraceFree(t *testing.T) {
	content := testPDF("Private Resume Text Backend Developer")
	var wait sync.WaitGroup
	errors := make(chan error, 8)
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			text, err := resumeDocumentText(context.Background(), "resume.pdf", content)
			if err != nil {
				errors <- err
				return
			}
			if !strings.Contains(text, "Private Resume Text") {
				errors <- fmt.Errorf("missing PDF text: %q", text)
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func TestResumePDFRejectsUnboundedPredictorColumns(t *testing.T) {
	content := testPDFStream(
		zlibBytes(t, []byte("BT ET")),
		" /Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns 9223372036854775807 >>",
	)
	if _, err := resumeDocumentText(context.Background(), "resume.pdf", content); err == nil {
		t.Fatal("accepted a PDF with unbounded predictor columns")
	}
}

func TestResumePDFRejectsDeflateBombBeyondDocumentBudget(t *testing.T) {
	// The compressed upload is tiny, but its page stream expands beyond the
	// 16 MiB document budget. Spaces force the content lexer to consume it.
	expanded := bytes.Repeat([]byte{' '}, (16<<20)+1)
	content := testPDFStream(zlibBytes(t, expanded), " /Filter /FlateDecode")
	if _, err := resumeDocumentText(context.Background(), "resume.pdf", content); err == nil {
		t.Fatal("accepted a PDF whose decoded streams exceed the document budget")
	}
}
