package repo

import (
	"errors"
	"strings"
	"time"
	"wisp/internal/models"
	"wisp/internal/owctrl"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DeviceStore struct {
	db *gorm.DB
}

func NewDeviceStore(db *gorm.DB) *DeviceStore {
	return &DeviceStore{db: db}
}

// UpsertByKey — создаёт/обновляет устройство по ключу.
func (s *DeviceStore) UpsertByKey(key string, d owctrl.DeviceFields) (owctrl.DeviceFields, bool) {
	var m models.Device
	isNew := false

	// ищем устройство по device_key (GORM сам экранирует)
	tx := s.db.Where(&models.Device{DeviceKey: key}).First(&m)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			// не найдено — создаём
			isNew = true
			uid := strings.TrimSpace(d.UUID)
			if uid == "" {
				uid = uuid.NewString()
			}
			m = models.Device{
				UUID:      uid,
				DeviceKey: key, // ВАЖНО: использовать DeviceKey
				Name:      d.Name,
				Backend:   d.Backend,
				MAC:       d.MAC,
				Status:    "",
			}
			_ = s.db.Create(&m).Error
		} else {
			// неожиданная ошибка — считаем как "новое" и вернём то, что есть
			isNew = true
		}
	} else {
		// найдено — обновим изменяемые поля
		changed := false
		if d.Name != "" && d.Name != m.Name {
			m.Name = d.Name
			changed = true
		}
		if d.Backend != "" && d.Backend != m.Backend {
			m.Backend = d.Backend
			changed = true
		}
		if d.MAC != "" && d.MAC != m.MAC {
			m.MAC = d.MAC
			changed = true
		}
		if changed {
			_ = s.db.Save(&m).Error
		}
	}

	return owctrl.DeviceFields{
		UUID:      m.UUID,
		Key:       m.DeviceKey, // возвращаем ключ контроллеру
		Name:      strings.TrimSpace(m.Name),
		Backend:   m.Backend,
		MAC:       m.MAC,
		Status:    m.Status,
		UpdatedAt: m.UpdatedAt,
	}, isNew
}

func (s *DeviceStore) UpdateStatusDetail(uuid, st, sha, errMsg string, facts map[string]any) error {
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Device{}).
			Where("uuid = ?", uuid).
			Updates(map[string]any{
				"status": st, "last_seen": now,
				"last_config_sha": sha, "last_error": errMsg,
			}).Error; err != nil {
			return err
		}
		h := models.DeviceStatusHistory{DeviceUUID: uuid, Status: st, ConfigSHA: sha, Error: errMsg}
		if err := tx.Create(&h).Error; err != nil {
			return err
		}
		// facts можно сохранить в отдельную json-таблицу при необходимости
		return nil
	})
}

func (s *DeviceStore) FindByUUID(id string) (owctrl.DeviceFields, bool) {
	var m models.Device
	if err := s.db.Where("uuid = ?", id).First(&m).Error; err != nil {
		return owctrl.DeviceFields{}, false
	}
	return owctrl.DeviceFields{
		UUID:      m.UUID,
		Key:       m.DeviceKey, // ← ВАЖНО: раньше было m.Key
		Name:      m.Name,
		Backend:   m.Backend,
		MAC:       m.MAC,
		Status:    m.Status,
		UpdatedAt: m.UpdatedAt,
	}, true
}

func (s *DeviceStore) UpdateStatus(id, status string) error {
	return s.db.Model(&models.Device{}).Where("uuid = ?", id).Update("status", status).Error
}
