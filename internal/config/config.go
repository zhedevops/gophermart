package config

import (
	"errors"
	"flag"
	"log"
	"net/url"
	"strings"

	"github.com/caarlos0/env/v6"
	"github.com/joho/godotenv"
)

var scheme = "http://"
var defaultAddress = "localhost:8080"

type netAddress struct {
	ServerAddress string
	withScheme    bool
}

type EnvParams struct {
	ServerAddr  *string `env:"RUN_ADDRESS"`
	AccrualAddr *string `env:"ACCRUAL_SYSTEM_ADDRESS"`
	LogLevel    *string `env:"LOG_LEVEL"`
	DatabaseDsn *string `env:"DATABASE_URI"`
}

type Config struct {
	ServerAddr  *netAddress
	AccrualAddr *netAddress
	LogLevel    string
	DatabaseDsn string
}

var cfg = &Config{
	ServerAddr:  &netAddress{ServerAddress: defaultAddress, withScheme: false},
	AccrualAddr: &netAddress{ServerAddress: scheme + defaultAddress, withScheme: true},
}

func (addr *netAddress) String() string {
	return addr.ServerAddress
}

func (addr *netAddress) Set(flagVal string) error {
	if !strings.Contains(flagVal, "://") {
		flagVal = scheme + flagVal
	}
	u, err := url.Parse(flagVal)
	if err != nil {
		return errors.New("need url in a form protocol:host:port")
	}
	protocol := u.Scheme
	host := u.Hostname()
	port := u.Port()

	if host == "" || port == "" {
		return errors.New("host or port is empty")
	}

	addr.ServerAddress = host + ":" + port
	if addr.withScheme {
		addr.ServerAddress = protocol + "://" + addr.ServerAddress
	}
	return nil
}

func SetConfig() {
	SetConfigByFlag()
	parseEnvParams()
}

func GetConfig() *Config {
	return cfg
}

func parseEnvParams() {
	_ = godotenv.Load(".env")
	var params EnvParams
	err := env.Parse(&params)
	if err != nil {
		log.Fatal(err)
	}

	if params.ServerAddr != nil {
		cfg.ServerAddr.ServerAddress = *params.ServerAddr
	}
	if params.AccrualAddr != nil {
		cfg.AccrualAddr.ServerAddress = *params.AccrualAddr
	}
	if params.LogLevel != nil {
		cfg.LogLevel = *params.LogLevel
	}

	if params.DatabaseDsn != nil {
		cfg.DatabaseDsn = *params.DatabaseDsn
	}
}

func SetConfigByFlag() {
	flag.Var(cfg.ServerAddr, "a", "server address host:port")
	flag.Var(cfg.AccrualAddr, "r", "accrual system address protocol://host:port")
	flag.StringVar(&cfg.DatabaseDsn, "d", "", "db dsn")
	flag.StringVar(&cfg.LogLevel, "l", "info", "log level")
	flag.Parse()
}
