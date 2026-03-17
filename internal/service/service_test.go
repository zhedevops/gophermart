package service

import (
	"errors"
	"testing"

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
