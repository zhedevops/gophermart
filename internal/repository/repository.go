package repository

import (
	"github.com/zhedevops/gophermart/internal/model"
)

type Repository interface {
	CreateUser(user model.User) (model.User, error)
	FindUser(user model.User) (model.User, error)
	SetOrder(order *model.Order) error
	UpdateOrder(order *model.Order) error
	GetOrdersByUser(userID uint32, operation model.OrderOperation) ([]*model.Order, error)
	GetOrdersByStatus(statuses []model.OrderStatus) ([]*model.Order, error)
	GetWithdrawalsByUser(userID uint32, operation model.OrderOperation) ([]*model.Order, error)
	GetBalance(userID uint32) (*model.Account, error)
	SetWithdraw(order *model.Order) error
}
