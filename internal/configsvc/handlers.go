package configsvc

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"wisp/internal/configsvc/varschema"
	"wisp/internal/models"

	"github.com/gorilla/mux"
)

type HTTP struct{ repo *Repo }

func NewHTTP(r *Repo) *HTTP { return &HTTP{repo: r} }

func (h *HTTP) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// templates
	api.HandleFunc("/templates", h.createTemplate).Methods(http.MethodPost)
	api.HandleFunc("/templates", h.listTemplates).Methods(http.MethodGet)
	api.HandleFunc("/templates/{id}", h.updateTemplate).Methods(http.MethodPut, http.MethodPatch)
	api.HandleFunc("/templates/{id}", h.deleteTemplate).Methods(http.MethodDelete)

	// device vars
	api.HandleFunc("/devices/{uuid}/vars", h.upsertVar).Methods(http.MethodPost)
	api.HandleFunc("/devices/{uuid}/vars", h.getVars).Methods(http.MethodGet)
	api.HandleFunc("/devices/{uuid}/vars/bulk", h.bulkUpsertVars).Methods(http.MethodPost)

	// assignments
	api.HandleFunc("/devices/{uuid}/templates/{id}", h.assignTemplate).Methods(http.MethodPost)
	api.HandleFunc("/devices/{uuid}/templates", h.listAssignments).Methods(http.MethodGet)
	api.HandleFunc("/devices/{uuid}/templates/order", h.reorderTemplates).Methods(http.MethodPut, http.MethodPost)

	api.HandleFunc("/groups/{id}/templates", h.assignTemplateToGroup).Methods(http.MethodPost)

	// DEVICE BLOCKS
	api.HandleFunc("/devices/{uuid}/templates/{id}/block", h.blockTpl).Methods(http.MethodDelete, http.MethodPost)
	api.HandleFunc("/devices/{uuid}/templates/{id}/unblock", h.unblockTpl).Methods(http.MethodDelete, http.MethodPost)

	// RESOLVED TEMPLATE LIST (по порядку: required→default→group→device)
	api.HandleFunc("/devices/{uuid}/templates/resolved", h.resolvedTemplates).Methods(http.MethodGet)
}

