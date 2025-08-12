package configsvc

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"wisp/internal/models"

	"github.com/gorilla/mux"
)

type GroupHTTP struct{ repo *Repo }

func NewGroupHTTP(r *Repo) *GroupHTTP { return &GroupHTTP{repo: r} }

func (h *GroupHTTP) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	// groups CRUD (минимум — create + list)
	api.HandleFunc("/groups", h.createGroup).Methods(http.MethodPost)
	api.HandleFunc("/groups", h.listGroups).Methods(http.MethodGet)

	// membership
	api.HandleFunc("/devices/{uuid}/groups/{id}", h.addMembership).Methods(http.MethodPost)
	api.HandleFunc("/devices/{uuid}/groups/{id}", h.removeMembership).Methods(http.MethodDelete)
	api.HandleFunc("/devices/{uuid}/groups", h.deviceGroups).Methods(http.MethodGet)

	// group vars
	api.HandleFunc("/groups/{id}/vars", h.upsertGroupVar).Methods(http.MethodPost)
	api.HandleFunc("/groups/{id}/vars", h.getGroupVars).Methods(http.MethodGet)
}

func (h *GroupHTTP) createGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		http.Error(w, "invalid json or empty name", 400)
		return
	}
	g := &models.Group{Name: in.Name}
	err := h.repo.CreateGroup(g)
	w.Header().Set("Content-Type", "application/json")
	switch {
	case err == nil:
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(g)
	case errors.Is(err, ErrDuplicate):
		// возвращаем существующую группу 200, чтобы UI взял её id
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(g)
	default:
		http.Error(w, err.Error(), 500)
	}
}

func (h *GroupHTTP) listGroups(w http.ResponseWriter, _ *http.Request) {
	gs, err := h.repo.ListGroups()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(gs)
}

func (h *GroupHTTP) addMembership(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	idU, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err != nil || idU == 0 || !h.repo.GroupExists(uint(idU)) {
		http.Error(w, "bad or unknown group id", 400)
		return
	}
	link, created, err := h.repo.AddDeviceToGroup(uuid, uint(idU))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(w).Encode(link)
}

func (h *GroupHTTP) removeMembership(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	idU, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err := h.repo.RemoveDeviceFromGroup(uuid, uint(idU)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *GroupHTTP) deviceGroups(w http.ResponseWriter, r *http.Request) {
	uuid := mux.Vars(r)["uuid"]
	gs, err := h.repo.GetDeviceGroups(uuid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(gs)
}
func (h *GroupHTTP) upsertGroupVar(w http.ResponseWriter, r *http.Request) {
	idU, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	var in struct{ Key, Value string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Key == "" {
		http.Error(w, "invalid json", 400)
		return
	}
	if err := h.repo.UpsertGroupVar(uint(idU), in.Key, in.Value); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *GroupHTTP) getGroupVars(w http.ResponseWriter, r *http.Request) {
	idU, _ := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	m, err := h.repo.GetGroupVars([]uint{uint(idU)})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(m)
}
