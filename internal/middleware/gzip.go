package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

var allowedContentTypes = map[string]struct{}{
	"application/json": {},
	"text/html":        {},
}

type gzipReader struct {
	r      io.ReadCloser
	reader *gzip.Reader
}

type gzipWriter struct {
	w      http.ResponseWriter
	writer *gzip.Writer
}

func (gzw *gzipWriter) Write(b []byte) (int, error) {
	return gzw.writer.Write(b)
}

func (gzw *gzipWriter) Header() http.Header {
	return gzw.w.Header()
}

func (gzw *gzipWriter) WriteHeader(statusCode int) {
	if statusCode < 300 {
		gzw.w.Header().Set("Content-Encoding", "gzip")
	}
	gzw.w.WriteHeader(statusCode)
}

func (gzw *gzipWriter) Close() error {
	return gzw.writer.Close()
}

func (gzr *gzipReader) Read(p []byte) (n int, err error) {
	return gzr.reader.Read(p)
}

func (gzr *gzipReader) Close() error {
	if err := gzr.reader.Close(); err != nil {
		return err
	}
	return gzr.r.Close()
}

func isAllowedContentType(r *http.Request) bool {
	var ct = r.Header.Get("Content-Type")
	if _, ok := allowedContentTypes[ct]; ok {
		return true
	}
	return false
}

func GzipHandle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ow := w

		if r.Header.Get("Content-Encoding") == "gzip" {
			origBody := r.Body
			gzr, err := gzip.NewReader(origBody)
			if err != nil {
				http.Error(ow, err.Error(), http.StatusBadRequest)
				return
			}
			r.Body = &gzipReader{r: origBody, reader: gzr}
			defer func() {
				_ = gzr.Close()
			}()
		}

		if !isAllowedContentType(r) {
			next.ServeHTTP(w, r)
			return
		}

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			gzw := gzip.NewWriter(w)
			ow = &gzipWriter{w: w, writer: gzw}
			defer func() {
				_ = gzw.Close()
			}()
		}

		next.ServeHTTP(ow, r)
	})
}
