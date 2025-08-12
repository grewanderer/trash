package models

import (
	"encoding/json"
	"net/http"
)

// Problem представляет ответ об ошибке в стиле RFC 7807.
type Problem struct {
	Type     string      `json:"type,omitempty"`   // URL с описанием типа проблемы (можно оставить пустым)
	Title    string      `json:"title"`            // краткое название
	Status   int         `json:"status"`           // HTTP код
	Detail   string      `json:"detail,omitempty"` // подробности
	Instance string      `json:"instance,omitempty"`
	Extra    interface{} `json:"extra,omitempty"` // произвольные поля (map/struct)
}

func WriteProblem(w http.ResponseWriter, status int, title, detail string, extra any) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{
		Title:  title,
		Status: status,
		Detail: detail,
		Extra:  extra,
	})
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
