package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
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
		err := repo.db.QueryRow(context.Background(),
			`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`,
			user.Login, user.PasswordHash,
		).Scan(&user.ID)
		require.NoError(t, err)
		require.NotZero(t, user.ID)
	})

	t.Run("insert new user err", func(t *testing.T) {
		err := repo.db.QueryRow(context.Background(),
			`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`,
			user.Login, user.PasswordHash,
		).Scan(&user.ID)
		require.Error(t, err)
	})

	t.Run("find user ok", func(t *testing.T) {
		finded := model.User{Login: "testuser"}
		err := repo.db.QueryRow(context.Background(),
			`SELECT id, password_hash FROM users WHERE login = $1`,
			finded.Login,
		).Scan(&finded.ID, &finded.PasswordHash)
		require.NoError(t, err)
		require.Equal(t, user.ID, finded.ID)
		require.Equal(t, user.PasswordHash, finded.PasswordHash)
	})

	t.Run("find user err", func(t *testing.T) {
		finded := model.User{Login: "otheruser"}
		err := repo.db.QueryRow(context.Background(),
			`SELECT id, password_hash FROM users WHERE login = $1`,
			finded.Login,
		).Scan(&finded.ID, &finded.PasswordHash)
		require.Error(t, err)
		require.ErrorIs(t, err, pgx.ErrNoRows)
	})

	t.Run("insert new order ok", func(t *testing.T) {
		order := &model.Order{
			Number: "79927398713",
			Status: model.StatusNew,
			UserID: user.ID,
		}
		err := repo.SetOrder(order)
		require.NoError(t, err)
		require.NotZero(t, order.ID)
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
}
