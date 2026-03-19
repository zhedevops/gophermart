package middleware

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/zhedevops/gophermart/internal/logger"

	"net/http"
	"strings"
	"time"
)

func RequireContentType(rct string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, rct) {
				http.Error(w, fmt.Sprintf("unsupported content type, require: %s", rct), http.StatusUnsupportedMediaType)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func Logger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := logger.ResponseData{
			Status: 0,
			Size:   0,
		}
		lw := logger.LoggingResponseWriter{
			ResponseWriter: w,
			ResponseData:   &resp,
		}
		start := time.Now()
		h.ServeHTTP(&lw, r)
		uri := r.RequestURI
		method := r.Method
		duration := time.Since(start)
		status := resp.Status
		size := resp.Size
		log.Info().
			Timestamp().
			Str("uri", uri).
			Str("method", method).
			Str("duration", fmt.Sprint(duration)).
			Int("status", status).
			Int("size", size).
			Send()
	})
}
