package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGzipHandle(t *testing.T) {
	resp := []byte("{GzipHandle test}")
	fn := func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
		if err != nil {
			http.Error(w, "cannot write response", http.StatusInternalServerError)
			return
		}
	}
	handler := GzipHandle(http.HandlerFunc(fn))

	tests := []struct {
		name            string
		contentEncoding string
		acceptEncoding  string
		contentType     string
		responseStatus  int
	}{
		{
			name:            "GzipHandle application/json gzip success",
			contentEncoding: "gzip",
			acceptEncoding:  "gzip",
			contentType:     "application/json",
			responseStatus:  http.StatusOK,
		},
		{
			name:            "GzipHandle text/html gzip success",
			contentEncoding: "gzip",
			acceptEncoding:  "gzip",
			contentType:     "text/html",
			responseStatus:  http.StatusOK,
		},
		{
			name:            "GzipHandle application/json acceptEncoding brotli",
			contentEncoding: "gzip",
			acceptEncoding:  "br",
			contentType:     "application/json",
			responseStatus:  http.StatusOK,
		},
		{
			name:            "GzipHandle text/html contentEncoding brotli",
			contentEncoding: "br",
			acceptEncoding:  "gzip",
			contentType:     "text/html",
			responseStatus:  http.StatusOK,
		},
		{
			name:            "GzipHandle abra/kadabra gzip",
			contentEncoding: "gzip",
			acceptEncoding:  "gzip",
			contentType:     "abra/kadabra",
			responseStatus:  http.StatusOK,
		},
		{
			name:            "GzipHandle abra/kadabra brotli",
			contentEncoding: "br",
			acceptEncoding:  "br",
			contentType:     "abra/kadabra",
			responseStatus:  http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if tt.contentEncoding == "gzip" {
				gzw := gzip.NewWriter(&buf)
				_, err := gzw.Write(resp)
				require.NoError(t, err)
				_ = gzw.Close()
			} else {
				_, err := buf.Write(resp)
				require.NoError(t, err)
			}

			request := httptest.NewRequest(http.MethodPost, "/ping", &buf)
			request.Header.Add("Content-Type", tt.contentType)
			request.Header.Add("Content-Encoding", tt.contentEncoding)
			request.Header.Add("Accept-Encoding", tt.acceptEncoding)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, request)
			res := w.Result()
			reader := res.Body
			defer func() {
				_ = res.Body.Close()
			}()

			assert.Equal(t, tt.responseStatus, res.StatusCode)

			if isAllowedContentType(request) && tt.acceptEncoding == "gzip" {
				assert.Equal(t, tt.acceptEncoding, res.Header.Get("Content-Encoding"))
				gzr, err := gzip.NewReader(res.Body)
				require.NoError(t, err)
				reader = gzr
			}

			resBody, err := io.ReadAll(reader)
			assert.NoError(t, err)
			assert.Equal(t, resp, resBody)
		})
	}
}
