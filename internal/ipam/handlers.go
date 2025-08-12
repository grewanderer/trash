package ipam

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type DeviceHTTP struct{ repo *Repo }

func NewDeviceHTTP(r *Repo) *DeviceHTTP { return &DeviceHTTP{repo: r} }

func (h *DeviceHTTP) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1/ipam").Subrouter()

	api.HandleFunc("/assign/device/{uuid}", h.assignToDeviceByGroup).Methods(http.MethodPost)
	api.HandleFunc("/devices/{uuid}/ips", h.listDeviceIPs).Methods(http.MethodGet)
	api.HandleFunc("/devices/{uuid}/ips/{id}", h.releaseDeviceIP).Methods(http.MethodDelete)
}

func (h *DeviceHTTP) assignToDeviceByGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	uuid := mux.Vars(r)["uuid"]
	groupStr := r.URL.Query().Get("group")
	gidU, err := strconv.ParseUint(groupStr, 10, 64)
	if err != nil || gidU == 0 {
		http.Error(w, "group query param required (uint)", http.StatusBadRequest)
		return
	}

	rec, err := h.repo.AssignIPToDeviceByGroup(uint(gidU), uuid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated) // 201
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":          rec.ID,
		"device_uuid": rec.DeviceUUID,
		"prefix_id":   rec.PrefixID,
		"address":     rec.Address,
	})
}

func (h *DeviceHTTP) listDeviceIPs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	uuid := mux.Vars(r)["uuid"]
	recs, err := h.repo.DeviceIPs(uuid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	type out struct {
		ID         uint   `json:"id"`
		PrefixID   uint   `json:"prefix_id"`
		Address    string `json:"address"`
		PrefixCIDR string `json:"prefix_cidr"`
		Netmask    string `json:"netmask"`
		Gateway    string `json:"gateway"`
	}

	result := make([]out, 0, len(recs))
	for _, r := range recs {
		p, _ := h.repo.GetPrefix(r.PrefixID)
		var cidr, nm, gw string
		if p != nil {
			cidr = p.CIDR
			if _, nw, e := net.ParseCIDR(p.CIDR); e == nil {
				nm = net.IP(nw.Mask).String()
				gw = firstUsableIPv4(nw)
			}
		}
		result = append(result, out{
			ID:         r.ID,
			PrefixID:   r.PrefixID,
			Address:    r.Address,
			PrefixCIDR: cidr,
			Netmask:    nm,
			Gateway:    gw,
		})
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (h *DeviceHTTP) releaseDeviceIP(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	idU, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || idU == 0 {
		http.Error(w, "invalid ip id", http.StatusBadRequest)
		return
	}
	if err := h.repo.ReleaseDeviceIP(uint(idU)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// firstUsableIPv4 — такой же helper, как раньше
func firstUsableIPv4(nw *net.IPNet) string {
	ip := nw.IP.To4()
	if ip == nil {
		return ""
	}
	u := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	u++ // network + 1
	return net.IPv4(byte(u>>24), byte(u>>16), byte(u>>8), byte(u)).String()
}
