// Package eventbus — in-process pub/sub на каналах.
//
// Каждый подписчик получает свою буферизованную очередь и свою горутину:
// медленный подписчик (achievements пишет в БД) не тормозит издателя и
// других подписчиков, а порядок событий для одного подписчика сохраняется.
package eventbus

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
)

const queueSize = 64

// Handler обрабатывает событие. ctx живёт, пока жив Bus.
//
// Не публикуй из Handler событие, на которое подписан он сам: при полной
// очереди горутина будет ждать сама себя.
type Handler func(ctx context.Context, e Event)

type subscriber struct {
	name string
	ch   chan Event
	h    Handler
}

// Publisher — то, что модулям-издателям нужно от шины. Сервисы принимают интерфейс,
// а не *Bus: в тестах подставляется запоминающий фейк.
type Publisher interface {
	Publish(e Event)
}

// Subscriber — то, что модулям-подписчикам (achievements) нужно от шины.
type Subscriber interface {
	Subscribe(name string, h Handler)
}

var (
	_ Publisher  = (*Bus)(nil)
	_ Subscriber = (*Bus)(nil)
)

type Bus struct {
	log  *slog.Logger
	ctx  context.Context
	done chan struct{} // закрывается в Close — сигнал «больше не принимаем»

	mu   sync.RWMutex
	subs []*subscriber

	closeOnce sync.Once
	wg        sync.WaitGroup
}

func New(log *slog.Logger) *Bus {
	return &Bus{
		log: log,
		// Свой ctx, а не ctx запроса: запрос закончится раньше, чем
		// подписчик успеет записать достижение в БД.
		ctx:  context.Background(),
		done: make(chan struct{}),
	}
}

// Subscribe регистрирует обработчик. Вызывается при старте приложения,
// до первых Publish; name нужен только для логов.
func (b *Bus) Subscribe(name string, h Handler) {
	s := &subscriber{name: name, ch: make(chan Event, queueSize), h: h}

	b.mu.Lock()
	b.subs = append(b.subs, s)
	b.mu.Unlock()

	b.wg.Add(1)
	go b.run(s)
}

// Publish рассылает событие всем подписчикам. Блокируется, только если
// чья-то очередь заполнена (backpressure вместо потери событий).
// После Close события молча отбрасываются.
func (b *Bus) Publish(e Event) {
	// Копируем срез под локом и отправляем уже без него: держать мьютекс
	// во время блокирующей отправки в канал — путь к дедлоку.
	b.mu.RLock()
	subs := b.subs
	b.mu.RUnlock()

	for _, s := range subs {
		select {
		case s.ch <- e:
		case <-b.done:
			return
		}
	}
}

// Close перестаёт принимать события, дожидается, пока подписчики разберут
// уже стоящее в очередях, и возвращается. Повторный вызов безопасен.
//
// Каналы подписчиков сознательно не закрываем: Publish из другой горутины
// мог бы отправить в закрытый канал и упасть в панику. Вместо этого общий done.
func (b *Bus) Close() {
	b.closeOnce.Do(func() { close(b.done) })
	b.wg.Wait()
}

func (b *Bus) run(s *subscriber) {
	defer b.wg.Done()
	for {
		select {
		case e := <-s.ch:
			b.dispatch(s, e)
		case <-b.done:
			// дочитываем хвост очереди и выходим
			for {
				select {
				case e := <-s.ch:
					b.dispatch(s, e)
				default:
					return
				}
			}
		}
	}
}

// dispatch изолирует панику подписчика: один сломанный обработчик не должен
// убить свою горутину (и перестать получать события) или весь процесс.
func (b *Bus) dispatch(s *subscriber, e Event) {
	defer func() {
		if rec := recover(); rec != nil {
			b.log.Error("eventbus: handler panic",
				"subscriber", s.name, "event", e, "panic", rec, "stack", string(debug.Stack()))
		}
	}()
	s.h(b.ctx, e)
}
