package models

import "gorm.io/gorm"

type Template struct {
	gorm.Model
	Name string
	Path string
	Body string
}

type DeviceVariable struct {
	gorm.Model
	DeviceUUID string `gorm:"index:idx_devvar,priority:1"`
	VarKey     string `gorm:"column:var_key;index:idx_devvar,priority:2"`
	Value      string
}

type DeviceTemplateAssignment struct {
	gorm.Model
	DeviceUUID string `gorm:"index;size:36"`
	TemplateID uint   `gorm:"index"`
	Enabled    bool   `gorm:"default:true"`
	Order      int    `gorm:"default:100;index:idx_assign_order"`
}

type Group struct {
	gorm.Model
	Name string `gorm:"uniqueIndex"`
}

type DeviceGroup struct {
	gorm.Model
	DeviceUUID string `gorm:"index"`
	GroupID    uint   `gorm:"index"`
	IsPrimary  bool   `gorm:"column:is_primary"` // раньше было Primary bool
}

type GroupVariable struct {
	gorm.Model
	GroupID uint   `gorm:"index"`
	VarKey  string `gorm:"column:var_key;index:idx_group_key,priority:1"`
	Value   string
}

type GroupTemplateAssignment struct {
	gorm.Model
	GroupID    uint `gorm:"index"`
	TemplateID uint `gorm:"index"`
	Enabled    bool
}
