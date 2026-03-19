package model

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

const (
	StatusNew OrderStatus = iota
	StatusProcessing
	StatusInvalid
	StatusProcessed
)

const (
	StatusAccrualRegistered = "REGISTERED"
	StatusAccrualProcessing = "PROCESSING"
	StatusAccrualInvalid    = "INVALID"
	StatusAccrualProcessed  = "PROCESSED"
)

const (
	OperationAccrual OrderOperation = iota
	OperationWithdrawal
)

var StatusMap = map[OrderStatus]string{
	StatusNew:        "NEW",
	StatusProcessing: "PROCESSING",
	StatusInvalid:    "INVALID",
	StatusProcessed:  "PROCESSED",
}

type OrderStatus int16
type OrderOperation int16

type RequestUser struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type RequestWithdraw struct {
	Order string          `json:"order"`
	Sum   decimal.Decimal `json:"sum"`
}

type Account struct {
	ID        uint32  `json:"id"`
	Deposit   float64 `json:"deposit"`
	Withdrawn float64 `json:"withdrawn"`
	UserID    uint32  `json:"user_id"`
}

type ResponseBalance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type ResponseUserOrders struct {
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    *float64  `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type ResponseUserWithdrawals struct {
	Order       string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

type ResponseAccrualService struct {
	Order   string           `json:"order"`
	Status  string           `json:"status"`
	Accrual *decimal.Decimal `json:"accrual,omitempty"`
}
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type User struct {
	ID           uint32    `json:"id"`
	Login        string    `json:"login"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

type Order struct {
	ID         uint32           `json:"id"`
	Number     string           `json:"number"`
	Status     OrderStatus      `json:"status"`
	UserID     uint32           `json:"user_id"`
	UploadedAt time.Time        `json:"uploaded_at"`
	Accrual    *decimal.Decimal `json:"accrual,omitempty"`
	Withdraw   decimal.Decimal  `json:"withdraw"`
}

type OrderTask struct {
	OrderNumber string
	NextCheck   time.Time
}

type UserJWT struct {
	UID uint32 `json:"uid"`
	Exp int64  `json:"exp"`
}

var ErrConflict = errors.New("data conflict")
var ErrUserNotAuthenticated = errors.New("user not authenticated")
var ErrUserNotFound = errors.New("invalid username/password")
var ErrOrderAlreadyExists = errors.New("order already exists")
var ErrWrongOrderNumber = errors.New("invalid order format")
var ErrInsufficientFunds = errors.New("insufficient funds")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrToManyRequests = errors.New("too many requests")
var ErrOrderNotRegistered = errors.New("order not registered")
