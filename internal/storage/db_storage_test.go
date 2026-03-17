package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/zhedevops/gophermart/internal/model"
)

func setupTestDB(t *testing.T) (*DBStorage, func(db *pgxpool.Pool)) {
	t.Helper()

	_ = godotenv.Load("../../.env")
	dsn, _ := os.LookupEnv("DATABASE_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@postgres:5432/praktikum"
	}

	var pool *pgxpool.Pool
	var err error
	for i := 0; i < 30; i++ {
		pool, err = pgxpool.New(context.Background(), dsn)
		if err == nil {
			err = pool.Ping(context.Background())
			if err == nil {
				break
			}
		}
		time.Sleep(1 * time.Second)
	}
	require.NoError(t, err, "Postgres is not ready")

	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	err = goose.UpContext(ctx, db, "../../migrations")
	require.NoError(t, err)

	repo := NewDBStorage(pool)

	teardown := func(db *pgxpool.Pool) {
		_, _ = db.Exec(context.Background(), `
		TRUNCATE TABLE order_operations, orders, users, accounts RESTART IDENTITY CASCADE
	`)
	}
	return repo, teardown
}

func TestIntegration(t *testing.T) {
	repo, teardown := setupTestDB(t)
	defer teardown(repo.db)

	user := model.User{
		Login:        "testuser",
		PasswordHash: "testpassword",
	}

	t.Run("insert new user ok", func(t *testing.T) {
		createdUser, err := repo.CreateUser(user)
		require.NoError(t, err)
		require.NotZero(t, createdUser.ID)
		user.ID = createdUser.ID
	})

	t.Run("insert new user err", func(t *testing.T) {
		_, err := repo.CreateUser(user)
		require.Error(t, err)
	})

	t.Run("find user ok", func(t *testing.T) {
		finded, err := repo.FindUser(model.User{Login: "testuser"})
		require.NoError(t, err)
		require.Equal(t, user.ID, finded.ID)
		require.Equal(t, user.PasswordHash, finded.PasswordHash)
	})

	t.Run("find user err", func(t *testing.T) {
		_, err := repo.FindUser(model.User{Login: "otheruser"})
		require.Error(t, err)
		require.ErrorIs(t, err, model.ErrUserNotFound)
	})

	t.Run("insert new order ok", func(t *testing.T) {
		order := &model.Order{
			Number: "79927398713",
			Status: model.StatusNew,
			UserID: user.ID,
		}
		err := repo.SetOrder(order)
		require.NoError(t, err)
	})

	t.Run("order already exists same user", func(t *testing.T) {
		order := &model.Order{
			Number: "79927398713",
			Status: model.StatusNew,
			UserID: user.ID,
		}
		err := repo.SetOrder(order)
		require.ErrorIs(t, err, model.ErrOrderAlreadyExists)
	})

	t.Run("order exists conflict", func(t *testing.T) {
		var otherUserID uint32
		err := repo.db.QueryRow(context.Background(),
			`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`,
			"otheruser", "hash2",
		).Scan(&otherUserID)
		require.NoError(t, err)

		order := &model.Order{
			Number: "79927398713",
			Status: model.StatusNew,
			UserID: otherUserID,
		}
		err = repo.SetOrder(order)
		require.ErrorIs(t, err, model.ErrConflict)
	})

	var accrual = 999.02
	acc := decimal.NewFromFloat(accrual)
	t.Run("update order ok", func(t *testing.T) {
		order := &model.Order{
			Number:  "79927398713",
			Status:  model.StatusProcessing,
			Accrual: &acc,
		}
		err := repo.UpdateOrder(order)
		require.NoError(t, err)
	})

	t.Run("update order err", func(t *testing.T) {
		order := &model.Order{
			Number:  "79927398799",
			Status:  model.StatusProcessing,
			Accrual: &acc,
		}
		err := repo.UpdateOrder(order)
		require.Error(t, err)
	})

	var withdraw = 211.01
	wtdrw := decimal.NewFromFloat(withdraw)
	t.Run("set withdraw order ok", func(t *testing.T) {
		order := &model.Order{
			Number:   "79927398713",
			UserID:   user.ID,
			Withdraw: wtdrw,
		}
		err := repo.SetWithdraw(order)
		require.NoError(t, err)
	})

	t.Run("set withdraw order err", func(t *testing.T) {
		order := &model.Order{
			Number:   "79927398799",
			UserID:   3,
			Withdraw: wtdrw,
		}
		err := repo.SetWithdraw(order)
		require.Error(t, err)
	})

	t.Run("get user balance ok", func(t *testing.T) {
		account, err := repo.GetBalance(user.ID)
		require.NoError(t, err)
		require.Equal(t, accrual-withdraw, account.Deposit)
		require.Equal(t, withdraw, account.Withdrawn)
	})

	t.Run("get user balance err", func(t *testing.T) {
		_, err := repo.GetBalance(2)
		require.Error(t, err)
	})

	t.Run("get orders by user ok", func(t *testing.T) {
		orders, err := repo.GetOrdersByUser(user.ID, model.OperationAccrual)
		require.NoError(t, err)
		require.NotEmpty(t, orders)
		require.Equal(t, accrual, orders[0].Accrual.InexactFloat64())
	})

	t.Run("get withdrawals by user ok", func(t *testing.T) {
		orders, err := repo.GetWithdrawalsByUser(user.ID, model.OperationWithdrawal)
		require.NoError(t, err)
		require.NotEmpty(t, orders)
		require.Equal(t, withdraw, orders[0].Withdraw.InexactFloat64())
	})
}