func (h *HTTP) assignTemplateToGroup(w http.ResponseWriter, r *http.Request) {
	gidU, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	var in struct {
		TemplateID uint  `json:"template_id"`
		Order      int   `json:"order"`
		Enabled    *bool `json:"enabled"`
	}
	enabled := true
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.TemplateID == 0 {
		http.Error(w, "template_id required", 400)
		return
	}
	if err := h.repo.AssignTemplateToGroup(uint(gidU), in.TemplateID, in.Order, enabled); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) blockTpl(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	idU, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err := h.repo.BlockTemplateForDevice(uuid, uint(idU)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) unblockTpl(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	idU, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err := h.repo.UnblockTemplateForDevice(uuid, uint(idU)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolved list на чтение (без рендера), полезно для UI
func (h *HTTP) resolvedTemplates(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]

	// required
	req, _ := h.repo.ListRequiredTemplates()

	// default
	def, _ := h.repo.ListDefaultTemplates()

	// group (с учётом блоков)
	gids, _ := h.repo.GetGroupIDs(uuid) // этот метод у тебя есть
	gas, _ := h.repo.ListGroupTemplates(gids)
	blocks, _ := h.repo.ListDeviceTemplateBlocks(uuid)
	gTplIDs := make([]uint, 0, len(gas))
	for _, a := range gas {
		if _, blocked := blocks[a.TemplateID]; blocked {
			continue
		}
		gTplIDs = append(gTplIDs, a.TemplateID)
	}

	// device
	das, _ := h.repo.ListAssignments(uuid)

	// собрать карты
	ids := make([]uint, 0, len(req)+len(def)+len(gTplIDs)+len(das))
	for _, t := range req {
		ids = append(ids, t.ID)
	}
	for _, t := range def {
		ids = append(ids, t.ID)
	}
	ids = append(ids, gTplIDs...)
	for _, a := range das {
		ids = append(ids, a.TemplateID)
	}
	byID, _ := h.repo.GetTemplatesByIDs(ids)

	type item struct {
		Source   string          `json:"source"` // required|default|group|device
		Order    int             `json:"order"`
		Template models.Template `json:"template"`
	}
	out := make([]item, 0, len(ids))

	for _, t := range req {
		out = append(out, item{Source: "required", Order: 0, Template: t})
	}
	for _, t := range def {
		out = append(out, item{Source: "default", Order: 0, Template: t})
	}
	for _, a := range gas {
		if _, blocked := blocks[a.TemplateID]; blocked {
			continue
		}
		if t, ok := byID[a.TemplateID]; ok {
			out = append(out, item{Source: "group", Order: a.Order, Template: t})
		}
	}
	for _, a := range das {
		if t, ok := byID[a.TemplateID]; ok {
			out = append(out, item{Source: "device", Order: a.Order, Template: t})
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (h *HTTP) createTemplate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t := &models.Template{Name: in.Name, Path: in.Path, Body: in.Body}
	if err := h.repo.CreateTemplate(t); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(t)
}

func (h *HTTP) listTemplates(w http.ResponseWriter, _ *http.Request) {
	ts, err := h.repo.ListTemplates()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(ts)
}

func (h *HTTP) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	t, err := h.repo.GetTemplate(uint(id))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	var in struct {
		Name *string `json:"name"`
		Path *string `json:"path"`
		Body *string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if in.Name != nil {
		t.Name = *in.Name
	}
	if in.Path != nil {
		t.Path = *in.Path
	}
	if in.Body != nil {
		t.Body = *in.Body
	}
	if err := h.repo.UpdateTemplate(t); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(t)
}

func (h *HTTP) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err := h.repo.DeleteTemplate(uint(id)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) upsertVar(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	var in struct{ Key, Value string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	in.Key = strings.TrimSpace(in.Key)
	if in.Key == "" {
		http.Error(w, "key required", 400)
		return
	}
	// validate/normalize
	val, err := varschema.ValidateOne(in.Key, in.Value)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if err := h.repo.UpsertDeviceVar(uuid, in.Key, val); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) bulkUpsertVars(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	var obj map[string]string
	if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	// validate all first
	type verr struct{ Key, Error string }
	var errs []verr
	norm := map[string]string{}
	for k, v := range obj {
		k2 := strings.TrimSpace(k)
		val, err := varschema.ValidateOne(k2, v)
		if err != nil {
			errs = append(errs, verr{Key: k2, Error: err.Error()})
			continue
		}
		norm[k2] = val
	}
	if len(errs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": errs})
		return
	}
	// upsert
	for k, v := range norm {
		if err := h.repo.UpsertDeviceVar(uuid, k, v); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) reorderTemplates(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	var in struct {
		Items []struct {
			ID    uint `json:"id"`
			Order int  `json:"order"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	items := make([]ReorderItem, 0, len(in.Items))
	for _, it := range in.Items {
		items = append(items, ReorderItem{ID: it.ID, Order: it.Order})
	}
	if err := h.repo.ReorderDeviceTemplates(uuid, items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) getVars(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	m, err := h.repo.GetDeviceVars(uuid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(m)
}

func (h *HTTP) assignTemplate(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	idU, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	enabled := true
	if err := json.NewDecoder(r.Body).Decode(&in); err == nil && in.Enabled != nil {
		enabled = *in.Enabled
	}
	if err := h.repo.AssignTemplate(uuid, uint(idU), enabled); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) listAssignments(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	as, err := h.repo.ListAssignments(uuid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(as)
}

// ListDefaultTemplates — вернуть шаблоны, помеченные как Default (применяются ко всем устройствам, если не переопределено).
func (r *Repo) ListDefaultTemplates() ([]models.Template, error) {
	var out []models.Template
	err := r.db.
		Where("`default` = ?", true).
		Order("id ASC").
		Find(&out).Error
	return out, err
}

// GetGroupIDs — ID всех групп, в которых состоит устройство.
func (r *Repo) GetGroupIDs(deviceUUID string) ([]uint, error) {
	var rows []models.DeviceGroup
	if err := r.db.
		Where("device_uuid = ?", deviceUUID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(rows))
	for _, dg := range rows {
		ids = append(ids, dg.GroupID)
	}
	return ids, nil
}
