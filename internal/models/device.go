package models

import "gorm.io/gorm"

// Device — регистрируемое openwrt-устройство (минимум для контроллера).
type Device struct {
	gorm.Model
	UUID      string `gorm:"column:uuid;uniqueIndex"`
	DeviceKey string `gorm:"column:device_key;index"`
	Name      string
	Backend   string
	MAC       string `gorm:"column:mac"`
	Status    string
}
