package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)

	fn := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "pong")
	}
	handler := Logger(http.HandlerFunc(fn))
	http.Handle(`/ping`, handler)
	request := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	res := w.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)
	defer func() {
		_ = res.Body.Close()
	}()
	resBody, err := io.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.Equal(t, "pong", string(resBody))

	line := buf.String()
	var logEntry map[string]interface{}
	err = json.Unmarshal([]byte(line), &logEntry)
	assert.NoError(t, err)
	assert.Equal(t, "/ping", logEntry["uri"])
	assert.Equal(t, float64(4), logEntry["size"])
	assert.Equal(t, http.MethodGet, logEntry["method"])
	assert.Equal(t, float64(http.StatusOK), logEntry["status"])
}
