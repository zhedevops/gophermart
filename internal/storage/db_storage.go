package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/zhedevops/gophermart/internal/model"
)

type DBStorage struct {
	db *pgxpool.Pool
}

func NewDBStorage(pool *pgxpool.Pool) *DBStorage {
	return &DBStorage{
		db: pool,
	}
}

func (dbs *DBStorage) SetOrder(order *model.Order) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5000*time.Millisecond)
	defer cancel()
	tx, err := dbs.db.Begin(ctx)
	if err != nil {
		return err
	}
	sql := `INSERT INTO orders (number, status, user_id) 
			VALUES ($1, $2, $3) 
			ON CONFLICT (number_id) 
			    DO NOTHING
			RETURNING id;`
	err = tx.QueryRow(ctx, sql, order.Number, order.Status, order.UserID).Scan(&order.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
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
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (dbs *DBStorage) GetBalance(userID uint32) (*model.Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var acc = &model.Account{}
	sql := `SELECT deposit, withdrawn FROM accounts WHERE user_id = $1 LIMIT 1`
	err := dbs.db.QueryRow(ctx, sql, userID).Scan(&acc.Deposit, &acc.Withdrawn)
	if errors.Is(err, pgx.ErrNoRows) {
		return acc, err
	}
	return acc, nil
}

func (dbs *DBStorage) SetWithdraw(order *model.Order) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5000*time.Millisecond)
	defer cancel()
	tx, err := dbs.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	sql := `SELECT id FROM orders WHERE user_id = $1 and number = $2`
	err = dbs.db.QueryRow(ctx, sql, order.UserID, order.Number).Scan(&order.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrOrderNotFound
	}
	if err != nil {
		return err
	}
	var deposit decimal.Decimal
	sql = `SELECT deposit FROM accounts WHERE user_id = $1`
	err = dbs.db.QueryRow(ctx, sql, order.UserID, order.Withdraw).Scan(&deposit)
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
	sql = `INSERT INTO order_operations (operation, summ, status, order_id)	VALUES ($1, $2, $3, $4);`
	_, err = tx.Exec(ctx, sql, model.OperationWithdrawal, order.Withdraw, model.StatusProcessing, order.ID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (dbs *DBStorage) Ping(ctx context.Context) error {
	return dbs.db.Ping(ctx)
}

func (dbs *DBStorage) CreateUser(user model.User) (model.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5000*time.Millisecond)
	defer cancel()
	tx, err := dbs.db.Begin(ctx)
	if err != nil {
		return user, err
	}
	sql := `INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id, created_at;`
	err = tx.QueryRow(ctx, sql, user.Login, user.PasswordHash).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		_ = tx.Rollback(ctx)
		return user, err
	}
	return user, tx.Commit(ctx)
}

func (dbs *DBStorage) FindUser(user model.User) (model.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5000*time.Millisecond)
	defer cancel()
	sql := `SELECT id FROM users WHERE login = $1 and password_hash = $2`
	err := dbs.db.QueryRow(ctx, sql, user.Login, user.PasswordHash).Scan(&user.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return user, model.ErrUserNotFound
	}
	return user, nil
}

func (dbs *DBStorage) GetOrdersByUser(userID uint32, operation model.OrderOperation) ([]*model.Order, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var orders []*model.Order
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
			&ord.Accrual,
		)
		if err != nil {
			return orders, err
		}
		orders = append(orders, &ord)
	}
	if err := rows.Err(); err != nil {
		return orders, err
	}
	return orders, nil
}
