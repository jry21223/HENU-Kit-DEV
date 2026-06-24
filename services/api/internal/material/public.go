package material

import "final-review-platform/services/api/internal/platform/model"

type PublicMaterial struct {
	ID             string `json:"id"`
	CourseID       string `json:"courseId"`
	Title          string `json:"title"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	FileName       string `json:"fileName"`
	FileSize       int64  `json:"fileSize"`
	PreviewContent string `json:"previewContent"`
	AccessLevel    string `json:"accessLevel"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func ToPublic(material model.Material) PublicMaterial {
	return PublicMaterial{
		ID:             material.ID,
		CourseID:       material.CourseID,
		Title:          material.Title,
		Type:           material.Type,
		Description:    material.Description,
		FileName:       material.FileName,
		FileSize:       material.FileSize,
		PreviewContent: material.PreviewContent,
		AccessLevel:    material.AccessLevel,
		Status:         material.Status,
		CreatedAt:      material.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      material.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func ToPublicList(materials []model.Material) []PublicMaterial {
	result := make([]PublicMaterial, 0, len(materials))
	for _, item := range materials {
		result = append(result, ToPublic(item))
	}
	return result
}
