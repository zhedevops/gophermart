package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/zhedevops/gophermart/internal/config"
	"github.com/zhedevops/gophermart/internal/mocks"
	"github.com/zhedevops/gophermart/internal/model"
)

func TestService_SetOrder(t *testing.T) {
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
	m.EXPECT().SetOrder(order).Return(model.ErrConflict)
	srv := NewService(m, cnf)

	t.Run("test ok", func(t *testing.T) {
		err := srv.SetOrder(user, orderNum)
		assert.Nil(t, err)
	})

	t.Run("test invalid order format", func(t *testing.T) {
		err := srv.SetOrder(user, "2ab")
		assert.NotNil(t, err)
		assert.Equal(t, "invalid order format", err.Error())
	})

	t.Run("test luhn wrong", func(t *testing.T) {
		err := srv.SetOrder(user, "123456")
		assert.NotNil(t, err)
		assert.Equal(t, "invalid order format", err.Error())
	})

	t.Run("test repo err conflict", func(t *testing.T) {
		err := srv.SetOrder(user, orderNum)
		assert.NotNil(t, err)
		assert.Equal(t, "data conflict", err.Error())
	})
}

func TestService_SetWithdraw(t *testing.T) {
	var user = model.User{
		ID: 1,
	}
	orderNum := "4305603"
	var req = model.RequestWithdraw{
		Order: orderNum,
		Sum:   decimal.NewFromFloat(111),
	}
	var order = &model.Order{
		Number:   req.Order,
		UserID:   user.ID,
		Withdraw: req.Sum,
	}
	var reqZero = model.RequestWithdraw{
		Order: orderNum,
		Sum:   decimal.NewFromFloat(0),
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockRepository(ctrl)
	cnf := config.GetConfig()
	m.EXPECT().SetWithdraw(order).Return(nil)
	m.EXPECT().SetWithdraw(order).Return(errors.New("unexpected error"))
	m.EXPECT().SetWithdraw(order).Return(model.ErrInsufficientFunds)
	m.EXPECT().SetWithdraw(order).Return(model.ErrWrongOrderNumber)
	srv := NewService(m, cnf)

	t.Run("test ok", func(t *testing.T) {
		err := srv.SetWithdraw(user, req)
		assert.Nil(t, err)
	})

	t.Run("test invalid sum", func(t *testing.T) {
		err := srv.SetWithdraw(user, reqZero)
		assert.NotNil(t, err)
		assert.Equal(t, "sum must be greater than 0", err.Error())
	})

	t.Run("test unexpected error", func(t *testing.T) {
		err := srv.SetWithdraw(user, req)
		assert.NotNil(t, err)
		assert.Equal(t, "unexpected error", err.Error())
	})

	t.Run("test insufficient funds", func(t *testing.T) {
		err := srv.SetWithdraw(user, req)
		assert.NotNil(t, err)
		assert.Equal(t, "insufficient funds", err.Error())
	})

	t.Run("test invalid order format", func(t *testing.T) {
		err := srv.SetWithdraw(user, req)
		assert.NotNil(t, err)
		assert.Equal(t, "invalid order format", err.Error())
	})
}

func TestService_GetNewUser(t *testing.T) {
	login := "d51eae65"
	password := "dlf82a5xunr"
	passHash := "$2a$10$is1n9tK40PTAR/BPETvAOu3MW9vFYABrUBX5b/o/T7Pj4sdOz4pYS"
	createdUser := model.User{
		ID:           1,
		Login:        login,
		PasswordHash: passHash,
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockRepository(ctrl)
	cnf := config.GetConfig()
	m.EXPECT().CreateUser(gomock.Any()).DoAndReturn(func(u model.User) (model.User, error) {
		assert.Equal(t, login, u.Login)
		return createdUser, nil
	})
	m.EXPECT().CreateUser(gomock.Any()).Return(model.User{}, errors.New("user cannot create"))
	srv := NewService(m, cnf)

	t.Run("test ok", func(t *testing.T) {
		user, err := srv.GetNewUser(login, password)
		assert.Nil(t, err)
		assert.Equal(t, createdUser.Login, user.Login)
		err = CheckPassword(user.PasswordHash, password)
		assert.Nil(t, err)
	})

	t.Run("test cannot create user", func(t *testing.T) {
		user, err := srv.GetNewUser(login, password)
		assert.NotNil(t, err)
		assert.Equal(t, "user cannot create", err.Error())
		assert.Equal(t, model.User{}, user)
	})

	t.Run("test invalid order format", func(t *testing.T) {
		srv.hashFunc = func(_ string) (string, error) {
			return "", errors.New("hash failed")
		}
		user, err := srv.GetNewUser(login, password)
		assert.NotNil(t, err)
		assert.Equal(t, "hash failed", err.Error())
		assert.Equal(t, model.User{}, user)
	})
}

func TestService_AuthentificateUser(t *testing.T) {
	login := "d51eae65"
	password := "dlf82a5xunr"
	passHash := "$2a$10$is1n9tK40PTAR/BPETvAOu3MW9vFYABrUBX5b/o/T7Pj4sdOz4pYS"
	wrongLogin := "St5AOFaghE"
	wrongHash := "$2a$10$bJdZVGtHILl2x/8LEm7zTuY0RjCKzjllAP0GLGdijQQ.kaey6AHoe"
	authdUser := model.User{
		ID:           1,
		Login:        login,
		PasswordHash: passHash,
	}
	wrongUser := model.User{
		ID:           2,
		Login:        wrongLogin,
		PasswordHash: wrongHash,
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockRepository(ctrl)
	m.EXPECT().FindUser(gomock.Any()).DoAndReturn(func(u model.User) (model.User, error) {
		assert.Equal(t, login, u.Login)
		return authdUser, nil
	})
	m.EXPECT().FindUser(gomock.Any()).Return(model.User{}, model.ErrUserNotFound)
	m.EXPECT().FindUser(gomock.Any()).Return(model.User{}, errors.New("unexpected error"))
	m.EXPECT().FindUser(gomock.Any()).DoAndReturn(func(u model.User) (model.User, error) {
		assert.Equal(t, login, u.Login)
		return wrongUser, nil
	})

	cnf := config.GetConfig()
	srv := NewService(m, cnf)

	t.Run("test ok", func(t *testing.T) {
		user, err := srv.AuthentificateUser(login, password)
		assert.Nil(t, err)
		assert.Equal(t, authdUser.ID, user.ID)
	})

	t.Run("test invalid username/password", func(t *testing.T) {
		_, err := srv.AuthentificateUser(login, password)
		assert.NotNil(t, err)
		assert.Equal(t, "invalid username/password", err.Error())
	})

	t.Run("test unexpected error", func(t *testing.T) {
		_, err := srv.AuthentificateUser(login, password)
		assert.NotNil(t, err)
		assert.Equal(t, "unexpected error", err.Error())
	})

	t.Run("test invalid credentials", func(t *testing.T) {
		_, err := srv.AuthentificateUser(login, password)
		assert.NotNil(t, err)
		assert.Equal(t, "invalid credentials", err.Error())
	})
}

func TestService_CheckAuthCookie(t *testing.T) {
	cnf := config.GetConfig()
	srv := NewService(nil, cnf)
	var u = model.User{
		ID: 1,
	}
	cookieStr := srv.GetAuthCookie(u)
	cookie := &http.Cookie{
		Name:     "Test",
		Value:    cookieStr,
		Path:     "/",
		HttpOnly: true,
	}

	t.Run("test ok", func(t *testing.T) {
		user, err := srv.CheckAuthCookie(cookie)
		assert.Nil(t, err)
		assert.Equal(t, u.ID, user.ID)
	})

	t.Run("test bad cookie value", func(t *testing.T) {
		var acBad = "eyJ1aWQiOjEsImV4cCI6MTc3Mzk0NTc3M30=KCsJqyE3XZlhfX6JwnPyK6JdwnqZZ2Zdbe/uADE2Q4s="
		cookieBad := &http.Cookie{
			Name:     "Test",
			Value:    acBad,
			Path:     "/",
			HttpOnly: true,
		}
		_, err := srv.CheckAuthCookie(cookieBad)
		assert.NotNil(t, err)
		assert.Equal(t, "bad cookie value", err.Error())
	})

	t.Run("test decode cookie value failed", func(t *testing.T) {
		var acBad = "eyJ1aWQiOjEsImV4cCI6MTc3Mzk0NTc3M30.KCsJqyE3XZlhfX6JwnPyK6JdwnqZZ2Zdbe/uADE2Q4s="
		cookieBad := &http.Cookie{
			Name:     "Test",
			Value:    acBad,
			Path:     "/",
			HttpOnly: true,
		}
		_, err := srv.CheckAuthCookie(cookieBad)
		assert.NotNil(t, err)
		assert.Equal(t, "decode cookie value failed", err.Error())
	})

	t.Run("test decode cookie value signature failed", func(t *testing.T) {
		var acBad = "eyJ1aWQiOjEsImV4cCI6MTc3Mzk0NTc3M30=.KCsJqyE3XZlhfX6JwnPyK6JdwnqZZ2Zdbe/uADE2Q4s"
		cookieBad := &http.Cookie{
			Name:     "Test",
			Value:    acBad,
			Path:     "/",
			HttpOnly: true,
		}
		_, err := srv.CheckAuthCookie(cookieBad)
		assert.NotNil(t, err)
		assert.Equal(t, "decode cookie value signature failed", err.Error())
	})

	t.Run("test signature verification failed", func(t *testing.T) {
		var acBad = "KCsJqyE3XZlhfX6JwnPyK6JdwnqZZ2Zdbe/uADE2Q4s=.eyJ1aWQiOjEsImV4cCI6MTc3Mzk0NTc3M30="
		cookieBad := &http.Cookie{
			Name:     "Test",
			Value:    acBad,
			Path:     "/",
			HttpOnly: true,
		}
		_, err := srv.CheckAuthCookie(cookieBad)
		assert.NotNil(t, err)
		assert.Equal(t, "signature verification failed", err.Error())
	})

	t.Run("test unmarshal user data failed", func(t *testing.T) {
		payload := `{"uid":1,"exp"}`
		payloadB64 := base64.StdEncoding.EncodeToString([]byte(payload))
		h := hmac.New(sha256.New, srv.cnf.Secret)
		h.Write([]byte(payload))
		sign := base64.StdEncoding.EncodeToString(h.Sum(nil))
		var acBad = payloadB64 + "." + sign
		cookieBad := &http.Cookie{
			Name:     "Test",
			Value:    acBad,
			Path:     "/",
			HttpOnly: true,
		}
		_, err := srv.CheckAuthCookie(cookieBad)
		assert.NotNil(t, err)
		assert.Equal(t, "unmarshal user data failed", err.Error())
	})

	t.Run("test user expired", func(t *testing.T) {
		payload := fmt.Sprintf(`{"uid":1,"exp":%d}`, time.Now().Add(-time.Hour).Unix())
		payloadB64 := base64.StdEncoding.EncodeToString([]byte(payload))
		h := hmac.New(sha256.New, srv.cnf.Secret)
		h.Write([]byte(payload))
		sign := base64.StdEncoding.EncodeToString(h.Sum(nil))
		var acBad = payloadB64 + "." + sign
		cookieBad := &http.Cookie{
			Name:     "Test",
			Value:    acBad,
			Path:     "/",
			HttpOnly: true,
		}
		_, err := srv.CheckAuthCookie(cookieBad)
		assert.NotNil(t, err)
		assert.Equal(t, "user expired", err.Error())
	})
}

func TestService_GetAuthCookie(t *testing.T) {
	cnf := config.GetConfig()
	srv := NewService(nil, cnf)
	user := model.User{
		ID: 1,
	}
	cookieStr := srv.GetAuthCookie(user)
	assert.NotEmpty(t, cookieStr)
	parts := strings.Split(cookieStr, ".")
	assert.Len(t, parts, 2)
	cookie := &http.Cookie{
		Value: cookieStr,
	}
	resUser, err := srv.CheckAuthCookie(cookie)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, resUser.ID)
}

func TestService_getAccrual(t *testing.T) {
	var accrual = 999.02
	acc := decimal.NewFromFloat(accrual)
	ts := httptest.NewServer(nil)
	defer ts.Close()
	setHandler := func(h http.HandlerFunc) {
		ts.Config.Handler = h
	}
	cnf := config.GetConfig()
	srv := NewService(nil, cnf)
	srv.cnf.AccrualAddr.ServerAddress = ts.URL

	t.Run("test ok", func(t *testing.T) {
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_ = json.NewEncoder(w).Encode(model.ResponseAccrualService{
				Order:   "123",
				Status:  "PROCESSED",
				Accrual: &acc,
			})
		})

		resp, retry, err := srv.getAccrual("123")
		assert.NoError(t, err)
		assert.Equal(t, time.Duration(0), retry)
		assert.Equal(t, "123", resp.Order)
	})

	t.Run("test too many requests", func(t *testing.T) {
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(http.StatusTooManyRequests)
		})

		_, retry, err := srv.getAccrual("123")
		assert.Error(t, err)
		assert.ErrorIs(t, err, model.ErrToManyRequests)
		assert.Equal(t, 10*time.Second, retry)
	})

	t.Run("test no contents", func(t *testing.T) {
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		_, _, err := srv.getAccrual("123")
		assert.Error(t, err)
		assert.ErrorIs(t, err, model.ErrOrderNotRegistered)
	})

	t.Run("test invalid json", func(t *testing.T) {
		setHandler(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("invalid json"))
		})

		_, _, err := srv.getAccrual("123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid character")
	})
}
