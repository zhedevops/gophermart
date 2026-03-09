package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zhedevops/gophermart/internal/model"
	"github.com/zhedevops/gophermart/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var secretkey = []byte("supersecretkey")

type Service struct {
	repo repository.Repository
}

func NewService(r repository.Repository) *Service {
	return &Service{repo: r}
}

func (srv *Service) SetOrder(user model.User, orderNumber string) error {
	if !regexp.MustCompile(`^\d+$`).MatchString(orderNumber) {
		return model.ErrWrongOrderNumber
	}
	if !ValidLuhn(orderNumber) {
		return model.ErrWrongOrderNumber
	}
	var order = &model.Order{
		Number: orderNumber,
		Status: model.StatusNew,
		UserID: user.ID,
	}
	err := srv.repo.SetOrder(order)
	if err != nil {
		return err
	}
	return nil
}

func (srv *Service) SetWithdraw(user model.User, req model.RequestWithdraw) error {
	if !req.Sum.GreaterThan(decimal.Zero) {
		return errors.New("sum must be greater than 0")
	}
	if !regexp.MustCompile(`^\d+$`).MatchString(req.Order) {
		return model.ErrWrongOrderNumber
	}
	if !ValidLuhn(req.Order) {
		return model.ErrWrongOrderNumber
	}
	var order = &model.Order{
		Number:   req.Order,
		UserID:   user.ID,
		Withdraw: req.Sum,
	}
	err := srv.repo.SetWithdraw(order)
	if err != nil {
		return err
	}
	return nil
}

func (srv *Service) Ping(ctx context.Context) error {
	return srv.repo.Ping(ctx)
}

func (srv *Service) GetNewUser(login string, password string) (model.User, error) {
	user := model.User{}
	passHash, err := HashPassword(password)
	if err != nil {
		return user, err
	}
	user.PasswordHash = passHash
	user.Login = login
	return srv.repo.CreateUser(user)
}

func (srv *Service) AuthentificateUser(login string, password string) (model.User, error) {
	user := model.User{}
	passHash, err := HashPassword(password)
	if err != nil {
		return user, err
	}
	err = CheckPassword(passHash, login)
	if err != nil {
		return user, err
	}
	user.PasswordHash = passHash
	user.Login = login
	return srv.repo.FindUser(user)
}

func (srv *Service) CheckAuthCookie(cookieAuth *http.Cookie) (model.User, error) {
	user := model.User{}
	ujwt := model.UserJWT{}
	values := strings.Split(cookieAuth.Value, ".")
	if len(values) != 2 {
		return user, errors.New("bad cookie value")
	}
	jwtData, err := base64.StdEncoding.DecodeString(values[0])
	if err != nil {
		return user, errors.New("decode cookie value failed")
	}
	signature, err := base64.StdEncoding.DecodeString(values[1])
	if err != nil {
		return user, errors.New("decode cookie value signature failed")
	}
	h := hmac.New(sha256.New, secretkey)
	h.Write(jwtData)
	sign := h.Sum(nil)
	if !hmac.Equal(sign, signature) {
		return user, errors.New("signature verification failed")
	}
	err = json.Unmarshal(jwtData, &ujwt)
	if err != nil {
		return user, errors.New("unmarshal user data failed")
	}
	if ujwt.Exp < time.Now().Unix() {
		return user, errors.New("user expired")
	}
	user.ID = ujwt.UID
	return user, nil
}

func (srv *Service) GetAuthCookie(user model.User) string {
	userJWT := model.UserJWT{
		UID: user.ID,
		Exp: time.Now().Add(time.Hour).Unix(),
	}
	userData, _ := json.Marshal(userJWT)
	h := hmac.New(sha256.New, secretkey)
	h.Write(userData)
	sign := h.Sum(nil)
	return base64.StdEncoding.EncodeToString(userData) + "." + base64.StdEncoding.EncodeToString(sign)
}

func (srv *Service) GetUserOrders(userID uint32) ([]*model.Order, error) {
	return srv.repo.GetOrdersByUser(userID, model.OperationAccrual)
}

func (srv *Service) GetBalance(userID uint32) (*model.Account, error) {
	return srv.repo.GetBalance(userID)
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPassword(hash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func ValidLuhn(number string) bool {
	sum := 0
	alt := false

	for i := len(number) - 1; i >= 0; i-- {
		n := int(number[i] - '0')

		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}

		sum += n
		alt = !alt
	}

	return sum%10 == 0
}
