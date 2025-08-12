package configsvc

import (
	"errors"
	"sort"
	"strings"
	"wisp/internal/models"

	"gorm.io/gorm"
)

// ── Groups CRUD ─────────────────────────────────────────────
func (r *Repo) DeleteGroup(id uint) error { return r.db.Delete(&models.Group{}, id).Error }
func (r *Repo) ListGroups() ([]models.Group, error) {
	var out []models.Group
	err := r.db.Order("id").Find(&out).Error
	return out, err
}
func (r *Repo) GetGroup(id uint) (*models.Group, error) {
	var g models.Group
	if err := r.db.First(&g, id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// ── Membership ─────────────────────────────────────────────

var ErrDuplicate = errors.New("duplicate")

func isDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "duplicate entry") || strings.Contains(s, "unique constraint")
}

func (r *Repo) CreateGroup(g *models.Group) error {
	if err := r.db.Create(g).Error; err != nil {
		if isDuplicateErr(err) {
			var ex models.Group
			if e2 := r.db.Where("name = ?", g.Name).First(&ex).Error; e2 == nil {
				*g = ex
				return ErrDuplicate
			}
		}
		return err
	}
	return nil
}

func (r *Repo) AddDeviceToGroup(uuid string, groupID uint) (models.DeviceGroup, bool, error) {
	var link models.DeviceGroup
	tx := r.db.Where(&models.DeviceGroup{DeviceUUID: uuid, GroupID: groupID}).First(&link)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			link = models.DeviceGroup{DeviceUUID: uuid, GroupID: groupID, IsPrimary: false}
			return link, true, r.db.Create(&link).Error
		}
		return models.DeviceGroup{}, false, tx.Error
	}
	return link, false, nil
}

func (r *Repo) GetDeviceGroups(uuid string) ([]models.Group, error) {
	var gs []models.Group
	err := r.db.
		Table("groups").
		Joins("JOIN device_groups dg ON dg.group_id = groups.id AND dg.deleted_at IS NULL").
		Where("dg.device_uuid = ?", uuid).
		Order("groups.id").
		Find(&gs).Error
	return gs, err
}

func (r *Repo) GroupExists(id uint) bool {
	var g models.Group
	return r.db.Select("id").First(&g, id).Error == nil
}

// ── Group variables ─────────────────────────────────────────

func (r *Repo) UpsertGroupVar(groupID uint, key, value string) error {
	var gv models.GroupVariable
	tx := r.db.Where(&models.GroupVariable{GroupID: groupID, VarKey: key}).First(&gv)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			gv = models.GroupVariable{GroupID: groupID, VarKey: key, Value: value}
			return r.db.Create(&gv).Error
		}
		return tx.Error
	}
	gv.Value = value
	return r.db.Save(&gv).Error
}

func (r *Repo) GetGroupVars(groupIDs []uint) (map[string]string, error) {
	if len(groupIDs) == 0 {
		return map[string]string{}, nil
	}
	var list []models.GroupVariable
	if err := r.db.Where("group_id IN ?", groupIDs).Find(&list).Error; err != nil {
		return nil, err
	}
	out := map[string]string{}
	// детерминированный мердж по group_id
	sort.Slice(list, func(i, j int) bool { return list[i].GroupID < list[j].GroupID })
	for _, v := range list {
		out[v.VarKey] = v.Value
	}
	return out, nil
}

func (r *Repo) RemoveDeviceFromGroup(uuid string, groupID uint) error {
	return r.db.Where(&models.DeviceGroup{DeviceUUID: uuid, GroupID: groupID}).
		Delete(&models.DeviceGroup{}).Error
}

// ── Group template assignments ──────────────────────────────

func (r *Repo) AssignTemplateToGroup(groupID, templateID uint, enabled bool) error {
	var a models.GroupTemplateAssignment
	tx := r.db.Where("group_id = ? AND template_id = ?", groupID, templateID).First(&a)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			a = models.GroupTemplateAssignment{GroupID: groupID, TemplateID: templateID, Enabled: enabled}
			return r.db.Create(&a).Error
		}
		return tx.Error
	}
	a.Enabled = enabled
	return r.db.Save(&a).Error
}

func (r *Repo) ListGroupAssignments(groupID uint) ([]models.GroupTemplateAssignment, error) {
	var out []models.GroupTemplateAssignment
	err := r.db.Where("group_id = ? AND enabled = TRUE", groupID).Order("id").Find(&out).Error
	return out, err
}

func (r *Repo) TemplatesByGroupIDs(groupIDs []uint) ([]models.Template, error) {
	if len(groupIDs) == 0 {
		return []models.Template{}, nil
	}
	// Соберём все template_id из включённых привязок
	var gas []models.GroupTemplateAssignment
	if err := r.db.Where("group_id IN ? AND enabled = TRUE", groupIDs).Find(&gas).Error; err != nil {
		return nil, err
	}
	if len(gas) == 0 {
		return []models.Template{}, nil
	}
	tids := make([]uint, 0, len(gas))
	for _, a := range gas {
		tids = append(tids, a.TemplateID)
	}
	return r.TemplatesByIDs(tids)
}
