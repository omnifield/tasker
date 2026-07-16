// Package migrations вкладывает goose-миграции в бинарь, чтобы идемпотентный `up`
// прогонялся на старте без внешнего каталога (канон: миграции едут внутри артефакта).
// sqlc читает *.sql этого каталога как schema; этот .go-файл им игнорируется.
package migrations

import "embed"

// FS — встроенные *.sql миграции (goose-формат). Корень FS = каталог migrations/.
//
//go:embed *.sql
var FS embed.FS
