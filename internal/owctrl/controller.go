package owctrl

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"wisp/internal/configsvc/varschema"
	"wisp/internal/models"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

/*
OpenWISP-compatible endpoints for the OpenWrt agent:

POST /controller/register/
GET  /controller/checksum/{uuid}/?key=...
GET  /controller/download-config/{uuid}/?key=...
POST /controller/report-status/{uuid}/  (form: key=...&status=running|error)

All responses must include header:
    X-Openwisp-Controller: true
*/

// DeviceFields — DTO, с которым работает контроллер.
type DeviceFields struct {
	UUID      string
	Key       string
	Name      string
	Backend   string
	MAC       string
	Status    string
	LastSeen  time.Time
	LastError string
	LastSHA   string
	UpdatedAt time.Time
}

type GlobalVarsProvider interface {
	GlobalVars() map[string]string
}

// Store — контракт хранилища устройств.
type Store interface {
	UpsertByKey(key string, d DeviceFields) (DeviceFields, bool)
	FindByUUID(id string) (DeviceFields, bool)
	UpdateStatus(id, status string) error
	UpdateStatusDetail(id, status, configSHA, errMsg string, facts map[string]any) error
}

// ConfigBuilder — контракт сборщика конфигурации устройства.
type ConfigBuilder interface {
	// Возвращает набор файлов конфигурации: абсолютный путь в tar -> содержимое.
	BuildConfig(d DeviceFields) (map[string]string, error)
}

type VarsProvider interface {
	GetDeviceVars(uuid string) (map[string]string, error)
	GetGroupIDs(uuid string) ([]uint, error)
	GetGroupVars(ids []uint) (map[string]string, error)
}

type IPAMProvider interface {
	GetDeviceIP(uuid string) (models.DeviceIP, bool, error) // верните хотя бы адрес и prefix cidr
	GetPrefixByID(id uint) (models.Prefix, bool, error)
}

// TemplateBuildAdapter — обёртка, которая делает наш Builder совместимым с ConfigBuilder.
type TemplateBuildAdapter struct{ B *Builder }

func (a *TemplateBuildAdapter) BuildConfig(d DeviceFields) (map[string]string, error) {
	// наш Builder рендерит по UUID, vars он соберёт сам через repo/ipam
	return a.B.RenderFiles(d.UUID)
}

// Builder имеет ссылки на repo и ipam
type Builder struct {
	repo    VarsProvider
	ipam    IPAMProvider
	tpl     TemplateRenderer
	gvars   GlobalVarsProvider
	tplrepo TemplateRepository
}

type TemplateRepository interface {
	ListRequiredTemplates() ([]models.Template, error)
	ListGroupTemplates(groupIDs []uint) ([]models.GroupTemplateAssignment, error)
	ListAssignments(uuid string) ([]models.DeviceTemplateAssignment, error)
	ListDeviceTemplateBlocks(uuid string) (map[uint]struct{}, error)
	GetTemplatesByIDs(ids []uint) (map[uint]models.Template, error)
}

func NewBuilder(repo VarsProvider, ip IPAMProvider, tpl TemplateRenderer, g GlobalVarsProvider) *Builder {
	return &Builder{repo: repo, ipam: ip, tpl: tpl, gvars: g}
}

