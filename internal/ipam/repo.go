package ipam

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"wisp/internal/models"

	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// CreateRootPrefix — создаёт корневой префикс (без родителя).
func (r *Repo) CreateRootPrefix(cidr string, note string) (*models.Prefix, error) {
	cidr = strings.TrimSpace(cidr)
	ip, nw, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	family := "ipv4"
	if ip.To4() == nil {
		family = "ipv6"
	} // пока не обрабатываем v6 в калькуляторе
	p := &models.Prefix{CIDR: nw.String(), ParentID: nil, Family: family, Note: note}
	return p, r.db.Create(p).Error
}

// GetPrefix — получить префикс по ID.
func (r *Repo) GetPrefix(id uint) (*models.Prefix, error) {
	var p models.Prefix
	if err := r.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListChildren — перечислить дочерние префиксы.
func (r *Repo) ListChildren(parentID uint) ([]models.Prefix, error) {
	var out []models.Prefix
	err := r.db.Where("parent_id = ?", parentID).Order("id").Find(&out).Error
	return out, err
}

// AllocateChild — выделить следующий свободный дочерний префикс заданной длины.
// Реализация для IPv4. Для IPv6 вернёт ошибку.
func (r *Repo) AllocateChild(parentID uint, newPrefixLen int, note string) (*models.Prefix, error) {
	parent, err := r.GetPrefix(parentID)
	if err != nil {
		return nil, err
	}

	_, parentNet, err := net.ParseCIDR(parent.CIDR)
	if err != nil {
		return nil, err
	}

	// IPv6 пока не поддерживаем в алгоритме деления
	if parentNet.IP.To4() == nil {
		return nil, errors.New("ipv6 division not implemented yet")
	}

	parentOnes, _ := parentNet.Mask.Size()
	if newPrefixLen <= parentOnes || newPrefixLen > 32 {
		return nil, fmt.Errorf("invalid new_prefix_len: %d", newPrefixLen)
	}

	// Сколько уже выделено детей этой длины
	var existing []models.Prefix
	if err := r.db.Where("parent_id = ?", parentID).Find(&existing).Error; err != nil {
		return nil, err
	}

	// вычислим следующий свободный субпрефикс
	size := 1 << uint(32-newPrefixLen)
	start := ip4ToUint(parentNet.IP.To4())
	end := start + (1 << uint(32-parentOnes)) - 1

	// отсортированный список занятых сетевых адресов детей этой длины
	occupied := map[uint32]bool{}
	for _, c := range existing {
		_, n, e := net.ParseCIDR(c.CIDR)
		if e != nil {
			continue
		}
		ones, _ := n.Mask.Size()
		if ones == newPrefixLen {
			occupied[ip4ToUint(n.IP.To4())] = true
		}
	}

	for netAddr := start; netAddr+uint32(size)-1 <= end; netAddr += uint32(size) {
		if !occupied[netAddr] {
			child := &models.Prefix{
				CIDR:     fmt.Sprintf("%s/%d", uintToIP4(netAddr).String(), newPrefixLen),
				ParentID: &parentID,
				Family:   "ipv4",
				Note:     note,
			}
			if err := r.db.Create(child).Error; err != nil {
				return nil, err
			}
			return child, nil
		}
	}
	return nil, errors.New("no free child prefix available")
}

// AssignPrefixToGroup — выделяет следующий свободный дочерний префикс у parent и назначает группе.
func (r *Repo) AssignPrefixToGroup(parentID uint, groupID uint, newPrefixLen int, note string) (*models.Prefix, error) {
	child, err := r.AllocateChild(parentID, newPrefixLen, note)
	if err != nil {
		return nil, err
	}
	gp := &models.GroupPrefix{GroupID: groupID, PrefixID: child.ID}
	if err := r.db.Create(gp).Error; err != nil {
		return nil, err
	}
	return child, nil
}

// GroupPrefixes — список префиксов, привязанных к группе.
func (r *Repo) GroupPrefixes(groupID uint) ([]models.Prefix, error) {
	var links []models.GroupPrefix
	if err := r.db.Where("group_id = ?", groupID).Find(&links).Error; err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return []models.Prefix{}, nil
	}
	ids := make([]uint, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.PrefixID)
	}
	var out []models.Prefix
	if err := r.db.Where("id IN ?", ids).Order("id").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// FirstGroupPrefix — первый префикс группы, если есть (удобно для шаблонов).
func (r *Repo) FirstGroupPrefix(groupID uint) (*models.Prefix, error) {
	ps, err := r.GroupPrefixes(groupID)
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &ps[0], nil
}

// Helpers IPv4
func ip4ToUint(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}
func uintToIP4(u uint32) net.IP {
	return net.IPv4(byte(u>>24), byte(u>>16), byte(u>>8), byte(u))
}

// AssignIPToDeviceByGroup — выбрать первый префикс группы и выдать след. свободный IP.
func (r *Repo) AssignIPToDeviceByGroup(groupID uint, deviceUUID string) (*models.DeviceIP, error) {
	pfx, err := r.FirstGroupPrefix(groupID)
	if err != nil {
		return nil, err
	}
	return r.assignIPInPrefix(pfx, deviceUUID)
}

// DeviceIPs — список IP устройства.
func (r *Repo) DeviceIPs(deviceUUID string) ([]models.DeviceIP, error) {
	var out []models.DeviceIP
	err := r.db.Where("device_uuid = ?", deviceUUID).Order("id").Find(&out).Error
	return out, err
}

// ReleaseDeviceIP — снять назначение IP (удалить запись).
func (r *Repo) ReleaseDeviceIP(id uint) error {
	return r.db.Delete(&models.DeviceIP{}, id).Error
}

// internal: выдача IP внутри конкретного префикса (IPv4).
func (r *Repo) assignIPInPrefix(pfx *models.Prefix, deviceUUID string) (*models.DeviceIP, error) {
	ip, nw, err := net.ParseCIDR(pfx.CIDR)
	if err != nil {
		return nil, err
	}
	if ip.To4() == nil {
		return nil, errors.New("ipv6 allocation not implemented yet")
	}
	ones, bits := nw.Mask.Size()
	if bits != 32 {
		return nil, errors.New("unexpected mask size")
	}
	// границы хостов (reserve .0 network, .1 gateway, .255 broadcast)
	netU := ip4ToUint(nw.IP.To4())
	first := netU + 2                       // .1 — gateway, начинаем с .2
	last := netU + (1 << uint(32-ones)) - 2 // -1 broadcast, -1 ещё для last usable

	// занятые адреса
	var taken []models.DeviceIP
	if err := r.db.Where("prefix_id = ?", pfx.ID).Find(&taken).Error; err != nil {
		return nil, err
	}
	occ := map[uint32]bool{
		netU:                            true,
		netU + 1:                        true, // gateway
		netU + (1 << uint(32-ones)) - 1: true, // broadcast
	}
	for _, t := range taken {
		ip := net.ParseIP(t.Address).To4()
		if ip != nil {
			occ[ip4ToUint(ip)] = true
		}
	}

	for u := first; u <= last; u++ {
		if !occ[u] {
			addr := uintToIP4(u).String()
			rec := &models.DeviceIP{
				DeviceUUID: deviceUUID,
				PrefixID:   pfx.ID,
				Address:    addr,
			}
			if err := r.db.Create(rec).Error; err != nil {
				return nil, err
			}
			return rec, nil
		}
	}
	return nil, errors.New("no free ip in prefix")
}
