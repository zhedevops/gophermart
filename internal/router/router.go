package router

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zhedevops/gophermart/internal/handler"
	"github.com/zhedevops/gophermart/internal/middleware"
)

func NewRouter(h *handler.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.GzipHandle)
	r.Get("/api/user/withdrawals", h.WithdrawalsHandler)
	r.Get("/api/user/balance", h.BalanceHandler)
	r.Get("/api/user/orders", h.ListOrdersHandler)
	r.With(middleware.RequireContentType("text/plain")).Post("/api/user/orders", h.OrdersHandler)
	r.With(middleware.RequireContentType("application/json")).Post("/api/user/login", h.LoginHandler)
	r.With(middleware.RequireContentType("application/json")).Post("/api/user/register", h.RegisterHandler)
	r.With(middleware.RequireContentType("application/json")).Post("/api/user/balance/withdraw", h.BalanceWithdrawHandler)
	return r
}

func Serve(h *handler.Handler) error {
	router := NewRouter(h)
	server := &http.Server{
		Addr:    h.Cfg.ServerAddr.ServerAddress,
		Handler: router,
	}
	// Канал для получения сигналов прерывания
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		log.Println("HTTP server started")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	// Когда будет получен сигнал прерывания выполнится код
	<-signalChan

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Println("HTTP server stoped")

	return server.Shutdown(shutdownCtx)
}
