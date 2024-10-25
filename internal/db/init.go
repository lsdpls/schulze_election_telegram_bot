package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
)

// Структура Storage, хранящая пул соединений
type Storage struct {
	dbpool *pgxpool.Pool
}

// Конструктор для создания нового Storage
func NewStorage(dsn string) (*Storage, error) {
	dbpool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("db.NewStorage cant create connection pool: %w", err)
	}
	// Проверка соединения с базой данных
	if err = dbpool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("db.NewStorage ping error: %w", err)
	}
	return &Storage{dbpool: dbpool}, nil
}

// Close закрывает соединение с базой данных
func (s *Storage) Close() {
	s.dbpool.Close()
	logrus.Info("Storage closed")
}

// Ping проверяет соединение с базой данных
func (s *Storage) Ping(ctx context.Context) error {
	return s.dbpool.Ping(ctx)
}

func (s *Storage) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	return s.dbpool.BeginTx(ctx, opts)
}
