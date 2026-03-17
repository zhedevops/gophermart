package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/zhedevops/gophermart/internal/model"
)

var contextTimeout = 5 * time.Second

type DBStorage struct {
	db *pgxpool.Pool
}

func NewDBStorage(pool *pgxpool.Pool) *DBStorage {
	return &DBStorage{
		db: pool,
	}
}

func withTx(ctx context.Context, db *pgxpool.Pool, fn func(ctx context.Context, tx pgx.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, contextTimeout)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			log.Error().Err(err).Msgf("tx rollback failed: %v", err)
		}
	}()

	if err := fn(ctx, tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (dbs *DBStorage) SetOrder(order *model.Order) error {
	return withTx(context.Background(), dbs.db, func(ctx context.Context, tx pgx.Tx) error {
		sql := `INSERT INTO orders (number, status, user_id) 
			VALUES ($1, $2, $3) 
			ON CONFLICT (number) 
			    DO NOTHING
			RETURNING id;`
		err := tx.QueryRow(ctx, sql, order.Number, order.Status, order.UserID).Scan(&order.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			var userID uint32
			err := dbs.db.QueryRow(ctx, `SELECT user_id FROM orders WHERE number = $1`, order.Number).Scan(&userID)
			if errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if userID == order.UserID {
				return model.ErrOrderAlreadyExists
			}
			return model.ErrConflict
		}
		if err != nil {
			return err
		}
		return nil
	})
}

func (dbs *DBStorage) UpdateOrder(order *model.Order) error {
	return withTx(context.Background(), dbs.db, func(ctx context.Context, tx pgx.Tx) error {
		sql := `UPDATE orders SET status = $1 WHERE number = $2 RETURNING id, user_id`
		err := tx.QueryRow(ctx, sql, order.Status, order.Number).Scan(&order.ID, &order.UserID)
		if err != nil {
			return err
		}
		if order.Accrual != nil && !order.Accrual.IsZero() {
			sql = `INSERT INTO order_operations (operation, summ, order_id)	VALUES ($1, $2, $3);`
			_, err = tx.Exec(ctx, sql, model.OperationAccrual, order.Accrual, order.ID)
			if err != nil {
				return err
			}
			sql = `UPDATE accounts SET deposit = deposit + $1 WHERE user_id=$2`
			_, err = tx.Exec(ctx, sql, order.Accrual, order.UserID)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (dbs *DBStorage) SetWithdraw(order *model.Order) error {
	return withTx(context.Background(), dbs.db, func(ctx context.Context, tx pgx.Tx) error {
		sql := `SELECT id FROM orders WHERE user_id = $1 and number = $2`
		err := tx.QueryRow(ctx, sql, order.UserID, order.Number).Scan(&order.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			sql = `INSERT INTO orders (number, status, user_id) 
			VALUES ($1, $2, $3) 
			ON CONFLICT (number) 
			    DO NOTHING
			RETURNING id;`
			err = tx.QueryRow(ctx, sql, order.Number, model.StatusNew, order.UserID).Scan(&order.ID)
			if err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		var deposit decimal.Decimal
		sql = `SELECT deposit FROM accounts WHERE user_id = $1`
		err = dbs.db.QueryRow(ctx, sql, order.UserID).Scan(&deposit)
		if err != nil {
			return err
		}
		if deposit.LessThan(order.Withdraw) {
			return model.ErrInsufficientFunds
		}
		sql = `UPDATE accounts SET withdrawn = withdrawn + $1, deposit = deposit - $1 WHERE user_id=$2 AND deposit >= $1`
		_, err = tx.Exec(ctx, sql, order.Withdraw, order.UserID)
		if err != nil {
			return err
		}
		sql = `INSERT INTO order_operations (operation, summ, order_id)	VALUES ($1, $2, $3);`
		_, err = tx.Exec(ctx, sql, model.OperationWithdrawal, order.Withdraw, order.ID)
		if err != nil {
			return err
		}
		return nil
	})
}

func (dbs *DBStorage) GetBalance(userID uint32) (*model.Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	var acc = &model.Account{}
	sql := `SELECT deposit, withdrawn FROM accounts WHERE user_id = $1 LIMIT 1`
	err := dbs.db.QueryRow(ctx, sql, userID).Scan(&acc.Deposit, &acc.Withdrawn)
	if errors.Is(err, pgx.ErrNoRows) {
		return acc, err
	}
	return acc, nil
}

func (dbs *DBStorage) CreateUser(user model.User) (model.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	tx, err := dbs.db.Begin(ctx)
	if err != nil {
		return user, err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			log.Error().Err(err).Msgf("tx rollback failed: %v", err)
		}
	}()
	sql := `INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id, created_at;`
	err = tx.QueryRow(ctx, sql, user.Login, user.PasswordHash).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return user, err
	}
	sql = `INSERT INTO accounts (deposit, withdrawn, user_id) VALUES ($1, $2, $3);`
	_, err = tx.Exec(ctx, sql, 0, 0, user.ID)
	if err != nil {
		return user, err
	}
	return user, tx.Commit(ctx)
}

func (dbs *DBStorage) FindUser(user model.User) (model.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	sql := `SELECT id, password_hash FROM users WHERE login = $1`
	err := dbs.db.QueryRow(ctx, sql, user.Login).Scan(&user.ID, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return user, model.ErrUserNotFound
	}
	return user, nil
}

func (dbs *DBStorage) GetOrdersByUser(userID uint32, operation model.OrderOperation) ([]*model.Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	var orders []*model.Order
	var accrual *decimal.Decimal
	sql := `SELECT o.number, o.status, o.uploaded_at, oo.summ 
         FROM orders o
    	 LEFT JOIN order_operations oo 
    	     ON o.id = oo.order_id AND oo.operation = $2
         WHERE o.user_id = $1 
         ORDER BY o.uploaded_at DESC`
	rows, err := dbs.db.Query(ctx, sql, userID, operation)
	if err != nil {
		return orders, err
	}
	defer rows.Close()
	for rows.Next() {
		var ord model.Order
		err = rows.Scan(
			&ord.Number,
			&ord.Status,
			&ord.UploadedAt,
			&accrual,
		)
		if err != nil {
			return orders, err
		}
		ord.Accrual = accrual
		orders = append(orders, &ord)
	}
	if err := rows.Err(); err != nil {
		return orders, err
	}
	return orders, nil
}

func (dbs *DBStorage) GetWithdrawalsByUser(userID uint32, operation model.OrderOperation) ([]*model.Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	var orders []*model.Order
	var withdrawal decimal.Decimal
	sql := `SELECT o.number, o.status, oo.processed_at, oo.summ 
         FROM order_operations oo
    	 LEFT JOIN orders o 
    	     ON o.id = oo.order_id AND oo.operation = $2
         WHERE o.user_id = $1 
         ORDER BY oo.processed_at DESC`
	rows, err := dbs.db.Query(ctx, sql, userID, operation)
	if err != nil {
		return orders, err
	}
	defer rows.Close()
	for rows.Next() {
		var ord model.Order
		err = rows.Scan(
			&ord.Number,
			&ord.Status,
			&ord.UploadedAt,
			&withdrawal,
		)
		if err != nil {
			return orders, err
		}
		ord.Withdraw = withdrawal
		orders = append(orders, &ord)
	}
	if err := rows.Err(); err != nil {
		return orders, err
	}
	return orders, nil
}
