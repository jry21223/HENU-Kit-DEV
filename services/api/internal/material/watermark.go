package material

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"final-review-platform/services/api/internal/platform/model"
)

type watermarkContext struct {
	User       *model.User
	MaterialID string
	Time       time.Time
}

func shouldWatermarkPDF(path string, fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(path))
	}
	return ext == ".pdf"
}

func watermarkedPDFPath(sourcePath string, ctx watermarkContext) (string, func(), error) {
	tempFile, err := os.CreateTemp("", "final-review-watermark-*.pdf")
	if err != nil {
		return "", nil, err
	}
	outputPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(outputPath)
		return "", nil, err
	}

	text := watermarkText(ctx)
	desc := "font:Helvetica, points:14, color:0.72 0.72 0.72, opacity:.18, diagonal:1"
	if err := api.AddTextWatermarksFile(sourcePath, outputPath, nil, true, text, desc, nil); err != nil {
		_ = os.Remove(outputPath)
		return "", nil, err
	}
	return outputPath, func() { _ = os.Remove(outputPath) }, nil
}

func watermarkText(ctx watermarkContext) string {
	userLabel := "anonymous"
	if ctx.User != nil && strings.TrimSpace(ctx.User.Email) != "" {
		userLabel = strings.TrimSpace(ctx.User.Email)
	}
	downloadedAt := ctx.Time.Format("2006-01-02 15:04:05")
	return fmt.Sprintf("final-review-platform | user:%s | material:%s | %s | personal review only", userLabel, ctx.MaterialID, downloadedAt)
}