func (b *Builder) collectTemplates(uuid string) ([]models.Template, error) {
	// 1) required
	req, _ := b.tplrepo.ListRequiredTemplates()
	// 2) group (с учётом блок-листа)
	gids, _ := b.repo.GetGroupIDs(uuid)
	gas, _ := b.tplrepo.ListGroupTemplates(gids)
	blocks, _ := b.tplrepo.ListDeviceTemplateBlocks(uuid)
	groupIDs := make([]uint, 0, len(gas))
	for _, a := range gas {
		if _, blocked := blocks[a.TemplateID]; blocked {
			continue
		}
		groupIDs = append(groupIDs, a.TemplateID)
	}
	// 3) device
	das, _ := b.tplrepo.ListAssignments(uuid)
	// собираем полный список ID
	ids := make([]uint, 0, len(req)+len(groupIDs)+len(das))
	for _, t := range req {
		ids = append(ids, t.ID)
	}
	ids = append(ids, groupIDs...)
	for _, a := range das {
		ids = append(ids, a.TemplateID)
	}
	// загружаем карты ID->Template
	byID, _ := b.tplrepo.GetTemplatesByIDs(ids)
	// финальная упорядоченная последовательность:
	out := make([]models.Template, 0, len(ids))

	// required: как есть, по id ASC (мы уже вытянули в порядке id)
	out = append(out, req...)

	// group: сортировка уже учтена в запросе по order asc, id asc
	for _, a := range gas {
		if _, blocked := blocks[a.TemplateID]; blocked {
			continue
		}
		if t, ok := byID[a.TemplateID]; ok {
			out = append(out, t)
		}
	}

	// device: сортировка по order asc, id asc
	for _, a := range das {
		if t, ok := byID[a.TemplateID]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (c *Controller) handleDebugConfig(w http.ResponseWriter, r *http.Request) {
	c.setOWHeader(w)
	id := mux.Vars(r)["uuid"]
	key := r.URL.Query().Get("key")

	dev, ok := c.store.FindByUUID(id)
	if !ok {
		models.WriteProblem(w, http.StatusNotFound, "Not found", "device not found", map[string]string{"uuid": id})
		return
	}
	if key == "" || key != dev.Key {
		models.WriteProblem(w, http.StatusForbidden, "Forbidden", "invalid key", nil)
		return
	}

	files, err := c.buildFiles(dev)
	if err != nil {
		models.WriteProblem(w, http.StatusUnprocessableEntity, "Build failed", err.Error(), nil)
		return
	}

	tgz := mustTarGz(files)
	sum := sha256.Sum256(tgz)
	shaHex := hex.EncodeToString(sum[:])

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	type fileInfo struct {
		Path    string `json:"path"`
		Size    int    `json:"size"`
		Preview string `json:"preview"`
	}
	out := struct {
		SHA256 string     `json:"sha256"`
		Files  []fileInfo `json:"files"`
	}{SHA256: shaHex}

	for _, p := range paths {
		body := files[p]
		prev := body
		if len(prev) > 300 {
			prev = prev[:300] + "...(truncated)"
		}
		out.Files = append(out.Files, fileInfo{Path: p, Size: len(body), Preview: prev})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

func (m *memStore) UpdateStatusDetail(id, st, sha, errMsg string, _ map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.byUUID[id]
	if !ok {
		return errors.New("not found")
	}
	d.Status = st
	d.LastSeen = time.Now()
	if sha != "" {
		d.LastSHA = sha
	}
	if errMsg != "" {
		d.LastError = errMsg
	}
	d.UpdatedAt = time.Now()
	m.byUUID[id] = d
	return nil
}

func (b *Builder) mergeVars(uuid string) (map[string]string, error) {
	merged := map[string]string{}

	// 0) global
	if b.gvars != nil {
		for k, v := range b.gvars.GlobalVars() {
			merged[k] = v
		}
	}

	// 1) group
	groupIDs, _ := b.repo.GetGroupIDs(uuid)
	gvars, _ := b.repo.GetGroupVars(groupIDs)
	for k, v := range gvars {
		merged[k] = v
	}

	// 2) device
	dvars, _ := b.repo.GetDeviceVars(uuid)
	for k, v := range dvars {
		merged[k] = v
	}

	// 3) derived from IPAM if missing
	if _, ok := merged["ipv4_address"]; !ok {
		if dip, ok2, _ := b.ipam.GetDeviceIP(uuid); ok2 {
			merged["ipv4_address"] = dip.Address

			// подтянем префикс по PrefixID, чтобы получить маску и вычислить шлюз
			if pfx, ok3, _ := b.ipam.GetPrefixByID(dip.PrefixID); ok3 {
				ip, ipnet, _ := net.ParseCIDR(pfx.CIDR)
				if ipnet != nil {
					// netmask
					if _, ok := merged["ipv4_netmask"]; !ok {
						merged["ipv4_netmask"] = net.IP(ipnet.Mask).String()
					}
					// gateway: network + 1
					if _, ok := merged["ipv4_gateway"]; !ok {
						base := ip.Mask(ipnet.Mask).To4()
						if base != nil {
							gw := make(net.IP, len(base))
							copy(gw, base)
							gw[3] += 1
							merged["ipv4_gateway"] = gw.String()
						}
					}
				}
			}
		}
	}

	// 4) final validation: ensure required are present and valid
	missing := []string{}
	for _, def := range varschema.Catalog {
		if !def.Required {
			continue
		}
		if v, ok := merged[def.Key]; !ok || v == "" {
			missing = append(missing, def.Key)
		} else {
			// re-validate normalized form
			if nv, err := varschema.ValidateOne(def.Key, v); err != nil {
				return nil, fmt.Errorf("invalid %s: %v", def.Key, err)
			} else {
				merged[def.Key] = nv
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required vars: %v", missing)
	}

	return merged, nil
}

// BuildConfig satisfies ConfigBuilder
func (b *Builder) BuildConfig(d DeviceFields) (map[string]string, error) {
	vars, err := b.mergeVars(d.UUID)
	if err != nil {
		return nil, err
	}
	vars["id"] = d.UUID
	vars["key"] = d.Key
	vars["name"] = d.Name
	if d.MAC != "" {
		vars["mac_address"] = d.MAC
	}
	if v := strings.TrimSpace(vars["hostname"]); v == "" && d.Name != "" {
		vars["hostname"] = d.Name
	}

	tpls, err := b.collectTemplates(d.UUID)
	if err != nil {
		return nil, fmt.Errorf("collect templates: %w", err)
	}

	// отрисовываем по порядку, позже перекрывает ранее созданный файл по тому же path
	files := map[string]string{}
	for _, t := range tpls {
		rendered, err := b.tpl.RenderOne(t, vars) // добавь метод; для Type="go" → Go-template; Type="netjson" → см. ниже
		if err != nil {
			return nil, fmt.Errorf("template %d (%s): %w", t.ID, t.Name, err)
		}
		// у шаблона один path; если «мультифайл» — верни map[path]body
		files[t.Path] = rendered
	}
	return files, nil
}

func (b *Builder) RenderFiles(uuid string) (map[string]string, error) {
	vars, err := b.mergeVars(uuid)
	if err != nil {
		return nil, fmt.Errorf("config build error: %w", err)
	}
	// отрендерить все назначенные шаблоны с vars
	return b.tpl.RenderAll(uuid, vars)
}

// ─────────────────────────── in-memory store (fallback) ───────────────────────────

type memStore struct {
	byUUID map[string]DeviceFields
	byKey  map[string]string // key -> uuid
	mu     sync.RWMutex
}

func NewMemStore() *memStore {
	return &memStore{
		byUUID: make(map[string]DeviceFields),
		byKey:  make(map[string]string),
	}
}

func (m *memStore) UpsertByKey(key string, d DeviceFields) (DeviceFields, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.byKey[key]; ok {
		ex := m.byUUID[id]
		if d.Name != "" {
			ex.Name = d.Name
		}
		if d.Backend != "" {
			ex.Backend = d.Backend
		}
		if d.MAC != "" {
			ex.MAC = d.MAC
		}
		ex.UpdatedAt = time.Now()
		m.byUUID[id] = ex
		return ex, false
	}
	if d.UUID == "" {
		d.UUID = uuid.NewString()
	}
	d.Key = key
	d.UpdatedAt = time.Now()
	m.byUUID[d.UUID] = d
	m.byKey[key] = d.UUID
	return d, true
}

func (m *memStore) FindByUUID(id string) (DeviceFields, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.byUUID[id]
	return d, ok
}

func (m *memStore) UpdateStatus(id, st string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.byUUID[id]
	if !ok {
		return errors.New("not found")
	}
	d.Status = st
	d.UpdatedAt = time.Now()
	m.byUUID[id] = d
	return nil
}

// ─────────────────────────── controller ───────────────────────────

type Controller struct {
	store        Store
	sharedSecret string
	builder      ConfigBuilder
}

func NewController(sharedSecret string) *Controller {
	return &Controller{
		store:        NewMemStore(),
		sharedSecret: sharedSecret,
	}
}

func NewControllerWithStore(sharedSecret string, store Store) *Controller {
	if store == nil {
		store = NewMemStore()
	}
	return &Controller{store: store, sharedSecret: sharedSecret}
}

func NewControllerWithStoreAndBuilder(sharedSecret string, store Store, builder ConfigBuilder) *Controller {
	if store == nil {
		store = NewMemStore()
	}
	return &Controller{store: store, sharedSecret: sharedSecret, builder: builder}
}

func (c *Controller) setOWHeader(w http.ResponseWriter) {
	w.Header().Set("X-Openwisp-Controller", "true")
}

// POST /controller/register/
func (c *Controller) handleRegister(w http.ResponseWriter, r *http.Request) {
	c.setOWHeader(w)
	if err := r.ParseForm(); err != nil {
		models.WriteProblem(w, http.StatusBadRequest, "Bad form", "cannot parse form", nil)
		return
	}
	secret := r.Form.Get("secret")
	if secret == "" || secret != c.sharedSecret {
		models.WriteProblem(w, http.StatusUnauthorized, "Unauthorized", "unrecognized secret", nil)
		return
	}

	name := r.Form.Get("name")
	backend := r.Form.Get("backend")
	mac := r.Form.Get("mac_address")
	key := r.Form.Get("key")
	if key == "" {
		mac := r.Form.Get("mac_address")
		sum := md5.Sum([]byte(mac + secret)) // OpenWISP совместимость
		key = hex.EncodeToString(sum[:])     // 32 hex
	}

	dev, isNew := c.store.UpsertByKey(key, DeviceFields{
		Name:    name,
		Backend: backend,
		MAC:     mac,
	})

	w.WriteHeader(http.StatusCreated)
	_, _ = io.WriteString(w,
		fmt.Sprintf("uuid: %s\nkey: %s\nhostname: %s\nis-new: %d\n",
			dev.UUID, key, dev.Name, btoi(isNew)))
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// GET /controller/checksum/{uuid}/?key=...
func (c *Controller) handleChecksum(w http.ResponseWriter, r *http.Request) {
	c.setOWHeader(w)
	id := mux.Vars(r)["uuid"]
	key := r.URL.Query().Get("key")

	dev, ok := c.store.FindByUUID(id)
	if !ok {
		models.WriteProblem(w, http.StatusNotFound, "Not found", "device not found", map[string]string{"uuid": id})
		return
	}
	if key == "" || key != dev.Key {
		models.WriteProblem(w, http.StatusForbidden, "Forbidden", "invalid key", nil)
		return
	}

	files, err := c.buildFiles(dev)
	if err != nil {
		models.WriteProblem(w, http.StatusUnprocessableEntity, "Build failed", err.Error(), map[string]string{
			"uuid": dev.UUID,
		})
		return
	}
	tgz := mustTarGz(files)
	sum := sha256.Sum256(tgz)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, hex.EncodeToString(sum[:])+"\n")
}

// GET /controller/download-config/{uuid}/?key=...
func (c *Controller) handleDownloadConfig(w http.ResponseWriter, r *http.Request) {
	c.setOWHeader(w)

	id := mux.Vars(r)["uuid"]
	key := r.URL.Query().Get("key")

	dev, ok := c.store.FindByUUID(id)
	if !ok {
		models.WriteProblem(w, http.StatusNotFound, "Not found", "device not found", map[string]string{"uuid": id})
		return
	}
	if key == "" || key != dev.Key {
		models.WriteProblem(w, http.StatusForbidden, "Forbidden", "invalid key", nil)
		return
	}

	files, err := c.buildFiles(dev)
	if err != nil {
		models.WriteProblem(w, http.StatusUnprocessableEntity, "Build failed", err.Error(), map[string]string{
			"uuid": dev.UUID,
		})
		return
	}
	tgz := mustTarGz(files)
	sum := sha256.Sum256(tgz)
	shaHex := hex.EncodeToString(sum[:])
	etag := `"` + shaHex + `"` // strong ETag

	// Если клиент прислал If-None-Match с тем же ETag — отдадим 304 без тела
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
		w.Header().Set("ETag", etag)
		w.Header().Set("X-Openwisp-Archive-Sha256", shaHex)
		w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("X-Openwisp-Archive-Sha256", shaHex)
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=configuration.tar.gz")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(tgz)
}

// POST /controller/report-status/{uuid}/  (form: key, status=running|error)
func (c *Controller) handleReportStatus(w http.ResponseWriter, r *http.Request) {
	c.setOWHeader(w)
	id := mux.Vars(r)["uuid"]

	ct := r.Header.Get("Content-Type")
	var key, status, configSHA, errLog string
	facts := map[string]any{}

	if strings.HasPrefix(ct, "application/json") {
		var in struct {
			Key       string         `json:"key"`
			Status    string         `json:"status"`
			ConfigSHA string         `json:"config_sha"`
			Error     string         `json:"error"`
			Log       string         `json:"log"`
			Facts     map[string]any `json:"facts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			models.WriteProblem(w, http.StatusBadRequest, "Bad JSON", err.Error(), nil)
			return
		}
		key = in.Key
		status = strings.ToLower(in.Status)
		status = normalizeStatus(status)

		if err := c.store.UpdateStatusDetail(id, status, configSHA, errLog, facts); err != nil {
			_ = c.store.UpdateStatus(id, status)
		}
		configSHA = in.ConfigSHA
		if in.Log != "" {
			errLog = in.Log
		} else {
			errLog = in.Error
		}
		if in.Facts != nil {
			facts = in.Facts
		}
	} else {
		if err := r.ParseForm(); err != nil {
			models.WriteProblem(w, http.StatusBadRequest, "Bad form", "cannot parse form", nil)
			return
		}
		key = r.Form.Get("key")
		status = strings.ToLower(r.Form.Get("status"))
		configSHA = r.Form.Get("config_sha")
		if v := r.Form.Get("log"); v != "" {
			errLog = v
		} else {
			errLog = r.Form.Get("error")
		}
	}

	dev, ok := c.store.FindByUUID(id)
	if !ok {
		models.WriteProblem(w, http.StatusNotFound, "Not found", "device not found", map[string]string{"uuid": id})
		return
	}
	if key == "" || key != dev.Key {
		models.WriteProblem(w, http.StatusForbidden, "Forbidden", "invalid key", nil)
		return
	}
	if status != "running" && status != "error" {
		models.WriteProblem(w, http.StatusBadRequest, "Bad status", "status must be running|error", nil)
		return
	}

	// сохраняем расширенно; если не реализовано — fallback на UpdateStatus
	if err := c.store.UpdateStatusDetail(id, status, configSHA, errLog, facts); err != nil {
		_ = c.store.UpdateStatus(id, status)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

// buildFiles — выбирает: использовать внешний билдер или минимальный fallback.
func (c *Controller) buildFiles(d DeviceFields) (map[string]string, error) {
	if c.builder != nil {
		return c.builder.BuildConfig(d)
	}
	// fallback: минимальный набор
	return map[string]string{
		"etc/config/system":                      "config system 'system'\n  option hostname '" + safe(d.Name) + "'\n  option timezone 'UTC'\n",
		"etc/openwisp/device.meta":               fmt.Sprintf("uuid=%s\nmac=%s\nbackend=%s\n", d.UUID, d.MAC, d.Backend),
		"etc/openwisp/managed_by_openwisp_go.md": "This device is managed by OpenWISP-Go controller.\n",
	}, nil
}

func safe(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "'", ""))
}

// tarGzFromMap — собирает tar.gz из карты файлов.
func tarGzFromMap(files map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		h := &tar.Header{
			Name:    name,
			Mode:    0644,
			Size:    int64(len(content)),
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(h); err != nil {
			_ = tw.Close()
			_ = gw.Close()
			return nil, err
		}
		if _, err := io.Copy(tw, strings.NewReader(content)); err != nil {
			_ = tw.Close()
			_ = gw.Close()
			return nil, err
		}
	}
	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes(), nil
}

func mustTarGz(files map[string]string) []byte {
	b, err := tarGzFromMap(files)
	if err != nil {
		// в реальном коде лучше логировать/оборачивать, но для совместимости вернём пустой архив
		return []byte{}
	}
	return b
}

// ─────────────────────────── route registrars ───────────────────────────

// RegisterRoutes — fast dev: in-memory store.
func RegisterRoutes(root *mux.Router, sharedSecret string) {
	ctrl := NewController(sharedSecret)
	sub := root.PathPrefix("/controller").Subrouter()
	sub.HandleFunc("/register/", ctrl.handleRegister).Methods(http.MethodPost)
	sub.HandleFunc("/checksum/{uuid}/", ctrl.handleChecksum).Methods(http.MethodGet)
	sub.HandleFunc("/download-config/{uuid}/", ctrl.handleDownloadConfig).Methods(http.MethodGet)
	sub.HandleFunc("/report-status/{uuid}/", ctrl.handleReportStatus).Methods(http.MethodPost)
	sub.HandleFunc("/debug-config/{uuid}/", ctrl.handleDebugConfig).Methods(http.MethodGet)
}

// RegisterRoutesWithStore — с внешним хранилищем (БД).
func RegisterRoutesWithStore(root *mux.Router, sharedSecret string, store Store) {
	ctrl := NewControllerWithStore(sharedSecret, store)
	sub := root.PathPrefix("/controller").Subrouter()
	sub.HandleFunc("/register/", ctrl.handleRegister).Methods(http.MethodPost)
	sub.HandleFunc("/checksum/{uuid}/", ctrl.handleChecksum).Methods(http.MethodGet)
	sub.HandleFunc("/download-config/{uuid}/", ctrl.handleDownloadConfig).Methods(http.MethodGet)
	sub.HandleFunc("/report-status/{uuid}/", ctrl.handleReportStatus).Methods(http.MethodPost)
	sub.HandleFunc("/debug-config/{uuid}/", ctrl.handleDebugConfig).Methods(http.MethodGet)
}

// RegisterRoutesWithStoreAndBuilder — с внешним хранилищем и сборщиком конфигов.
func RegisterRoutesWithStoreAndBuilder(root *mux.Router, sharedSecret string, store Store, builder ConfigBuilder) {
	ctrl := NewControllerWithStoreAndBuilder(sharedSecret, store, builder)
	sub := root.PathPrefix("/controller").Subrouter()
	sub.HandleFunc("/register/", ctrl.handleRegister).Methods(http.MethodPost)
	sub.HandleFunc("/checksum/{uuid}/", ctrl.handleChecksum).Methods(http.MethodGet)
	sub.HandleFunc("/download-config/{uuid}/", ctrl.handleDownloadConfig).Methods(http.MethodGet)
	sub.HandleFunc("/report-status/{uuid}/", ctrl.handleReportStatus).Methods(http.MethodPost)
	sub.HandleFunc("/debug-config/{uuid}/", ctrl.handleDebugConfig).Methods(http.MethodGet)
}

func normalizeStatus(s string) string {
	switch s {
	case "running", "applied", "ok", "success":
		return "applied"
	case "error", "failed", "rollbacked":
		return "error"
	case "deactivating":
		return "deactivating"
	default:
		return "pending" // changed, modified, queued и пр.
	}
}
