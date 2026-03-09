package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/zhedevops/gophermart/internal/config"
	"github.com/zhedevops/gophermart/internal/model"
	"github.com/zhedevops/gophermart/internal/service"
)

type Handler struct {
	service *service.Service
	Cfg     *config.Config
}

func NewHandler(s *service.Service, cnf *config.Config) *Handler {
	return &Handler{
		service: s,
		Cfg:     cnf,
	}
}

func (h *Handler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var req model.RequestUser
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "cannot decode request JSON body", http.StatusBadRequest)
		return
	}
	if req.Login == "" || req.Password == "" {
		http.Error(w, "login and password required", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	user, err := h.service.GetNewUser(req.Login, req.Password)
	if err != nil {
		if errors.Is(err, model.ErrConflict) {
			h.setErrorResponseOnConflict(w)
			return
		}
		http.Error(w, "cannot create user", http.StatusInternalServerError)
		return
	}
	h.setupCookie(w, user)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req model.RequestUser
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "cannot decode request JSON body", http.StatusBadRequest)
		return
	}
	if req.Login == "" || req.Password == "" {
		http.Error(w, "login and password required", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	user, err := h.service.AuthentificateUser(req.Login, req.Password)
	if err != nil {
		if errors.Is(err, model.ErrUserNotFound) {
			writeJSONError(w, http.StatusUnauthorized, "invalid username/password", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "cannot login user", err.Error())
		return
	}
	h.setupCookie(w, user)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) OrdersHandler(w http.ResponseWriter, r *http.Request) {
	user, err := h.handleCookie(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	order := strings.TrimSpace(string(body))
	err = h.service.SetOrder(user, order)
	if err != nil {
		if errors.Is(err, model.ErrConflict) {
			h.setErrorResponseOnConflict(w)
			return
		}
		if errors.Is(err, model.ErrWrongOrderNumber) {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if errors.Is(err, model.ErrOrderAlreadyExists) {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) ListOrdersHandler(w http.ResponseWriter, r *http.Request) {
	user, err := h.handleCookie(w, r)
	if err != nil {
		if errors.Is(err, model.ErrUserNotAuthenticated) {
			writeJSONError(w, http.StatusUnauthorized, "user not authenticated", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "handleCookie_failure", err.Error())
		return
	}
	orders, err := h.service.GetUserOrders(user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "service_ListOrdersHandler_failure", err.Error())
	}
	if orders == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	resp := []model.ResponseUserOrders{}
	for _, o := range orders {
		var accrual *decimal.Decimal

		if !o.Accrual.IsZero() {
			accrual = &o.Accrual
		}
		resp = append(resp, model.ResponseUserOrders{
			Number:     o.Number,
			Status:     model.StatusMap[o.Status],
			Accrual:    accrual,
			UploadedAt: o.UploadedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(resp); err != nil {
		log.Error().Err(err).Msg("error encoding response")
	}
}

func (h *Handler) BalanceHandler(w http.ResponseWriter, r *http.Request) {
	user, err := h.handleCookie(w, r)
	if err != nil {
		if errors.Is(err, model.ErrUserNotAuthenticated) {
			writeJSONError(w, http.StatusUnauthorized, "user not authenticated", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "handleCookie_failure", err.Error())
		return
	}
	account, err := h.service.GetBalance(user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "service_BalanceHandler_failure", err.Error())
	}
	resp := model.ResponseBalance{
		Current:   account.Deposit,
		Withdrawn: account.Withdrawn,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(resp); err != nil {
		log.Error().Err(err).Msg("error encoding response")
	}
}

func (h *Handler) BalanceWithdrawHandler(w http.ResponseWriter, r *http.Request) {
	user, err := h.handleCookie(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req model.RequestWithdraw
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "cannot decode request JSON body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	err = h.service.SetWithdraw(user, req)
	if err != nil {
		if errors.Is(err, model.ErrInsufficientFunds) {
			writeJSONError(w, http.StatusPaymentRequired, err.Error(), err.Error())
			return
		}
		if errors.Is(err, model.ErrWrongOrderNumber) {
			writeJSONError(w, http.StatusUnprocessableEntity, err.Error(), err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "service_BalanceWithdrawHandler_failure", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) PingHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := h.service.Ping(ctx)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "service_Ping_failure", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) setErrorResponseOnConflict(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusConflict)
}

func (h *Handler) handleCookie(w http.ResponseWriter, r *http.Request) (model.User, error) {
	var user = model.User{}
	cookieAuth, err := r.Cookie("Authorization")
	if err != nil {
		return user, model.ErrUserNotAuthenticated
	}
	user, err = h.service.CheckAuthCookie(cookieAuth)
	if err != nil {
		return user, model.ErrUserNotAuthenticated //todo error
	}
	return user, nil
}

func (h *Handler) setupCookie(w http.ResponseWriter, user model.User) {
	ac := h.service.GetAuthCookie(user)
	http.SetCookie(w, &http.Cookie{
		Name:     "Authorization",
		Value:    ac,
		Path:     "/",
		HttpOnly: true,
	})
}

func writeJSONError(w http.ResponseWriter, status int, errCode, msg string) {
	err := model.ErrorResponse{
		Error:   errCode,
		Message: msg,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(err)
}
