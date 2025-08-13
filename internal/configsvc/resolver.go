// internal/configsvc/resolver.go
package configsvc

import (
	"slices"
	"wisp/internal/models"
)

// ResolvedTemplatesForDevice возвращает шаблоны в порядке применения:
// 1) group-assignments (order,id ASC) для групп устройства
// 2) device-assignments  (order,id ASC) — перекрывают group по одинаковым путям
// 3) исключаются заблокированные (DeviceTemplateBlock)
func (r *Repo) ResolvedTemplatesForDevice(uuid string) ([]models.Template, error) {
	// блокировки
	var blocks []models.DeviceTemplateBlock
	_ = r.db.Where("device_uuid = ?", uuid).Find(&blocks).Error
	blocked := map[uint]struct{}{}
	for _, b := range blocks {
		blocked[b.TemplateID] = struct{}{}
	}

	// device assignments
	var das []models.DeviceTemplateAssignment
	_ = r.db.
		Where("device_uuid = ? AND enabled = 1", uuid).
		Order("`order` ASC, id ASC").
		Find(&das).Error

	// group assignments
	gids, _ := r.GetGroupIDs(uuid)
	var gas []models.GroupTemplateAssignment
	if len(gids) > 0 {
		_ = r.db.
			Where("group_id IN ? AND enabled = 1", gids).
			Order("`order` ASC, id ASC").
			Find(&gas).Error
	}

	// соберём итоговый список templateID: сначала группы, затем устройство
	orderIDs := make([]uint, 0, len(gas)+len(das))
	seen := map[uint]struct{}{}

	for _, a := range gas {
		if _, off := blocked[a.TemplateID]; off {
			continue
		}
		if _, ok := seen[a.TemplateID]; !ok {
			seen[a.TemplateID] = struct{}{}
			orderIDs = append(orderIDs, a.TemplateID)
		}
	}
	for _, a := range das {
		if _, off := blocked[a.TemplateID]; off {
			continue
		}
		if _, ok := seen[a.TemplateID]; !ok {
			seen[a.TemplateID] = struct{}{}
			orderIDs = append(orderIDs, a.TemplateID)
		}
	}

	// загрузим шаблоны в сохранённом порядке
	if len(orderIDs) == 0 {
		return nil, nil
	}
	var ts []models.Template
	_ = r.db.Where("id IN ?", orderIDs).Find(&ts).Error
	// восстановим порядок как в orderIDs
	slices.SortFunc(ts, func(a, b models.Template) int {
		ia, ib := -1, -1
		for i, id := range orderIDs {
			if id == a.ID {
				ia = i
			}
			if id == b.ID {
				ib = i
			}
		}
		switch {
		case ia < ib:
			return -1
		case ia > ib:
			return 1
		default:
			return 0
		}
	})
	return ts, nil
}
