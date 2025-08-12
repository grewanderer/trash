package configsvc

import (
	"errors"
	"wisp/internal/models"

	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ── Templates CRUD ───────────────────────────────────────────

func (r *Repo) CreateTemplate(t *models.Template) error { return r.db.Create(t).Error }
func (r *Repo) UpdateTemplate(t *models.Template) error { return r.db.Save(t).Error }
func (r *Repo) DeleteTemplate(id uint) error            { return r.db.Delete(&models.Template{}, id).Error }
func (r *Repo) GetTemplate(id uint) (*models.Template, error) {
	var t models.Template
	if err := r.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}
func (r *Repo) ListTemplates() ([]models.Template, error) {
	var out []models.Template
	err := r.db.Order("id").Find(&out).Error
	return out, err
}

// ── Device variables ────────────────────────────────────────

func (r *Repo) UpsertDeviceVar(uuid, key, value string) error {
	var dv models.DeviceVariable
	tx := r.db.Where(&models.DeviceVariable{DeviceUUID: uuid, VarKey: key}).First(&dv)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			dv = models.DeviceVariable{DeviceUUID: uuid, VarKey: key, Value: value}
			return r.db.Create(&dv).Error
		}
		return tx.Error
	}
	dv.Value = value
	return r.db.Save(&dv).Error
}

func (r *Repo) GetDeviceVars(uuid string) (map[string]string, error) {
	var list []models.DeviceVariable
	if err := r.db.Where(&models.DeviceVariable{DeviceUUID: uuid}).
		Order("var_key"). // <— не `key`
		Find(&list).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(list))
	for _, v := range list {
		out[v.VarKey] = v.Value
	}
	return out, nil
}

// ── Assignments ────────────────────────────────────────────

// Отдаём назначения по устройству, отсортированные по order ASC, id ASC
func (r *Repo) ListAssignments(uuid string) ([]models.DeviceTemplateAssignment, error) {
	var as []models.DeviceTemplateAssignment
	err := r.db.
		Where("device_uuid = ? AND enabled = ?", uuid, true).
		Order("`order` ASC, id ASC").
		Find(&as).Error
	return as, err
}

type ReorderItem struct {
	ID    uint
	Order int
}

// Массовое изменение порядка шаблонов
func (r *Repo) ReorderDeviceTemplates(uuid string, items []ReorderItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			if err := tx.Model(&models.DeviceTemplateAssignment{}).
				Where("device_uuid = ? AND template_id = ?", uuid, it.ID).
				Update("order", it.Order).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repo) AssignTemplate(uuid string, templateID uint, enabled bool) error {
	var a models.DeviceTemplateAssignment
	tx := r.db.Where("device_uuid = ? AND template_id = ?", uuid, templateID).First(&a)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			a = models.DeviceTemplateAssignment{DeviceUUID: uuid, TemplateID: templateID, Enabled: enabled}
			return r.db.Create(&a).Error
		}
		return tx.Error
	}
	a.Enabled = enabled
	return r.db.Save(&a).Error
}

func (r *Repo) TemplatesByIDs(ids []uint) ([]models.Template, error) {
	if len(ids) == 0 {
		return []models.Template{}, nil
	}
	var out []models.Template
	err := r.db.Where("id IN ?", ids).Find(&out).Error
	return out, err
}

func (r *Repo) ListRequiredTemplates() ([]models.Template, error) {
	var t []models.Template
	err := r.db.Where("required = ?", true).Order("id ASC").Find(&t).Error
	return t, err
}

// group templates
func (r *Repo) ListGroupTemplates(groupIDs []uint) ([]models.GroupTemplateAssignment, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	var as []models.GroupTemplateAssignment
	err := r.db.
		Where("enabled = 1 AND group_id IN ?", groupIDs).
		Order("`order` ASC, id ASC").
		Find(&as).Error
	return as, err
}

// device blocks
func (r *Repo) ListDeviceTemplateBlocks(uuid string) (map[uint]struct{}, error) {
	var rows []models.DeviceTemplateBlock
	err := r.db.Where("device_uuid = ?", uuid).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	m := make(map[uint]struct{}, len(rows))
	for _, b := range rows {
		m[b.TemplateID] = struct{}{}
	}
	return m, nil
}

// helpers
func (r *Repo) GetTemplatesByIDs(ids []uint) (map[uint]models.Template, error) {
	if len(ids) == 0 {
		return map[uint]models.Template{}, nil
	}
	var ts []models.Template
	if err := r.db.Where("id IN ?", ids).Find(&ts).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]models.Template, len(ts))
	for _, t := range ts {
		out[t.ID] = t
	}
	return out, nil
}
