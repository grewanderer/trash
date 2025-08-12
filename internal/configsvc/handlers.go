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
		http.Error(w, "invalid json", 400)
		return
	}
	items := make([]ReorderItem, 0, len(in.Items))
	for _, it := range in.Items {
		items = append(items, ReorderItem{ID: it.ID, Order: it.Order})
	}
	if err := h.repo.ReorderDeviceTemplates(uuid, items); err != nil {
		http.Error(w, err.Error(), 500)
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
