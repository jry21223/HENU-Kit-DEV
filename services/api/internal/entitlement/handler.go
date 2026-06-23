package entitlement

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type materialEntitlement struct {
	Grant    model.MaterialAccessGrant `json:"grant"`
	Material *model.Material           `json:"material,omitempty"`
}

type packageEntitlement struct {
	Grant     model.MaterialAccessGrant `json:"grant"`
	Package   *model.CoursePackage      `json:"package,omitempty"`
	Materials []model.Material          `json:"materials"`
}

type entitlementSummary struct {
	DirectMaterialGrants int `json:"directMaterialGrants"`
	PackageGrants        int `json:"packageGrants"`
	UnlockedMaterials    int `json:"unlockedMaterials"`
}

func (h Handler) Me(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}

	grants, err := h.activeGrants(user.ID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	directGrantRows, packageGrantRows, err := h.expandGrants(grants)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	response.OK(ctx, gin.H{
		"summary": entitlementSummary{
			DirectMaterialGrants: len(directGrantRows),
			PackageGrants:        len(packageGrantRows),
			UnlockedMaterials:    countUnlockedMaterials(directGrantRows, packageGrantRows),
		},
		"materialGrants": directGrantRows,
		"packageGrants":  packageGrantRows,
	})
}

func (h Handler) activeGrants(userID string) ([]model.MaterialAccessGrant, error) {
	var grants []model.MaterialAccessGrant
	err := h.db.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now()).
		Order("created_at desc").
		Limit(200).
		Find(&grants).Error
	return grants, err
}

func (h Handler) expandGrants(grants []model.MaterialAccessGrant) ([]materialEntitlement, []packageEntitlement, error) {
	materialIDs := make([]string, 0, len(grants))
	packageIDs := make([]string, 0, len(grants))
	for _, grant := range grants {
		if grant.MaterialID != nil && *grant.MaterialID != "" {
			materialIDs = append(materialIDs, *grant.MaterialID)
		}
		if grant.PackageID != nil && *grant.PackageID != "" {
			packageIDs = append(packageIDs, *grant.PackageID)
		}
	}

	materialsByID, err := h.materialsByID(materialIDs)
	if err != nil {
		return nil, nil, err
	}
	packagesByID, err := h.publishedPackagesByID(packageIDs)
	if err != nil {
		return nil, nil, err
	}
	packageMaterials, err := h.packageMaterials(packageIDs)
	if err != nil {
		return nil, nil, err
	}

	materialRows := make([]materialEntitlement, 0, len(materialIDs))
	packageRows := make([]packageEntitlement, 0, len(packageIDs))
	for _, grant := range grants {
		if grant.MaterialID != nil && *grant.MaterialID != "" {
			var material *model.Material
			if row, ok := materialsByID[*grant.MaterialID]; ok {
				material = &row
			}
			materialRows = append(materialRows, materialEntitlement{Grant: grant, Material: material})
			continue
		}
		if grant.PackageID != nil && *grant.PackageID != "" {
			pkg, ok := packagesByID[*grant.PackageID]
			if !ok {
				continue
			}
			packageRows = append(packageRows, packageEntitlement{
				Grant:     grant,
				Package:   &pkg,
				Materials: packageMaterials[*grant.PackageID],
			})
		}
	}
	return materialRows, packageRows, nil
}

func (h Handler) materialsByID(ids []string) (map[string]model.Material, error) {
	rows := map[string]model.Material{}
	if len(ids) == 0 {
		return rows, nil
	}
	var materials []model.Material
	if err := h.db.Where("id IN ?", ids).Find(&materials).Error; err != nil {
		return nil, err
	}
	for _, material := range materials {
		rows[material.ID] = material
	}
	return rows, nil
}

func (h Handler) publishedPackagesByID(ids []string) (map[string]model.CoursePackage, error) {
	rows := map[string]model.CoursePackage{}
	if len(ids) == 0 {
		return rows, nil
	}
	var packages []model.CoursePackage
	if err := h.db.Where("id IN ? AND status = ?", ids, model.StatusPublished).Find(&packages).Error; err != nil {
		return nil, err
	}
	for _, pkg := range packages {
		rows[pkg.ID] = pkg
	}
	return rows, nil
}

func (h Handler) packageMaterials(packageIDs []string) (map[string][]model.Material, error) {
	rows := map[string][]model.Material{}
	if len(packageIDs) == 0 {
		return rows, nil
	}
	var items []model.CoursePackageItem
	if err := h.db.Where("package_id IN ? AND resource_type = ?", packageIDs, "material").
		Order("sort_order asc").
		Find(&items).Error; err != nil {
		return nil, err
	}
	materialIDs := make([]string, 0, len(items))
	for _, item := range items {
		materialIDs = append(materialIDs, item.ResourceID)
	}
	materialsByID, err := h.materialsByID(materialIDs)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if material, ok := materialsByID[item.ResourceID]; ok {
			rows[item.PackageID] = append(rows[item.PackageID], material)
		}
	}
	return rows, nil
}

func countUnlockedMaterials(materialRows []materialEntitlement, packageRows []packageEntitlement) int {
	seen := map[string]bool{}
	for _, row := range materialRows {
		if row.Material != nil {
			seen[row.Material.ID] = true
		}
	}
	for _, row := range packageRows {
		for _, material := range row.Materials {
			seen[material.ID] = true
		}
	}
	return len(seen)
}
