package materialimport

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
)

var (
	ErrEmptyManifest      = errors.New("empty_manifest")
	ErrMissingRequired    = errors.New("missing_required_field")
	ErrInvalidMaterial    = errors.New("invalid_material")
	ErrMissingFile        = errors.New("manifest_file_missing")
	ErrUnsafeFilePath     = errors.New("unsafe_manifest_file_path")
	ErrPackageScope       = errors.New("package_scope_mismatch")
	ErrUnsupportedVersion = errors.New("unsupported_manifest")
)

type ManifestEntry struct {
	School             string             `json:"school"`
	College            string             `json:"college"`
	Major              string             `json:"major"`
	MajorSlug          string             `json:"majorSlug"`
	Grade              string             `json:"grade"`
	CourseSlug         string             `json:"courseSlug"`
	CourseName         string             `json:"courseName"`
	CourseDescription  string             `json:"courseDescription"`
	ExamScope          string             `json:"examScope"`
	PackageSlug        string             `json:"packageSlug"`
	PackageTitle       string             `json:"packageTitle"`
	PackageDescription string             `json:"packageDescription"`
	PackageStatus      string             `json:"packageStatus"`
	PriceFen           *int64             `json:"priceFen"`
	Currency           string             `json:"currency"`
	Materials          []ManifestMaterial `json:"materials"`
}

