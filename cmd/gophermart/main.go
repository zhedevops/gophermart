package main

import (
	"errors"
	"log"
	"strings"

	"github.com/zhedevops/gophermart/internal/config"
	"github.com/zhedevops/gophermart/internal/database"
	"github.com/zhedevops/gophermart/internal/handler"
	"github.com/zhedevops/gophermart/internal/logger"
	"github.com/zhedevops/gophermart/internal/repository"
	"github.com/zhedevops/gophermart/internal/router"
	"github.com/zhedevops/gophermart/internal/service"
	"github.com/zhedevops/gophermart/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	config.SetConfig()
	cnf := config.GetConfig()

	if err := logger.Initialize(cnf.LogLevel); err != nil {
		return err
	}

	var completion func()

	var st repository.Repository
	dsn := strings.TrimSpace(cnf.DatabaseDsn)
	if dsn == "" {
		return errors.New("dsn is empty")
	}
	pool, err := database.ConnectDB(dsn)
	if err != nil {
		return err
	}
	st = storage.NewDBStorage(pool)
	completion = func() {
		log.Println("database pool closed")
		database.CloseDB(pool)
	}
	srv := service.NewService(st)
	h := handler.NewHandler(srv, cnf)

	if err := router.Serve(h); err != nil {
		return err
	}

	completion()

	return nil
}
