package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zhedevops/gophermart/internal/config"
	"github.com/zhedevops/gophermart/internal/database"
	"github.com/zhedevops/gophermart/internal/middleware"
	"github.com/zhedevops/gophermart/internal/mocks"
	"github.com/zhedevops/gophermart/internal/model"
	"github.com/zhedevops/gophermart/internal/service"
	"github.com/zhedevops/gophermart/internal/storage"
)

type mockHandler struct{}

func (h *mockHandler) OrdersHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`ok`))
}

func (h *mockHandler) BalanceHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`balance`))
}

func TestRouter(t *testing.T) {
	h := &mockHandler{}
	r := chi.NewRouter()
	r.Get("/api/user/balance", h.BalanceHandler)
	r.With(middleware.RequireContentType("text/plain")).Post("/api/user/orders", h.OrdersHandler)

	t.Run("GET balance returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		resp := w.Result()
		defer func() {
			_ = resp.Body.Close()
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("POST orders with correct Content-Type", func(t *testing.T) {
		body := strings.NewReader("123456")
		req := httptest.NewRequest(http.MethodPost, "/api/user/orders", body)
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		resp := w.Result()
		defer func() {
			_ = resp.Body.Close()
		}()

		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	})

	t.Run("wrong Content-Type", func(t *testing.T) {
		body := strings.NewReader("123456")
		req := httptest.NewRequest(http.MethodPost, "/api/user/orders", body)
		req.Header.Set("Content-Type", "application/json") // неправильно
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		resp := w.Result()
		defer func() {
			_ = resp.Body.Close()
		}()

		assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	})

	t.Run("unknown route returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		resp := w.Result()
		defer func() {
			_ = resp.Body.Close()
		}()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestHandler_PingHandler(t *testing.T) {
	a := assert.New(t)
	_ = godotenv.Load("../../.env")
	dsn, _ := os.LookupEnv("DATABASE_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@postgres:5432/praktikum"
	}
	cnf := config.GetConfig()
	// Открываем пул
	pool, err := database.ConnectDB(dsn)
	a.Nil(err)
	a.NotNil(pool)
	a.IsType(&pgxpool.Pool{}, pool)
	st := storage.NewDBStorage(pool)
	srv := service.NewService(st, cnf)
	h := &Handler{service: srv, Cfg: cnf}
	r := chi.NewRouter()
	r.HandleFunc("/ping", h.PingHandler)
	t.Run("Pool opened. Ping ok", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)

		res := w.Result()
		assert.Equal(t, http.StatusOK, res.StatusCode)
		defer func() {
			_ = res.Body.Close()
		}()
	})
	t.Run("Pool closed. Ping failure", func(t *testing.T) {
		// Удаляем пул
		database.CloseDB(pool)
		ctx := context.Background()
		err = pool.Ping(ctx)
		a.NotNil(err)

		request := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)

		res := w.Result()
		assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
		defer func() {
			_ = res.Body.Close()
		}()
	})
}

func TestHandler_OrdersHandler(t *testing.T) {
	var user = model.User{
		ID: 1,
	}
	orderNum := "4305603"
	order := &model.Order{
		Number: orderNum,
		Status: model.StatusNew,
		UserID: user.ID,
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockRepository(ctrl)
	cnf := config.GetConfig()
	m.EXPECT().SetOrder(order).Return(nil)
	m.EXPECT().SetOrder(order).Return(model.ErrOrderAlreadyExists)
	m.EXPECT().SetOrder(order).Return(model.ErrConflict)
	srv := service.NewService(m, cnf)
	h := &Handler{service: srv, Cfg: cnf}
	ac := h.service.GetAuthCookie(user)
	cookie := &http.Cookie{
		Name:     "Authorization",
		Value:    ac,
		Path:     "/",
		HttpOnly: true,
	}
	var target = "/api/user/orders"
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.With(middleware.RequireContentType("text/plain")).HandleFunc(target, h.OrdersHandler)

	type want struct {
		code        int
		response    string
		err         string
		contentType string
	}
	type args struct {
		method      string
		target      string
		body        string
		contentType string
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "MethodPost result success",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        "4305603",
				contentType: "text/plain",
			},
			want: want{
				code:        http.StatusAccepted,
				response:    "",
				err:         "",
				contentType: "text/plain",
			},
		},
		{
			name: "order already exists",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        "4305603",
				contentType: "text/plain",
			},
			want: want{
				code:        http.StatusOK,
				response:    "",
				err:         "",
				contentType: "text/plain",
			},
		},
		{
			name: "order data conflict",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        "4305603",
				contentType: "text/plain",
			},
			want: want{
				code:        http.StatusConflict,
				response:    "",
				err:         "",
				contentType: "text/plain",
			},
		},
		{
			name: "wrong order",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        "123",
				contentType: "text/plain",
			},
			want: want{
				code:        http.StatusUnprocessableEntity,
				response:    "",
				err:         "invalid order format",
				contentType: "text/plain",
			},
		},
		{
			name: "bad request",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        "123",
				contentType: "application/json",
			},
			want: want{
				code:        http.StatusUnsupportedMediaType,
				response:    "",
				err:         "unsupported content type",
				contentType: "text/plain",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.args.method, tt.args.target, strings.NewReader(tt.args.body))
			request.Header.Add("Content-Type", tt.args.contentType)
			request.AddCookie(cookie)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, request)

			res := w.Result()
			assert.Equal(t, tt.want.code, res.StatusCode)

			defer func() {
				_ = res.Body.Close()
			}()
			resBody, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			if tt.want.err != "" {
				assert.Contains(t, string(resBody), tt.want.err)
			}
		})
	}
}

