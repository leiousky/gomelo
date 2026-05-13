package lib

import (
	"log"
	"sync"
	"sync/atomic"
)

type EventCallback func(args ...any)

type EventID uint64

type eventHandler struct {
	id       EventID
	callback EventCallback
	once     bool
}

type EventEmitter struct {
	events map[string][]*eventHandler
	mu     sync.RWMutex
	nextID uint64
}

func NewEventEmitter() *EventEmitter {
	return &EventEmitter{events: make(map[string][]*eventHandler)}
}

func (e *EventEmitter) On(event string, callback EventCallback) EventID {
	e.mu.Lock()
	id := EventID(atomic.AddUint64(&e.nextID, 1))
	e.events[event] = append(e.events[event], &eventHandler{id: id, callback: callback})
	e.mu.Unlock()
	return id
}

func (e *EventEmitter) Once(event string, callback EventCallback) EventID {
	e.mu.Lock()
	id := EventID(atomic.AddUint64(&e.nextID, 1))
	e.events[event] = append(e.events[event], &eventHandler{id: id, callback: callback, once: true})
	e.mu.Unlock()
	return id
}

func (e *EventEmitter) Off(event string, id EventID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if handlers, ok := e.events[event]; ok {
		for i, h := range handlers {
			if h.id == id {
				e.events[event] = append(handlers[:i], handlers[i+1:]...)
				return
			}
		}
	}
}

func (e *EventEmitter) Emit(event string, args ...any) {
	e.mu.RLock()
	handlers := make([]*eventHandler, len(e.events[event]))
	copy(handlers, e.events[event])
	e.mu.RUnlock()

	if len(handlers) == 0 {
		return
	}

	e.mu.Lock()
	var keep []*eventHandler
	for _, h := range handlers {
		if !h.once {
			keep = append(keep, h)
		}
	}
	e.events[event] = keep
	e.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(len(handlers))
	for _, h := range handlers {
		go func(handler *eventHandler) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("event handler panic: event=%s, err=%v", event, r)
				}
			}()
			handler.callback(args...)
		}(h)
	}
}

func (e *EventEmitter) Clear(event string) {
	e.mu.Lock()
	delete(e.events, event)
	e.mu.Unlock()
}

var globalEventID uint64

func NextEventID() EventID {
	return EventID(atomic.AddUint64(&globalEventID, 1))
}
