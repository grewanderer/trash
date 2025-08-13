// internal/db/migrations.go
package db

import (
	"fmt"

	"gorm.io/gorm"
)

func MigrateTemplateUniqueIndex(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	dialect := db.Dialector.Name()

	switch dialect {
	case "mysql":
		_ = db.Exec("DROP INDEX `idx_tpl_name` ON `templates`").Error
		return db.Exec("CREATE UNIQUE INDEX `ux_templates_name_del` ON `templates` (`name`, `deleted_at`)").Error

	case "postgres":
		_ = db.Exec(`DROP INDEX IF EXISTS idx_tpl_name`).Error
		// partial unique index (куда лучше для soft-delete)
		return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_templates_name_null ON "templates" ("name") WHERE "deleted_at" IS NULL`).Error

	case "sqlite":
		_ = db.Exec(`DROP INDEX IF EXISTS idx_tpl_name`).Error
		return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_templates_name_del ON templates (name, deleted_at)`).Error

	default:
		return fmt.Errorf("unsupported dialect: %s", dialect)
	}
}
