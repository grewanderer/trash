package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wisp/config"
	"wisp/internal/configsvc"
	"wisp/internal/db"
	"wisp/internal/health"
	"wisp/internal/ipam"
	"wisp/internal/logs"
	"wisp/internal/middleware"
	"wisp/internal/models"
	"wisp/internal/owctrl"
	"wisp/internal/repo"

	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

type App struct {
	cfg        *config.Config
	Router     *mux.Router
	httpServer *http.Server

	db     *gorm.DB
	ctx    context.Context
	cancel context.CancelFunc
}

func (a *App) Initialize(cfg *config.Config) {
	a.cfg = cfg

	// 1) Логи
	logs.Init(logs.Options{
		Level:  a.cfg.Logging.Level,
		Format: a.cfg.Logging.Format,
		File:   a.cfg.Logging.File,
	})

	// 2) БД (опционально)
	if drv := a.cfg.Database.Driver; drv != "" {
		d, err := db.Open(drv, a.cfg.Database.DSN)
		if err != nil {
			log.Fatalf("db open failed: %v", err)
		}
		a.db = d
		// базовые миграции устройств
		if err := a.db.AutoMigrate(&models.Device{}); err != nil {
			log.Fatalf("db migrate failed: %v", err)
		}
	}

	// миграции доменных сущностей (если БД подключена)
	// ---- DB migrations (only if DB is connected) ----
	if a.db != nil {
		// 1) One-off rename of reserved columns (MySQL/MariaDB safe)
		if err := db.MigrateReservedColumns(a.db); err != nil {
			logs.Logger.Warnf("reserved columns migration: %v", err)
		}

		// 2) AutoMigrate all domain models
		if err := a.db.AutoMigrate(
			// devices
			&models.Device{},

			// configsvc (templates/vars/groups)
			&models.Template{},
			&models.DeviceVariable{},
			&models.DeviceTemplateAssignment{},
			&models.Group{},
			&models.DeviceGroup{},
			&models.GroupVariable{},
			&models.GroupTemplateAssignment{},
			&models.DeviceTemplateBlock{},
			&models.DeviceStatusHistory{},

			// ipam (prefixes & IPs)
			&models.Prefix{},
			&models.GroupPrefix{},
			&models.DeviceIP{},
		); err != nil {
			logs.Logger.Errorf("automigrate: %v", err)
		}
	}

	// 3) Роутер + middleware
	a.Router = mux.NewRouter()
	a.Router.Use(middleware.RequestID)
	a.Router.Use(middleware.Recoverer)
	a.Router.Use(middleware.LoggerMW)

	a.RegisterWebUI("/ui/")

	// 4) Health маршруты (вот почему у вас был 404)
	if a.db != nil {
		health.RegisterRoutesWithDB(a.Router, a.db) // /healthz и /readyz
	} else {
		health.RegisterRoutes(a.Router) // только /healthz
	}

	// 5) Конфиг-сервис и IPAM HTTP + билдер
	var cfgRepoInst *configsvc.Repo
	var cfgBuilder *configsvc.Builder
	if a.db != nil {
		cfgRepoInst = configsvc.NewRepo(a.db)
		ipamRepo := ipam.NewRepo(a.db)

		// === ВАЖНО: регистрируем HTTP-ручки configsvc ===
		cfgHTTP := configsvc.NewHTTP(cfgRepoInst)
		cfgHTTP.RegisterRoutes(a.Router)

		grpHTTP := configsvc.NewGroupHTTP(cfgRepoInst)
		grpHTTP.RegisterRoutes(a.Router)

		ipamHTTP := ipam.NewHTTP(ipamRepo) // для префиксов и назначения группе
		ipamHTTP.RegisterRoutes(a.Router)
		ipamDevHTTP := ipam.NewDeviceHTTP(ipamRepo) // для IP устройства
		ipamDevHTTP.RegisterRoutes(a.Router)

		cfgBuilder = configsvc.NewBuilderWithIPAM(cfgRepoInst, ipamRepo)
	}
	// 6) Контроллер (agent-совместимость)
	if a.db != nil {
		ds := repo.NewDeviceStore(a.db)
		owctrl.RegisterRoutesWithStoreAndBuilder(a.Router, a.cfg.OpenWISP.SharedSecret, ds, cfgBuilder)
	} else {
		owctrl.RegisterRoutes(a.Router, a.cfg.OpenWISP.SharedSecret)
	}

	a.Router.Walk(func(rt *mux.Route, r *mux.Router, ancestors []*mux.Route) error {
		path, _ := rt.GetPathTemplate()
		methods, _ := rt.GetMethods()
		log.Printf("route: %-6v %s", methods, path)
		return nil
	})
}

func (a *App) Run() error {
	if a.Router == nil || a.cfg == nil {
		return ErrNotInitialized
	}
	bind := net.JoinHostPort(a.cfg.Server.Address, a.cfg.Server.HTTPPort)

	a.ctx, a.cancel = context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigs; a.cancel() }()

	a.httpServer = &http.Server{
		Addr:         bind,
		Handler:      a.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("HTTP listening on %s", bind)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-a.ctx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.httpServer.Shutdown(ctx)
	return nil
}

var ErrNotInitialized = &initError{"server not initialized (call Initialize(cfg) first)"}

type initError struct{ s string }

func (e *initError) Error() string { return e.s }
