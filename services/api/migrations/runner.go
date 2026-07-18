package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

//go:embed *.up.sql *.down.sql
var files embed.FS

type appliedMigration struct {
	Version   string    `gorm:"primaryKey;size:160"`
	Checksum  string    `gorm:"size:64;not null"`
	AppliedAt time.Time `gorm:"not null"`
}

func (appliedMigration) TableName() string { return "schema_migrations" }

type Runner struct{ db *gorm.DB }

func NewRunner(db *gorm.DB) Runner { return Runner{db: db} }

func (runner Runner) Up(ctx context.Context) error {
	if err := runner.db.WithContext(ctx).AutoMigrate(&appliedMigration{}); err != nil {
		return err
	}
	names, err := migrationNames(".up.sql")
	if err != nil {
		return err
	}
	for _, name := range names {
		body, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		version := strings.TrimSuffix(name, ".up.sql")
		checksum := checksum(body)
		var applied appliedMigration
		err = runner.db.WithContext(ctx).First(&applied, "version = ?", version).Error
		if err == nil {
			if applied.Checksum != checksum {
				return fmt.Errorf("migration %s checksum changed", version)
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := runner.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(body)).Error; err != nil {
				return fmt.Errorf("apply %s: %w", version, err)
			}
			return tx.Create(&appliedMigration{Version: version, Checksum: checksum, AppliedAt: time.Now().UTC()}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func (runner Runner) Down(ctx context.Context) error {
	if err := runner.db.WithContext(ctx).AutoMigrate(&appliedMigration{}); err != nil {
		return err
	}
	var applied appliedMigration
	if err := runner.db.WithContext(ctx).Order("version desc").First(&applied).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	name := applied.Version + ".down.sql"
	body, err := files.ReadFile(name)
	if err != nil {
		return fmt.Errorf("missing rollback for %s: %w", applied.Version, err)
	}
	return runner.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(string(body)).Error; err != nil {
			return fmt.Errorf("rollback %s: %w", applied.Version, err)
		}
		return tx.Delete(&applied).Error
	})
}

func migrationNames(suffix string) ([]string, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func checksum(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