type ManifestMaterial struct {
	Title          string `json:"title"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	FilePath       string `json:"filePath"`
	FileName       string `json:"fileName"`
	AccessLevel    string `json:"accessLevel"`
	Status         string `json:"status"`
	PreviewContent string `json:"previewContent"`
}

type Result struct {
	Entries           int `json:"entries"`
	SchoolsCreated    int `json:"schoolsCreated"`
	CollegesCreated   int `json:"collegesCreated"`
	MajorsCreated     int `json:"majorsCreated"`
	CoursesCreated    int `json:"coursesCreated"`
	PackagesCreated   int `json:"packagesCreated"`
	PackagesUpdated   int `json:"packagesUpdated"`
	MaterialsCreated  int `json:"materialsCreated"`
	MaterialsUpdated  int `json:"materialsUpdated"`
	PackageItemsAdded int `json:"packageItemsAdded"`
	PackageItemsKept  int `json:"packageItemsKept"`
}

type Importer struct {
	db        *gorm.DB
	uploadDir string
}

func New(db *gorm.DB, uploadDir string) Importer {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	return Importer{db: db, uploadDir: uploadDir}
}

func (i Importer) ImportFile(path string) (Result, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	var manifest []ManifestEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Result{}, err
	}
	return i.Import(manifest)
}

func (i Importer) Import(manifest []ManifestEntry) (Result, error) {
	if len(manifest) == 0 {
		return Result{}, ErrEmptyManifest
	}
	var result Result
	err := i.db.Transaction(func(tx *gorm.DB) error {
		worker := Importer{db: tx, uploadDir: i.uploadDir}
		for index := range manifest {
			if err := worker.importEntry(&result, manifest[index]); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func (i Importer) importEntry(result *Result, entry ManifestEntry) error {
	result.Entries++
	if strings.TrimSpace(entry.School) == "" ||
		strings.TrimSpace(entry.College) == "" ||
		strings.TrimSpace(entry.Major) == "" ||
		strings.TrimSpace(entry.Grade) == "" ||
		strings.TrimSpace(entry.CourseSlug) == "" ||
		strings.TrimSpace(entry.CourseName) == "" ||
		strings.TrimSpace(entry.PackageSlug) == "" ||
		len(entry.Materials) == 0 {
		return ErrMissingRequired
	}

	school, created, err := i.ensureSchool(entry.School)
	if err != nil {
		return err
	}
	if created {
		result.SchoolsCreated++
	}
	college, created, err := i.ensureCollege(school, entry.College)
	if err != nil {
		return err
	}
	if created {
		result.CollegesCreated++
	}
	majorSlug := strings.TrimSpace(entry.MajorSlug)
	if majorSlug == "" {
		majorSlug = slugFromName(entry.Major)
	}
	major, created, err := i.ensureMajor(school, college, entry.Major, majorSlug)
	if err != nil {
		return err
	}
	if created {
		result.MajorsCreated++
	}
	course, created, err := i.ensureCourse(school, college, major, entry)
	if err != nil {
		return err
	}
	if created {
		result.CoursesCreated++
	}
	coursePackage, created, err := i.ensurePackage(school, college, major, course, entry)
	if err != nil {
		return err
	}
	if created {
		result.PackagesCreated++
	} else {
		result.PackagesUpdated++
	}

	for materialIndex := range entry.Materials {
		material, created, err := i.upsertMaterial(course, entry.Materials[materialIndex])
		if err != nil {
			return err
		}
		if created {
			result.MaterialsCreated++
		} else {
			result.MaterialsUpdated++
		}
		added, err := i.ensurePackageItem(coursePackage, material, materialIndex+1)
		if err != nil {
			return err
		}
		if added {
			result.PackageItemsAdded++
		} else {
			result.PackageItemsKept++
		}
	}
	return nil
}

func (i Importer) ensureSchool(name string) (model.School, bool, error) {
	school := model.School{Name: strings.TrimSpace(name)}
	if err := i.db.Where("name = ?", school.Name).First(&school).Error; err == nil {
		return school, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.School{}, false, err
	}
	school = model.School{
		Name:   strings.TrimSpace(name),
		Slug:   slugFromName(name),
		Status: model.StatusPublished,
	}
	if err := i.db.Create(&school).Error; err != nil {
		return model.School{}, false, err
	}
	return school, true, nil
}

func (i Importer) ensureCollege(school model.School, name string) (model.College, bool, error) {
	college := model.College{}
	trimmedName := strings.TrimSpace(name)
	if err := i.db.Where("school_id = ? AND name = ?", school.ID, trimmedName).First(&college).Error; err == nil {
		return college, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.College{}, false, err
	}
	college = model.College{SchoolID: school.ID, Name: trimmedName, Status: model.StatusPublished}
	if err := i.db.Create(&college).Error; err != nil {
		return model.College{}, false, err
	}
	return college, true, nil
}

func (i Importer) ensureMajor(school model.School, college model.College, name string, slug string) (model.Major, bool, error) {
	major := model.Major{}
	trimmedSlug := strings.TrimSpace(slug)
	if err := i.db.Where("school_id = ? AND slug = ?", school.ID, trimmedSlug).First(&major).Error; err == nil {
		updates := map[string]interface{}{"college_id": college.ID, "name": strings.TrimSpace(name), "status": model.StatusPublished}
		if err := i.db.Model(&major).Updates(updates).Error; err != nil {
			return model.Major{}, false, err
		}
		if err := i.db.First(&major, "id = ?", major.ID).Error; err != nil {
			return model.Major{}, false, err
		}
		return major, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Major{}, false, err
	}
	major = model.Major{
		SchoolID:  school.ID,
		CollegeID: college.ID,
		Name:      strings.TrimSpace(name),
		Slug:      trimmedSlug,
		Status:    model.StatusPublished,
	}
	if err := i.db.Create(&major).Error; err != nil {
		return model.Major{}, false, err
	}
	return major, true, nil
}

func (i Importer) ensureCourse(school model.School, college model.College, major model.Major, entry ManifestEntry) (model.Course, bool, error) {
	course := model.Course{}
	slug := strings.TrimSpace(entry.CourseSlug)
	grade := strings.TrimSpace(entry.Grade)
	if err := i.db.Where("major_id = ? AND grade = ? AND slug = ?", major.ID, grade, slug).First(&course).Error; err == nil {
		updates := map[string]interface{}{
			"school_id":   school.ID,
			"college_id":  college.ID,
			"name":        strings.TrimSpace(entry.CourseName),
			"description": strings.TrimSpace(entry.CourseDescription),
			"exam_scope":  strings.TrimSpace(entry.ExamScope),
			"status":      model.StatusPublished,
		}
		if err := i.db.Model(&course).Updates(updates).Error; err != nil {
			return model.Course{}, false, err
		}
		if err := i.db.First(&course, "id = ?", course.ID).Error; err != nil {
			return model.Course{}, false, err
		}
		return course, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Course{}, false, err
	}
	course = model.Course{
		SchoolID:    school.ID,
		CollegeID:   college.ID,
		MajorID:     major.ID,
		Grade:       grade,
		Name:        strings.TrimSpace(entry.CourseName),
		Slug:        slug,
		Description: strings.TrimSpace(entry.CourseDescription),
		ExamScope:   strings.TrimSpace(entry.ExamScope),
		Status:      model.StatusPublished,
	}
	if err := i.db.Create(&course).Error; err != nil {
		return model.Course{}, false, err
	}
	return course, true, nil
}

func (i Importer) ensurePackage(school model.School, college model.College, major model.Major, course model.Course, entry ManifestEntry) (model.CoursePackage, bool, error) {
	coursePackage := model.CoursePackage{}
	slug := strings.TrimSpace(entry.PackageSlug)
	title := strings.TrimSpace(entry.PackageTitle)
	if title == "" {
		title = course.Name + "期末复习包"
	}
	description := strings.TrimSpace(entry.PackageDescription)
	if description == "" {
		description = fmt.Sprintf("%s %s %s %s 课程复习包", school.Name, college.Name, entry.Grade, course.Name)
	}
	priceFen := int64(0)
	if entry.PriceFen != nil {
		if *entry.PriceFen < 0 {
			return model.CoursePackage{}, false, ErrInvalidMaterial
		}
		priceFen = *entry.PriceFen
	}
	currency := strings.ToUpper(strings.TrimSpace(entry.Currency))
	if currency == "" {
		currency = "CNY"
	}
	status, ok := normalizeStatus(entry.PackageStatus, model.StatusPublished)
	if !ok {
		return model.CoursePackage{}, false, ErrInvalidMaterial
	}

	if err := i.db.Where("slug = ?", slug).First(&coursePackage).Error; err == nil {
		if coursePackage.SchoolID != school.ID || coursePackage.CollegeID != college.ID || coursePackage.MajorID != major.ID || coursePackage.Grade != course.Grade {
			return model.CoursePackage{}, false, ErrPackageScope
		}
		courseID := course.ID
		updates := map[string]interface{}{
			"course_id":   &courseID,
			"title":       title,
			"description": description,
			"price_fen":   priceFen,
			"currency":    currency,
			"status":      status,
			"school_id":   school.ID,
			"college_id":  college.ID,
			"major_id":    major.ID,
			"grade":       course.Grade,
		}
		if err := i.db.Model(&coursePackage).Updates(updates).Error; err != nil {
			return model.CoursePackage{}, false, err
		}
		if err := i.db.First(&coursePackage, "id = ?", coursePackage.ID).Error; err != nil {
			return model.CoursePackage{}, false, err
		}
		return coursePackage, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.CoursePackage{}, false, err
	}
	courseID := course.ID
	coursePackage = model.CoursePackage{
		SchoolID:    school.ID,
		CollegeID:   college.ID,
		MajorID:     major.ID,
		CourseID:    &courseID,
		Grade:       course.Grade,
		Title:       title,
		Slug:        slug,
		Description: description,
		PriceFen:    priceFen,
		Currency:    currency,
		Status:      status,
	}
	if err := i.db.Create(&coursePackage).Error; err != nil {
		return model.CoursePackage{}, false, err
	}
	return coursePackage, true, nil
}

func (i Importer) upsertMaterial(course model.Course, manifestMaterial ManifestMaterial) (model.Material, bool, error) {
	title := strings.TrimSpace(manifestMaterial.Title)
	if title == "" || strings.TrimSpace(manifestMaterial.FilePath) == "" {
		return model.Material{}, false, ErrMissingRequired
	}
	materialType, ok := normalizeMaterialType(manifestMaterial.Type)
	if !ok {
		return model.Material{}, false, ErrInvalidMaterial
	}
	accessLevel, ok := normalizeAccessLevel(manifestMaterial.AccessLevel)
	if !ok {
		return model.Material{}, false, ErrInvalidMaterial
	}
	status, ok := normalizeStatus(manifestMaterial.Status, model.StatusDraft)
	if !ok {
		return model.Material{}, false, ErrInvalidMaterial
	}
	storageKey, size, err := i.storageKeyForManifestPath(manifestMaterial.FilePath)
	if err != nil {
		return model.Material{}, false, err
	}
	fileName := strings.TrimSpace(manifestMaterial.FileName)
	if fileName == "" {
		fileName = filepath.Base(storageKey)
	}

	material := model.Material{}
	updates := map[string]interface{}{
		"type":            materialType,
		"description":     strings.TrimSpace(manifestMaterial.Description),
		"storage_key":     storageKey,
		"file_name":       fileName,
		"file_size":       size,
		"preview_content": strings.TrimSpace(manifestMaterial.PreviewContent),
		"access_level":    accessLevel,
		"status":          status,
	}
	if err := i.db.Where("course_id = ? AND title = ?", course.ID, title).First(&material).Error; err == nil {
		if err := i.db.Model(&material).Updates(updates).Error; err != nil {
			return model.Material{}, false, err
		}
		if err := i.db.First(&material, "id = ?", material.ID).Error; err != nil {
			return model.Material{}, false, err
		}
		return material, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Material{}, false, err
	}
	material = model.Material{
		CourseID:       course.ID,
		Title:          title,
		Type:           materialType,
		Description:    strings.TrimSpace(manifestMaterial.Description),
		StorageKey:     storageKey,
		FileName:       fileName,
		FileSize:       size,
		PreviewContent: strings.TrimSpace(manifestMaterial.PreviewContent),
		AccessLevel:    accessLevel,
		Status:         status,
	}
	if err := i.db.Create(&material).Error; err != nil {
		return model.Material{}, false, err
	}
	return material, true, nil
}

func (i Importer) ensurePackageItem(coursePackage model.CoursePackage, material model.Material, sortOrder int) (bool, error) {
	if coursePackage.CourseID != nil && *coursePackage.CourseID != material.CourseID {
		return false, ErrPackageScope
	}
	item := model.CoursePackageItem{}
	err := i.db.Where("package_id = ? AND resource_type = ? AND resource_id = ?", coursePackage.ID, "material", material.ID).First(&item).Error
	if err == nil {
		if item.SortOrder != sortOrder {
			if err := i.db.Model(&item).Update("sort_order", sortOrder).Error; err != nil {
				return false, err
			}
		}
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	item = model.CoursePackageItem{
		PackageID:    coursePackage.ID,
		ResourceType: "material",
		ResourceID:   material.ID,
		SortOrder:    sortOrder,
	}
	if err := i.db.Create(&item).Error; err != nil {
		return false, err
	}
	return true, nil
}

func (i Importer) storageKeyForManifestPath(rawPath string) (string, int64, error) {
	root, err := filepath.Abs(i.uploadDir)
	if err != nil {
		return "", 0, err
	}
	candidate := filepath.Clean(filepath.FromSlash(strings.TrimSpace(rawPath)))
	if candidate == "." || candidate == "" {
		return "", 0, ErrUnsafeFilePath
	}
	var absPath string
	if filepath.IsAbs(candidate) {
		absPath = candidate
	} else {
		parts := splitPath(candidate)
		if len(parts) > 0 && (parts[0] == "uploads" || parts[0] == filepath.Base(root)) {
			candidate = filepath.Join(parts[1:]...)
		}
		absPath = filepath.Join(root, candidate)
	}
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", 0, err
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return "", 0, ErrUnsafeFilePath
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", 0, ErrUnsafeFilePath
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", 0, ErrMissingFile
		}
		return "", 0, err
	}
	if info.IsDir() {
		return "", 0, ErrMissingFile
	}
	return filepath.ToSlash(rel), info.Size(), nil
}

func splitPath(path string) []string {
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			clean = append(clean, part)
		}
	}
	return clean
}

func normalizeMaterialType(value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		normalized = "other"
	}
	switch normalized {
	case "knowledge_note", "mock_paper", "answer", "quick_review", "past_exam", "other":
		return normalized, true
	default:
		return "", false
	}
}

func normalizeAccessLevel(value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		normalized = model.MaterialAccessLoginRequired
	}
	switch normalized {
	case model.MaterialAccessFree, model.MaterialAccessLoginRequired, model.MaterialAccessPaid, model.MaterialAccessMemberOnly:
		return normalized, true
	default:
		return "", false
	}
}

func normalizeStatus(value string, fallback string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		normalized = fallback
	}
	if normalized == "pending_review" {
		normalized = model.StatusPending
	}
	switch normalized {
	case model.StatusDraft, model.StatusPending, model.StatusPublished, model.StatusRejected, model.StatusArchived:
		return normalized, true
	default:
		return "", false
	}
}

func slugFromName(value string) string {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "河南大学":
		return "henu"
	case "网络工程":
		return "network-engineering"
	case "软件工程":
		return "software-engineering"
	}
	lower := strings.ToLower(trimmed)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := strings.Trim(re.ReplaceAllString(lower, "-"), "-")
	if slug != "" {
		return slug
	}
	sum := sha1.Sum([]byte(trimmed))
	return "item-" + hex.EncodeToString(sum[:])[:10]
}
