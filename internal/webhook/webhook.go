// Package webhook — форма событий канон-схемы с 1-го дня (полноценная доставка — итерация 2).
// Одна канон-сущность питает и API, и webhook-payload: событие зеркалит сущность (create/
// update/remove). Сейчас sink — no-op с slog-логом; позже сюда встанет HTTP-доставка/очередь.
package webhook

import (
	"context"
	"log/slog"
)

// Action — тип изменения сущности (зеркалит REST-мутации).
type Action string

// Канон-действия над сущностью (зеркалят REST-мутации create/update/remove).
const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionRemove Action = "remove"
)

// Event — канон-конверт: что за ресурс, какое действие, и сама сущность (payload = сущность).
type Event struct {
	Action   Action `json:"action"`
	Resource string `json:"resource"` // node|relation|workspace|label|activity
	ID       string `json:"id"`       // стабильный идентификатор сущности (node.key для узлов)
	Payload  any    `json:"payload"`  // сериализованная сущность — зеркало REST-ответа
}

// Emitter — точка расширения доставки. Реализация решает, куда уходит событие.
type Emitter interface {
	Emit(ctx context.Context, e Event)
}

// LogEmitter — дефолт v0: пишет событие в лог (доказывает, что форма событий течёт по системе),
// ничего не доставляя наружу. Замена на HTTP-sink — без изменения вызывающих (сервис-слоя).
type LogEmitter struct{ Logger *slog.Logger }

// Emit логирует событие. Никогда не паникует и не блокирует вызывающего надолго.
func (l LogEmitter) Emit(ctx context.Context, e Event) {
	log := l.Logger
	if log == nil {
		log = slog.Default()
	}
	log.InfoContext(ctx, "webhook event",
		slog.String("action", string(e.Action)),
		slog.String("resource", e.Resource),
		slog.String("id", e.ID),
	)
}

// Nop — молчаливый sink (тесты): реализует Emitter, ничего не делает.
type Nop struct{}

// Emit — no-op.
func (Nop) Emit(context.Context, Event) {}
