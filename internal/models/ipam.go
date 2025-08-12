package models

import "gorm.io/gorm"

type Prefix struct {
	gorm.Model
	CIDR     string `gorm:"type:varchar(64);uniqueIndex"`
	ParentID *uint  `gorm:"index"`
	Family   string `gorm:"type:varchar(8)"`
	Note     string `gorm:"type:varchar(255)"`
}

type GroupPrefix struct {
	gorm.Model
	GroupID  uint `gorm:"index"`
	PrefixID uint `gorm:"uniqueIndex"` // префикс выдаётся одной сущности
}

type DeviceIP struct {
	gorm.Model
	DeviceUUID string `gorm:"type:char(36);index"`
	PrefixID   uint   `gorm:"index"`
	Address    string `gorm:"type:varchar(45);uniqueIndex"` // IPv4/IPv6
}
