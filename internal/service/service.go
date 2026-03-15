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
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zhedevops/gophermart/internal/config"
	"github.com/zhedevops/gophermart/internal/model"
	"github.com/zhedevops/gophermart/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo   repository.Repository
	cnf    *config.Config
	tasks  chan model.OrderTask
	client *http.Client
}

var orderNumberRegex = regexp.MustCompile(`^\d+$`)

func NewService(r repository.Repository, cnf *config.Config) *Service {
	tasks := make(chan model.OrderTask, 15)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	srv := &Service{
		repo:   r,
		cnf:    cnf,
		tasks:  tasks,
		client: client,
	}
	for i := 0; i < 3; i++ {
		go srv.worker(tasks)
	}
	return srv
}

func (srv *Service) SetOrder(user model.User, orderNumber string) error {
	if !orderNumberRegex.MatchString(orderNumber) {
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
	if !orderNumberRegex.MatchString(req.Order) {
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
	u := model.User{
		Login: login,
	}
	user, err := srv.repo.FindUser(u)
	if err != nil {
		return user, err
	}
	err = CheckPassword(user.PasswordHash, password)
	if err != nil {
		return user, model.ErrInvalidCredentials
	}
	return user, nil
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
	h := hmac.New(sha256.New, srv.cnf.Secret)
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
	h := hmac.New(sha256.New, srv.cnf.Secret)
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

func (srv *Service) GetUserWithdrawals(userID uint32) ([]*model.Order, error) {
	return srv.repo.GetWithdrawalsByUser(userID, model.OperationWithdrawal)
}

func (srv *Service) ProcessOrder(orderNumber string) {
	srv.tasks <- model.OrderTask{
		OrderNumber: orderNumber,
		NextCheck:   time.Now(),
	}
}

func (srv *Service) worker(tasks chan model.OrderTask) {
	for task := range tasks {
		if time.Now().Before(task.NextCheck) {
			time.Sleep(time.Second)
			tasks <- task
			continue
		}

		wait := 5 * time.Second
		resp, dur, err := srv.getAccrual(task.OrderNumber)
		if err != nil {
			if dur != 0 {
				wait = dur
			}
			if errors.Is(err, model.ErrToManyRequests) || errors.Is(err, model.ErrOrderNotRegistered) {
				task.NextCheck = time.Now().Add(wait)
				tasks <- task
			}
			continue
		}

		if resp.Status == model.StatusAccrualProcessing || resp.Status == model.StatusAccrualRegistered {
			task.NextCheck = time.Now().Add(wait)
			tasks <- task
			continue
		}

		srv.updateOrder(resp)
	}
}

func (srv *Service) getAccrual(orderNumber string) (model.ResponseAccrualService, time.Duration, error) {
	resp, err := srv.client.Get(srv.cnf.AccrualAddr.ServerAddress + "/api/orders/" + orderNumber)
	if err != nil {
		return model.ResponseAccrualService{}, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		retryAfter := time.Second * 5
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		return model.ResponseAccrualService{}, retryAfter, model.ErrToManyRequests
	}

	if resp.StatusCode == 204 {
		return model.ResponseAccrualService{}, 0, model.ErrOrderNotRegistered
	}

	var response model.ResponseAccrualService
	decoder := json.NewDecoder(resp.Body)
	if err = decoder.Decode(&response); err != nil {
		return model.ResponseAccrualService{}, 0, err
	}
	return response, 0, nil
}

func (srv *Service) updateOrder(resp model.ResponseAccrualService) {
	status := model.StatusInvalid
	if resp.Status == model.StatusAccrualProcessed {
		status = model.StatusProcessed
	}
	var order = &model.Order{
		Number:  resp.Order,
		Status:  status,
		Accrual: resp.Accrual,
	}
	_ = srv.repo.UpdateOrder(order)
}

func (srv *Service) Ping(ctx context.Context) error {
	return srv.repo.Ping(ctx)
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
