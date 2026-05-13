package broadcast

import (
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chuhongliang/gomelo/lib"
)

var ErrSessionNil = errors.New("session is nil")
var ErrSessionNoUID = errors.New("session has no UID")

type BroadcastService interface {
	Broadcast(route string, msg any)
	BroadcastTo(uids []string, route string, msg any)
	BroadcastByIDs(ids []uint64, route string, msg any)
	Push(uid string, route string, msg any)
	PushBatch(uids []string, route string, msg any)
	Add(uids ...string) error
	Remove(uids ...string) error
	Size() int
	Clear()
	Close() error
}

type broadcastService struct {
	name      string
	route     string
	members   map[string][]*lib.Session
	byID      map[uint64][]*lib.Session
	pending   chan *broadcastTask
	batchSize int
	batchTick time.Duration
	workers   int
	wg        sync.WaitGroup
	closed    bool
	mu        sync.RWMutex

	stats struct {
		totalPush  int64
		failedPush int64
	}
}

type broadcastTask struct {
	Route string
	Msg   any
	Type  int
	UIDs  []string
	SIDs  []uint64
}

const (
	BroadcastTypeAll    = 0
	BroadcastTypeUIDs   = 1
	BroadcastTypeIDs    = 2
	BroadcastTypeSingle = 3
)

func NewBroadcast(route string, opts ...BroadcastOption) BroadcastService {
	b := &broadcastService{
		name:      route,
		route:     route,
		members:   make(map[string][]*lib.Session),
		byID:      make(map[uint64][]*lib.Session),
		pending:   make(chan *broadcastTask, 1024),
		batchSize: 50,
		batchTick: 100 * time.Millisecond,
		workers:   2,
	}

	for _, opt := range opts {
		opt(b)
	}

	for i := 0; i < b.workers; i++ {
		b.wg.Add(1)
		go b.worker(i)
	}

	return b
}

type BroadcastOption func(*broadcastService)

func WithBroadcastBatchSize(size int) BroadcastOption {
	return func(b *broadcastService) { b.batchSize = size }
}

func WithBroadcastTick(tick time.Duration) BroadcastOption {
	return func(b *broadcastService) { b.batchTick = tick }
}

func WithBroadcastWorkers(workers int) BroadcastOption {
	return func(b *broadcastService) { b.workers = workers }
}

func (b *broadcastService) worker(id int) {
	defer b.wg.Done()

	ticker := time.NewTicker(b.batchTick)
	defer ticker.Stop()

	var batch []*broadcastTask

	for {
		select {
		case task, ok := <-b.pending:
			if !ok {
				if len(batch) > 0 {
					b.flushBatch(batch)
				}
				log.Printf("broadcast worker %d exiting with %d pending tasks", id, len(batch))
				return
			}
			batch = append(batch, task)
			if len(batch) >= b.batchSize {
				b.flushBatch(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				b.flushBatch(batch)
				batch = nil
			}
		}
	}
}

func (b *broadcastService) flushBatch(batch []*broadcastTask) {
	for _, task := range batch {
		switch task.Type {
		case BroadcastTypeAll:
			b.doBroadcast(task.Route, task.Msg)
		case BroadcastTypeUIDs:
			b.doBroadcastTo(task.UIDs, task.Route, task.Msg)
		case BroadcastTypeIDs:
			b.doBroadcastByIDs(task.SIDs, task.Route, task.Msg)
		case BroadcastTypeSingle:
			if len(task.UIDs) > 0 {
				b.doBroadcastTo(task.UIDs, task.Route, task.Msg)
			}
		}
	}
}

func (b *broadcastService) doBroadcast(route string, msg any) {
	b.mu.RLock()
	members := make([]*lib.Session, 0)
	for _, sessions := range b.members {
		for _, s := range sessions {
			members = append(members, s)
		}
	}
	b.mu.RUnlock()

	for _, s := range members {
		b.pushToSession(s, route, msg)
	}
}

func (b *broadcastService) doBroadcastTo(uids []string, route string, msg any) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, uid := range uids {
		if sessions, ok := b.members[uid]; ok {
			for _, s := range sessions {
				b.pushToSession(s, route, msg)
			}
		}
	}
}

func (b *broadcastService) doBroadcastByIDs(sids []uint64, route string, msg any) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sid := range sids {
		if sessions, ok := b.byID[sid]; ok {
			for _, s := range sessions {
				b.pushToSession(s, route, msg)
			}
		}
	}
}

