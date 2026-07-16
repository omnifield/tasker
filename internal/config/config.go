// Package config — env-only конфиг tasker (маленькая Config с явным парсом, канон go.md).
// Ноль флагов/файлов: TASKER_PORT (порт HTTP), TASKER_DB (путь к БД-волюму, не репо/tmp).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config — весь рантайм-ввод сервиса. Явные поля, явные дефолты, явная валидация.
type Config struct {
	Port int    // TASKER_PORT — слушаем 0.0.0.0:Port (G1: bind wildcard, не loopback)
	DB   string // TASKER_DB — путь к SQLite-файлу (per-product волюм); ":memory:" для тестов
}

const (
	defaultPort = 8030 // контрактный порт registry/ports.md (дверь /api/tasker -> tasker:8030)
	defaultDB   = "tasker.db"
)

// Load читает конфиг из окружения. Пустой/кривой ввод -> дефолт (кроме явно битого порта).
func Load() (Config, error) {
	c := Config{Port: defaultPort, DB: defaultDB}

	if v := strings.TrimSpace(os.Getenv("TASKER_PORT")); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return Config{}, fmt.Errorf("TASKER_PORT=%q невалиден (ожидается 1..65535)", v)
		}
		c.Port = p
	}
	if v := strings.TrimSpace(os.Getenv("TASKER_DB")); v != "" {
		c.DB = v
	}
	return c, nil
}

// Addr — адрес прослушивания. Bind 0.0.0.0 (G1): сосед по docker-сети должен достучаться,
// loopback -> 502 через дверь.
func (c Config) Addr() string {
	return fmt.Sprintf("0.0.0.0:%d", c.Port)
}
