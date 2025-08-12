package health

import (
	"net/http"

	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

// RegisterRoutes — базовый liveness.
func RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/healthz", liveness).Methods(http.MethodGet)
}

// RegisterRoutesWithDB — liveness + readiness (проверка БД).
func RegisterRoutesWithDB(r *mux.Router, db *gorm.DB) {
	RegisterRoutes(r)
	r.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if db == nil {
			http.Error(w, "db not configured", http.StatusServiceUnavailable)
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			http.Error(w, "db handle error", http.StatusServiceUnavailable)
			return
		}
		if err := sqlDB.Ping(); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}).Methods(http.MethodGet)
}

func liveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}
