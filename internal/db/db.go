package db

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open подключает БД по driver/dsn.
// Поддержка: "mysql" | "postgres" | "" (нет БД, in-memory режим).
func Open(driver, dsn string) (*gorm.DB, error) {
	switch driver {
	case "":
		return nil, nil
	case "mysql":
		// Пример DSN:
		// user:pass@tcp(127.0.0.1:3306)/openwisp?parseTime=true&charset=utf8mb4&loc=Local
		return gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres":
		// Пример DSN:
		// postgres://user:pass@localhost:5432/openwisp?sslmode=disable
		return gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func MigrateReservedColumns(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	dialect := db.Dialector.Name()

	// devices.key -> devices.device_key
	if db.Migrator().HasTable("devices") {
		hasOld := db.Migrator().HasColumn("devices", "key")
		hasNew := db.Migrator().HasColumn("devices", "device_key")
		if hasOld && !hasNew {
			if err := db.Migrator().RenameColumn("devices", "key", "device_key"); err != nil {
				var e error
				switch dialect {
				case "mysql":
					e = db.Exec("ALTER TABLE `devices` CHANGE COLUMN `key` `device_key` varchar(255) NOT NULL").Error
				case "postgres":
					e = db.Exec(`ALTER TABLE "devices" RENAME COLUMN "key" TO "device_key"`).Error
				case "sqlite":
					e = db.Exec(`ALTER TABLE devices RENAME COLUMN key TO device_key`).Error
				default:
					e = err
				}
				if e != nil {
					return fmt.Errorf("rename devices.key -> device_key: %w", e)
				}
			}
		}
		if !db.Migrator().HasIndex("devices", "idx_devices_device_key") {
			switch dialect {
			case "postgres":
				_ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_devices_device_key ON "devices" ("device_key")`).Error
			default:
				_ = db.Exec("CREATE INDEX idx_devices_device_key ON `devices` (`device_key`)").Error
			}
		}
	}

	// device_variables.key -> device_variables.var_key
	if db.Migrator().HasTable("device_variables") {
		hasOld := db.Migrator().HasColumn("device_variables", "key")
		hasNew := db.Migrator().HasColumn("device_variables", "var_key")
		if hasOld && !hasNew {
			if err := db.Migrator().RenameColumn("device_variables", "key", "var_key"); err != nil {
				var e error
				switch dialect {
				case "mysql":
					e = db.Exec("ALTER TABLE `device_variables` CHANGE COLUMN `key` `var_key` varchar(255) NOT NULL").Error
				case "postgres":
					e = db.Exec(`ALTER TABLE "device_variables" RENAME COLUMN "key" TO "var_key"`).Error
				case "sqlite":
					e = db.Exec(`ALTER TABLE device_variables RENAME COLUMN key TO var_key`).Error
				default:
					e = err
				}
				if e != nil {
					return fmt.Errorf("rename device_variables.key -> var_key: %w", e)
				}
			}
		}
	}

	// group_variables.key -> group_variables.var_key
	if db.Migrator().HasTable("group_variables") {
		hasOld := db.Migrator().HasColumn("group_variables", "key")
		hasNew := db.Migrator().HasColumn("group_variables", "var_key")
		if hasOld && !hasNew {
			if err := db.Migrator().RenameColumn("group_variables", "key", "var_key"); err != nil {
				var e error
				switch dialect {
				case "mysql":
					e = db.Exec("ALTER TABLE `group_variables` CHANGE COLUMN `key` `var_key` varchar(255) NOT NULL").Error
				case "postgres":
					e = db.Exec(`ALTER TABLE "group_variables" RENAME COLUMN "key" TO "var_key"`).Error
				case "sqlite":
					e = db.Exec(`ALTER TABLE group_variables RENAME COLUMN key TO var_key`).Error
				default:
					e = err
				}
				if e != nil {
					return fmt.Errorf("rename group_variables.key -> var_key: %w", e)
				}
			}
		}
	}

	return nil
}
