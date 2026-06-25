package packagecatalog

import "final-review-platform/services/api/internal/platform/model"

type PublicPackage struct {
	ID          string  `json:"id"`
	SchoolID    string  `json:"schoolId"`
	CollegeID   string  `json:"collegeId"`
	MajorID     string  `json:"majorId"`
	CourseID    *string `json:"courseId,omitempty"`
	Grade       string  `json:"grade"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	PriceFen    int64   `json:"priceFen"`
	Currency    string  `json:"currency"`
}

type PublicPackageItem struct {
	ID           string `json:"id"`
	PackageID    string `json:"packageId"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	SortOrder    int    `json:"sortOrder"`
}

func ToPublicPackage(coursePackage model.CoursePackage) PublicPackage {
	return PublicPackage{
		ID:          coursePackage.ID,
		SchoolID:    coursePackage.SchoolID,
		CollegeID:   coursePackage.CollegeID,
		MajorID:     coursePackage.MajorID,
		CourseID:    coursePackage.CourseID,
		Grade:       coursePackage.Grade,
		Title:       coursePackage.Title,
		Slug:        coursePackage.Slug,
		Description: coursePackage.Description,
		PriceFen:    coursePackage.PriceFen,
		Currency:    coursePackage.Currency,
	}
}

func ToPublicPackages(packages []model.CoursePackage) []PublicPackage {
	result := make([]PublicPackage, 0, len(packages))
	for _, coursePackage := range packages {
		result = append(result, ToPublicPackage(coursePackage))
	}
	return result
}

func ToPublicPackageItem(item model.CoursePackageItem) PublicPackageItem {
	return PublicPackageItem{
		ID:           item.ID,
		PackageID:    item.PackageID,
		ResourceType: item.ResourceType,
		ResourceID:   item.ResourceID,
		SortOrder:    item.SortOrder,
	}
}

func ToPublicPackageItems(items []model.CoursePackageItem) []PublicPackageItem {
	result := make([]PublicPackageItem, 0, len(items))
	for _, item := range items {
		result = append(result, ToPublicPackageItem(item))
	}
	return result
}
