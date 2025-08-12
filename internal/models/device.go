package models

import (
	"time"

	"gorm.io/gorm"
)

// Device — регистрируемое openwrt-устройство (минимум для контроллера).
type Device struct {
	gorm.Model
	UUID          string `gorm:"uniqueIndex;size:36"`
	DeviceKey     string `gorm:"index;size:64"`
	Name          string
	Backend       string
	MAC           string
	Status        string
	LastSeen      *time.Time
	LastError     string `gorm:"type:text"`
	LastConfigSHA string `gorm:"size:64"`
}

type DeviceStatusHistory struct {
	gorm.Model
	DeviceUUID string `gorm:"index;size:36"`
	Status     string `gorm:"index;size:16"`
	ConfigSHA  string `gorm:"size:64"`
	Error      string `gorm:"type:text"`
}
