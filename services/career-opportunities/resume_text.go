package career

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxResumePromptBytes = 20_000
	maxDOCXXMLBytes      = 2 << 20
	maxDOCXEntries       = 256
	maxPDFPages          = 10
	maxPDFPageImageBytes = 4 << 20
	maxPDFImagesBytes    = 20 << 20
	maxPDFToolOutput     = 64 << 10
	maxPDFPageSide       = 2048
	pdfToolTimeout       = 20 * time.Second
)

// resumeDocumentText converts each accepted upload format into bounded UTF-8
// before it crosses the LLM boundary. It never writes the resume to disk.
func resumeDocumentText(ctx context.Context, fileName string, content []byte) (text string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", ErrExtractionFailed
	}
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(fileName))) {
	case ".txt":
		if !utf8.Valid(content) {
			return "", fmt.Errorf("%w: resume text is not UTF-8", ErrExtractionFailed)
		}
		text = strings.TrimPrefix(string(content), "\ufeff")
	case ".docx":
		text, err = docxResumeText(content)
	case ".pdf":
		text, err = pdfResumeText(ctx, content)
	default:
		return "", fmt.Errorf("%w: unsupported resume document", ErrExtractionFailed)
	}
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\x00", " "))
	if text == "" || !utf8.ValidString(text) {
		return "", fmt.Errorf("%w: resume document contains no extractable text", ErrExtractionFailed)
	}
	return truncateProfileText(text, maxResumePromptBytes), nil
}

func docxResumeText(content []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil || len(archive.File) == 0 || len(archive.File) > maxDOCXEntries {
		return "", fmt.Errorf("%w: DOCX archive is invalid", ErrExtractionFailed)
	}
	var document *zip.File
	hasContentTypes := false
	for _, file := range archive.File {
		switch file.Name {
		case "[Content_Types].xml":
			hasContentTypes = true
		case "word/document.xml":
			if document != nil {
				return "", fmt.Errorf("%w: DOCX document entry is duplicated", ErrExtractionFailed)
			}
			document = file
		}
	}
	if !hasContentTypes || document == nil || document.UncompressedSize64 > maxDOCXXMLBytes {
		return "", fmt.Errorf("%w: DOCX document entry is missing or too large", ErrExtractionFailed)
	}
	stream, err := document.Open()
	if err != nil {
		return "", fmt.Errorf("%w: DOCX document cannot be opened", ErrExtractionFailed)
	}
	defer stream.Close()

	decoder := xml.NewDecoder(io.LimitReader(stream, maxDOCXXMLBytes+1))
	var builder strings.Builder
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w: DOCX document XML is invalid", ErrExtractionFailed)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "t":
				var run string
				if err := decoder.DecodeElement(&run, &value); err != nil {
					return "", fmt.Errorf("%w: DOCX text run is invalid", ErrExtractionFailed)
				}
				builder.WriteString(run)
			case "tab":
				builder.WriteByte('\t')
			case "br":
				builder.WriteByte('\n')
			}
		case xml.EndElement:
			if value.Name.Local == "p" {
				builder.WriteByte('\n')
			}
		}
		if builder.Len() > maxResumePromptBytes {
			break
		}
	}
	return builder.String(), nil
}

type boundedCommandOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (output *boundedCommandOutput) Write(value []byte) (int, error) {
	if len(value) > output.limit-output.buffer.Len() {
		return 0, errors.New("command output exceeds limit")
	}
	return output.buffer.Write(value)
}

func runPDFTool(ctx context.Context, name string, args []string, content []byte, limit int) ([]byte, error) {
	toolContext, cancel := context.WithTimeout(ctx, pdfToolTimeout)
	defer cancel()
	command := exec.CommandContext(toolContext, name, args...)
	command.Stdin = bytes.NewReader(content)
	stdout := &boundedCommandOutput{limit: limit}
	stderr := &boundedCommandOutput{limit: 4 << 10}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("%w: PDF tool failed", ErrExtractionFailed)
	}
	if err := toolContext.Err(); err != nil {
		return nil, fmt.Errorf("%w: PDF tool timed out", ErrExtractionFailed)
	}
	return stdout.buffer.Bytes(), nil
}

func pdfPageCount(ctx context.Context, content []byte) (int, error) {
	if len(content) < 5 || string(content[:5]) != "%PDF-" {
		return 0, fmt.Errorf("%w: PDF header is invalid", ErrExtractionFailed)
	}
	info, err := runPDFTool(ctx, "pdfinfo", []string{"-"}, content, maxPDFToolOutput)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(info), "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "Pages") {
			continue
		}
		pages, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr == nil && pages >= 1 && pages <= maxPDFPages {
			return pages, nil
		}
		break
	}
	return 0, fmt.Errorf("%w: PDF page count is invalid", ErrExtractionFailed)
}

func pdfResumeText(ctx context.Context, content []byte) (string, error) {
	if _, err := pdfPageCount(ctx, content); err != nil {
		return "", err
	}
	text, err := runPDFTool(ctx, "pdftotext", []string{"-", "-"}, content, maxResumePromptBytes+1)
	if err != nil || len(text) > maxResumePromptBytes {
		return "", fmt.Errorf("%w: PDF text cannot be extracted", ErrExtractionFailed)
	}
	return string(text), nil
}

// pdfResumeImages renders every bounded resume page to an in-memory JPEG. The
// model endpoint accepts image_url parts but rejects PDF/file parts, so this is
// the only protocol that preserves visual layout without writing user files to
// disk. Each Poppler process is context-killed and every output is hard-capped.
func pdfResumeImages(ctx context.Context, content []byte) ([][]byte, error) {
	pages, err := pdfPageCount(ctx, content)
	if err != nil {
		return nil, err
	}
	images := make([][]byte, 0, pages)
	total := 0
	for page := 1; page <= pages; page++ {
		imageBytes, err := runPDFTool(ctx, "pdftoppm", []string{
			"-jpeg", "-jpegopt", "quality=85", "-scale-to", strconv.Itoa(maxPDFPageSide), "-f", strconv.Itoa(page),
			"-l", strconv.Itoa(page), "-singlefile", "-",
		}, content, maxPDFPageImageBytes)
		if err != nil || len(imageBytes) < 3 || imageBytes[0] != 0xff || imageBytes[1] != 0xd8 || imageBytes[2] != 0xff {
			return nil, fmt.Errorf("%w: PDF page cannot be rendered", ErrExtractionFailed)
		}
		total += len(imageBytes)
		if total > maxPDFImagesBytes {
			return nil, fmt.Errorf("%w: rendered PDF is too large", ErrExtractionFailed)
		}
		images = append(images, imageBytes)
	}
	return images, nil
}
