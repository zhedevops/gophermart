package service

import (
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