func (b *broadcastService) pushToSession(s *lib.Session, route string, msg any) {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddInt64(&b.stats.failedPush, 1)
			log.Printf("broadcast pushToSession panic: %v", r)
		}
	}()

	if s.IsClosed() {
		atomic.AddInt64(&b.stats.failedPush, 1)
		return
	}

	atomic.AddInt64(&b.stats.totalPush, 1)

	conn := s.Connection()
	if conn == nil {
		atomic.AddInt64(&b.stats.failedPush, 1)
		log.Printf("broadcast: session has no connection, uid=%s", s.UID())
		return
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*50) * time.Millisecond)
		}
		err := conn.Send(&lib.Message{
			Type:  lib.Broadcast,
			Route: route,
			Body:  msg,
		})
		if err == nil {
			return
		}
		lastErr = err
	}

	atomic.AddInt64(&b.stats.failedPush, 1)
	if lastErr != nil {
		log.Printf("broadcast push failed after 3 attempts: route=%s, err=%v", route, lastErr)
	}
}

func (b *broadcastService) Broadcast(route string, msg any) {
	b.mu.RLock()
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		atomic.AddInt64(&b.stats.failedPush, 1)
		return
	}
	select {
	case b.pending <- &broadcastTask{Route: route, Msg: msg, Type: BroadcastTypeAll}:
	default:
		atomic.AddInt64(&b.stats.failedPush, 1)
	}
}

func (b *broadcastService) BroadcastTo(uids []string, route string, msg any) {
	b.mu.RLock()
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		atomic.AddInt64(&b.stats.failedPush, 1)
		return
	}
	select {
	case b.pending <- &broadcastTask{Route: route, Msg: msg, Type: BroadcastTypeUIDs, UIDs: uids}:
	default:
		atomic.AddInt64(&b.stats.failedPush, 1)
	}
}

func (b *broadcastService) BroadcastByIDs(ids []uint64, route string, msg any) {
	b.mu.RLock()
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		atomic.AddInt64(&b.stats.failedPush, 1)
		return
	}
	select {
	case b.pending <- &broadcastTask{Route: route, Msg: msg, Type: BroadcastTypeIDs, SIDs: ids}:
	default:
		atomic.AddInt64(&b.stats.failedPush, 1)
	}
}

func (b *broadcastService) Push(uid string, route string, msg any) {
	b.mu.RLock()
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		atomic.AddInt64(&b.stats.failedPush, 1)
		return
	}
	select {
	case b.pending <- &broadcastTask{Route: route, Msg: msg, Type: BroadcastTypeSingle, UIDs: []string{uid}}:
	default:
		atomic.AddInt64(&b.stats.failedPush, 1)
	}
}

func (b *broadcastService) PushBatch(uids []string, route string, msg any) {
	b.BroadcastTo(uids, route, msg)
}

func (b *broadcastService) Add(uids ...string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, uid := range uids {
		if _, ok := b.members[uid]; !ok {
			s := lib.NewSession()
			s.Set("uid", uid)
			b.members[uid] = []*lib.Session{s}
		}
	}
	return nil
}

func (b *broadcastService) AddSession(session *lib.Session) error {
	if session == nil {
		return ErrSessionNil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	uid := session.UID()
	if uid == "" {
		if u, ok := session.Get("uid").(string); ok {
			uid = u
		}
	}
	if uid == "" {
		return ErrSessionNoUID
	}
	b.members[uid] = append(b.members[uid], session)
	if connID := session.GetConnectionID(); connID != 0 {
		b.byID[connID] = append(b.byID[connID], session)
	}
	return nil
}

func (b *broadcastService) Remove(uids ...string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, uid := range uids {
		if sessions, ok := b.members[uid]; ok {
			for _, s := range sessions {
				if connID := s.GetConnectionID(); connID != 0 {
					delete(b.byID, connID)
				}
			}
		}
		delete(b.members, uid)
	}
	return nil
}

func (b *broadcastService) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.members)
}

func (b *broadcastService) Clear() {
	b.mu.Lock()
	b.members = make(map[string][]*lib.Session)
	b.byID = make(map[uint64][]*lib.Session)
	b.mu.Unlock()
}

func (b *broadcastService) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	close(b.pending)
	b.mu.Unlock()

	ch := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(ch)
	}()

	select {
	case <-ch:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("close timeout")
	}
}

func (b *broadcastService) Stats() (total, failed int64) {
	total = atomic.LoadInt64(&b.stats.totalPush)
	failed = atomic.LoadInt64(&b.stats.failedPush)
	return
}

type BroadcastManager struct {
	channels map[string]BroadcastService
	mu       sync.RWMutex
}

func NewBroadcastManager() *BroadcastManager {
	return &BroadcastManager{
		channels: make(map[string]BroadcastService),
	}
}

func (m *BroadcastManager) Create(name string, opts ...BroadcastOption) (BroadcastService, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.channels[name]; ok {
		return c, false
	}

	c := NewBroadcast(name, opts...)
	m.channels[name] = c
	return c, true
}

func (m *BroadcastManager) Get(name string) (BroadcastService, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.channels[name]
	return c, ok
}

func (m *BroadcastManager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.channels[name]; ok {
		c.Close()
		delete(m.channels, name)
	}
}

func (m *BroadcastManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range m.channels {
		c.Close()
	}
	m.channels = make(map[string]BroadcastService)
}

func (m *BroadcastManager) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels)
}