func TestHandler_handleCookie(t *testing.T) {
	var user = model.User{
		ID: 1,
	}
	orderNum := "4305603"
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockRepository(ctrl)
	cnf := config.GetConfig()
	srv := service.NewService(m, cnf)
	h := &Handler{service: srv, Cfg: cnf}
	ac := h.service.GetAuthCookie(user)
	cookie := &http.Cookie{
		Name:     "Test",
		Value:    ac,
		Path:     "/",
		HttpOnly: true,
	}
	badCookie := &http.Cookie{
		Name:  "Authorization",
		Value: "broken-cookie",
		Path:  "/",
	}
	var target = "/api/user/orders"
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.With(middleware.RequireContentType("text/plain")).HandleFunc(target, h.OrdersHandler)

	t.Run("test user not authenticated", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(orderNum))
		request.Header.Add("Content-Type", "text/plain")
		request.AddCookie(cookie)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)

		res := w.Result()
		assert.Equal(t, http.StatusUnauthorized, res.StatusCode)

		defer func() {
			_ = res.Body.Close()
		}()
		resBody, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		assert.Contains(t, string(resBody), "user not authenticated")
	})

	t.Run("test handleCookie_failure", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(orderNum))
		request.Header.Add("Content-Type", "text/plain")
		request.AddCookie(badCookie)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)

		res := w.Result()
		assert.Equal(t, http.StatusInternalServerError, res.StatusCode)

		defer func() {
			_ = res.Body.Close()
		}()
		resBody, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		assert.Contains(t, string(resBody), "handleCookie_failure")
	})
}

func TestHandler_RegisterHandler(t *testing.T) {
	login := "d51eae65"
	password := "dlf82a5xunr"
	passHash := "$2a$10$is1n9tK40PTAR/BPETvAOu3MW9vFYABrUBX5b/o/T7Pj4sdOz4pYS"
	createdUser := model.User{
		ID:           1,
		Login:        login,
		PasswordHash: passHash,
	}
	body := fmt.Sprintf(`{"login":"%s","password":"%s"}`, login, password)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockRepository(ctrl)
	m.EXPECT().CreateUser(gomock.Any()).DoAndReturn(func(u model.User) (model.User, error) {
		assert.Equal(t, login, u.Login)
		return createdUser, nil
	})
	m.EXPECT().CreateUser(gomock.Any()).Return(model.User{}, errors.New("user cannot create"))
	cnf := config.GetConfig()
	srv := service.NewService(m, cnf)
	h := &Handler{service: srv, Cfg: cnf}
	var target = "/api/user/register"
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.With(middleware.RequireContentType("application/json")).HandleFunc(target, h.RegisterHandler)

	type want struct {
		code        int
		response    string
		err         string
		contentType string
	}
	type args struct {
		method      string
		target      string
		body        string
		contentType string
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "MethodPost result success",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        body,
				contentType: "application/json",
			},
			want: want{
				code:        http.StatusOK,
				response:    "",
				err:         "",
				contentType: "",
			},
		},
		{
			name: "unsupported content type",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        body,
				contentType: "text/plain",
			},
			want: want{
				code:        http.StatusUnsupportedMediaType,
				response:    "",
				err:         "unsupported content type",
				contentType: "text/plain",
			},
		},
		{
			name: "invalid json",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        `{"login":"d51eae65"`,
				contentType: "application/json",
			},
			want: want{
				code:        http.StatusBadRequest,
				response:    "",
				err:         "cannot decode request JSON body",
				contentType: "text/plain",
			},
		},
		{
			name: "empty login",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        `{"login":"","password":"dlf82a5xunr"}`,
				contentType: "application/json",
			},
			want: want{
				code:        http.StatusBadRequest,
				response:    "",
				err:         "login and password required",
				contentType: "application/json",
			},
		},
		{
			name: "cannot create user",
			args: args{
				method:      http.MethodPost,
				target:      target,
				body:        body,
				contentType: "application/json",
			},
			want: want{
				code:        http.StatusInternalServerError,
				response:    "",
				err:         "cannot create user",
				contentType: "application/json",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.args.method, tt.args.target, strings.NewReader(tt.args.body))
			request.Header.Add("Content-Type", tt.args.contentType)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, request)

			res := w.Result()
			assert.Equal(t, tt.want.code, res.StatusCode)

			defer func() {
				_ = res.Body.Close()
			}()
			resBody, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			cookies := res.Cookies()
			var authCookie *http.Cookie
			for _, c := range cookies {
				if c.Name == "Authorization" {
					authCookie = c
					break
				}
			}
			if tt.want.err != "" {
				assert.Contains(t, string(resBody), tt.want.err)
				assert.Nil(t, authCookie, "Authorization cookie should not be set")
			} else {
				assert.NotNil(t, authCookie, "Authorization cookie should be set")
				assert.NotEmpty(t, authCookie.Value, "Authorization cookie should not be empty")
			}
		})
	}
}
