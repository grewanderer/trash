package middleware

import (
	"net/http"
	"runtime/debug"
	"wisp/internal/logs"
	"wisp/internal/models"
)

// Recoverer перехватывает панику в обработчике, пишет лог со стеком
// и возвращает 500 в формате application/problem+json.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqid := GetRequestID(r)
				logs.Logger.Errorf("panic: %v reqid=%s uri=%s method=%s\nstack:\n%s",
					rec, reqid, r.RequestURI, r.Method, string(debug.Stack()))
				// Отдаём единый JSON-ответ об ошибке
				models.WriteProblem(w, http.StatusInternalServerError,
					"Internal Server Error",
					"unexpected server error (see logs by reqid)", map[string]any{
						"reqid": reqid,
					})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
