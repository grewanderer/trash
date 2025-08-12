package middleware

import (
	"net/http"
	"time"

	"wisp/internal/logs"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func LoggerMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r)
		d := time.Since(start)
		logs.Logger.Infof("reqid=%s method=%s uri=%s status=%d bytes=%d dur=%s ip=%s ua=%q",
			GetRequestID(r), r.Method, r.RequestURI, sw.status, sw.bytes, d, r.RemoteAddr, r.UserAgent())
	})
}
