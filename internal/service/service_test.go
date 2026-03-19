package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
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
	var u = model.User{
		ID: 1,
	}
	var ac = "eyJ1aWQiOjEsImV4cCI6MTc3Mzk0NTc3M30=.KCsJqyE3XZlhfX6JwnPyK6JdwnqZZ2Zdbe/uADE2Q4s="
	cookie := &http.Cookie{
		Name:     "Test",
		Value:    ac,
		Path:     "/",
		HttpOnly: true,
	}
	cnf := config.GetConfig()
	srv := NewService(nil, cnf)

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
