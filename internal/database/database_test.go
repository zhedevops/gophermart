package database

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
)

func TestConnectDBWithoutDsn(t *testing.T) {
	a := assert.New(t)
	_, err := ConnectDB("")
	a.NotNil(err)
}

func TestConnectDBWithDsn(t *testing.T) {
	a := assert.New(t)
	_ = godotenv.Load("../../.env")
	dsn, dsnErr := os.LookupEnv("DATABASE_DSN")
	if dsn == "" {
		t.Skip("dsn is required")
	}
	a.True(dsnErr)
	pool, err := ConnectDB(dsn)
	a.Nil(err)
	a.NotNil(pool)
	a.IsType(&pgxpool.Pool{}, pool)
	CloseDB(pool)
	ctx := context.Background()
	err = pool.Ping(ctx)
	a.NotNil(err)
}
