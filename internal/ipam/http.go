package ipam

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type HTTP struct{ repo *Repo }

func NewHTTP(r *Repo) *HTTP { return &HTTP{repo: r} }

func (h *HTTP) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1/ipam").Subrouter()

	// Root prefix
	// POST /api/v1/ipam/prefixes  { cidr, note }
	api.HandleFunc("/prefixes", h.createRootPrefix).Methods(http.MethodPost)

	// Allocate child from parent
	// POST /api/v1/ipam/prefixes/{id}/allocate?new_prefix_len=24&note=...
	api.HandleFunc("/prefixes/{id}/allocate", h.allocateChild).Methods(http.MethodPost)

	// Assign next free child to group
	// POST /api/v1/ipam/assign/group/{groupID}?parent={parentID}&len=24&note=...
	api.HandleFunc("/assign/group/{groupID}", h.assignToGroup).Methods(http.MethodPost)

	// List group prefixes
	// GET /api/v1/ipam/groups/{groupID}/prefixes
	api.HandleFunc("/groups/{groupID}/prefixes", h.groupPrefixes).Methods(http.MethodGet)
}

func (h *HTTP) createRootPrefix(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var in struct {
		CIDR string `json:"cidr"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.CIDR == "" {
		http.Error(w, "invalid body (need {cidr, note})", http.StatusBadRequest)
		return
	}
	p, err := h.repo.CreateRootPrefix(in.CIDR, in.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (h *HTTP) allocateChild(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idU, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err != nil || idU == 0 {
		http.Error(w, "invalid parent id", http.StatusBadRequest)
		return
	}
	newLen, err := strconv.ParseInt(r.URL.Query().Get("new_prefix_len"), 10, 64)
	if err != nil || newLen == 0 {
		http.Error(w, "new_prefix_len required", http.StatusBadRequest)
		return
	}
	note := r.URL.Query().Get("note")
	p, e := h.repo.AllocateChild(uint(idU), int(newLen), note)
	if e != nil {
		http.Error(w, e.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (h *HTTP) assignToGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	groupU, err := strconv.ParseUint(mux.Vars(r)["groupID"], 10, 64)
	if err != nil || groupU == 0 {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}
	parentU, err := strconv.ParseUint(r.URL.Query().Get("parent"), 10, 64)
	if err != nil || parentU == 0 {
		http.Error(w, "parent query param required", http.StatusBadRequest)
		return
	}
	lenU, err := strconv.ParseInt(r.URL.Query().Get("len"), 10, 64)
	if err != nil || lenU == 0 {
		http.Error(w, "len query param required", http.StatusBadRequest)
		return
	}
	note := r.URL.Query().Get("note")
	p, e := h.repo.AssignPrefixToGroup(uint(parentU), uint(groupU), int(lenU), note)
	if e != nil {
		http.Error(w, e.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (h *HTTP) groupPrefixes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	groupU, err := strconv.ParseUint(mux.Vars(r)["groupID"], 10, 64)
	if err != nil || groupU == 0 {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}
	ps, e := h.repo.GroupPrefixes(uint(groupU))
	if e != nil {
		http.Error(w, e.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(ps)
}
