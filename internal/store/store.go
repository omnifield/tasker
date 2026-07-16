// Package store — обёртка sqlc-ядра: открытие БД (SQLite сейчас, PG — drop-in позже),
// идемпотентные goose-миграции на старте, транзакция-хелпер для атомарных сервис-операций
// (dual-id seq + INSERT в одной Tx). Этот файл РУЧНОЙ — sqlc не трогает его при regenerate.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/omnifield/tasker/migrations"
	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite" // pure-Go SQLite драйвер (без cgo -> go test -race и CI простые)
)

// Store — соединение + sqlc-Queries. Встраивает *Queries: методы sqlc доступны прямо на Store.
type Store struct {
	DB *sql.DB
	*Queries
}

// Open открывает SQLite по пути (":memory:" допустим для тестов), прогоняет миграции
// и возвращает готовый Store. FK enforce'им и в приложении (сервис-слой), и через PRAGMA.
func Open(ctx context.Context, dbPath string) (*Store, error) {
	dsn := sqliteDSN(dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	// SQLite + database/sql: единственное соединение сериализует запись -> нет "database is
	// locked" под конкурентной нагрузкой/‑race. v0-масштаб это устраивает; PG снимет лимит.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", dbPath, err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{DB: db, Queries: New(db)}, nil
}

// Close закрывает пул соединений.
func (s *Store) Close() error { return s.DB.Close() }

// Tx выполняет fn в транзакции: коммит при nil-ошибке, иначе rollback. Внутри fn работаем
// через переданный *Queries (WithTx), чтобы все запросы шли в одной транзакции.
func (s *Store) Tx(ctx context.Context, fn func(q *Queries) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(s.WithTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func sqliteDSN(path string) string {
	// _pragma в DSN применяется к КАЖДОМУ соединению пула (per-conn state). FK — на всякий,
	// хотя основной enforce в сервис-слое (Jira-урок). busy_timeout — страховка от гонок.
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	return "file:" + path + "?" + q.Encode()
}

func migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
