package storage

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/zhedevops/gophermart/internal/model"
)

func setupTestDB(t *testing.T) (*DBStorage, func()) {
	t.Helper()

	cmd := exec.Command("/usr/bin/docker", "compose", "-f", "../../docker-compose.test.yml", "up", "-d")
	require.NoError(t, cmd.Run(), "failed to start testdb")

	dsn := "postgres://test:test@localhost:5439/test"
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

	ctx, cancel := context.WithTimeout(context.Background(), 5000*time.Millisecond)
	defer cancel()
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	err = goose.UpContext(ctx, db, "../../migrations")
	require.NoError(t, err)

	repo := NewDBStorage(pool)

	teardown := func() {
		exec.Command("docker", "compose", "-f", "../../docker-compose.test.yml", "down").Run()
	}

	return repo, teardown
}

func TestSetOrderIntegration(t *testing.T) {
	repo, teardown := setupTestDB(t)
	defer teardown()

	var userID uint32
	err := repo.db.QueryRow(context.Background(),
		`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`,
		"testuser", "hash123",
	).Scan(&userID)
	require.NoError(t, err)

	user := model.User{ID: userID, Login: "testuser"}

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
