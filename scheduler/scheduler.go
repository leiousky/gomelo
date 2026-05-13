package scheduler

import (
	"container/heap"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type Task struct {
	UID      string
	Route    string
	Message  any
	ServerID string
}

type Scheduler struct {
	tasks   chan *Task
	workers int
	wg      sync.WaitGroup
	mu      sync.RWMutex
	closed  bool
	handler TaskHandler
	stopOnce sync.Once
}

type TaskHandler interface {
	HandlePushToUID(uid string, route string, msg any)
	HandlePushToServer(serverID string, route string, msg any)
}

func (p *Scheduler) SetHandler(h TaskHandler) {
	p.mu.Lock()
	p.handler = h
	p.mu.Unlock()
}

func New(workers int, queueSize int) *Scheduler {
	if workers <= 0 {
		workers = 4
	}
	if queueSize <= 0 {
		queueSize = 1024
	}
	return &Scheduler{
		tasks:   make(chan *Task, queueSize),
		workers: workers,
	}
}

func (p *Scheduler) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

func (p *Scheduler) Stop() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	p.stopOnce.Do(func() {
		close(p.tasks)
	})
	p.wg.Wait()
}

func (p *Scheduler) worker(id int) {
	defer p.wg.Done()

	for task := range p.tasks {
		p.dispatch(task)
	}
}

func (p *Scheduler) dispatch(task *Task) {
	p.mu.RLock()
	handler := p.handler
	p.mu.RUnlock()

	if handler == nil {
		return
	}

	if task.UID != "" {
		handler.HandlePushToUID(task.UID, task.Route, task.Message)
	} else if task.ServerID != "" {
		handler.HandlePushToServer(task.ServerID, task.Route, task.Message)
	}
}

func (p *Scheduler) Push(task *Task) {
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("scheduler push panic: %v", r)
		}
	}()
	select {
	case p.tasks <- task:
	default:
	}
}

func (p *Scheduler) PushToUID(uid string, route string, msg any) {
	p.Push(&Task{UID: uid, Route: route, Message: msg})
}

func (p *Scheduler) PushToServer(serverID string, route string, msg any) {
	p.Push(&Task{ServerID: serverID, Route: route, Message: msg})
}

type Dispatcher struct {
	scheduler *Scheduler
	routers   map[string]func(*Task) string
	mu        sync.RWMutex
	stats     struct {
		TotalPushes  int64
		FailedPushes int64
		PendingTasks int64
	}
}

func NewDispatcher(workers int) *Dispatcher {
	return &Dispatcher{
		scheduler: New(workers, 1024),
		routers:   make(map[string]func(*Task) string),
	}
}

func (d *Dispatcher) Start() {
	d.scheduler.Start()
}

func (d *Dispatcher) Stop() {
	d.scheduler.Stop()
}

func (d *Dispatcher) SetRouter(serverType string, router func(*Task) string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.routers[serverType] = router
}

func (d *Dispatcher) Push(route string, msg any, uids ...string) {
	atomic.AddInt64(&d.stats.TotalPushes, 1)
	for _, uid := range uids {
		d.scheduler.PushToUID(uid, route, msg)
	}
}

func (d *Dispatcher) Broadcast(serverType string, route string, msg any) {
	atomic.AddInt64(&d.stats.TotalPushes, 1)
	d.scheduler.PushToServer(serverType, route, msg)
}

func (d *Dispatcher) GetStats() (total, failed, pending int64) {
	total = atomic.LoadInt64(&d.stats.TotalPushes)
	failed = atomic.LoadInt64(&d.stats.FailedPushes)
	pending = atomic.LoadInt64(&d.stats.PendingTasks)
	return
}

type TaskExt struct {
	Task
	Priority   int
	RetryCount int
	MaxRetries int
	ExpireTime time.Time
}

type PriorityScheduler struct {
	tasks   []*TaskExt
	mu      sync.Mutex
	cond    *sync.Cond
	closed  bool
	workers int
	wg      sync.WaitGroup
}

func NewPriorityScheduler(workers int) *PriorityScheduler {
	p := &PriorityScheduler{
		workers: workers,
		mu:      sync.Mutex{},
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *PriorityScheduler) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

func (p *PriorityScheduler) Stop() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.cond.Broadcast()
	p.wg.Wait()
}

type taskHeap []*TaskExt

func (h taskHeap) Len() int           { return len(h) }
func (h taskHeap) Less(i, j int) bool { return h[i].Priority > h[j].Priority }
func (h taskHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(*TaskExt))
}
func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

func (p *PriorityScheduler) Add(task *TaskExt) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if task.ExpireTime.IsZero() {
		task.ExpireTime = time.Now().Add(5 * time.Minute)
	}

	heap.Push((*taskHeap)(&p.tasks), task)
	p.cond.Signal()
}

func (p *PriorityScheduler) worker(id int) {
	defer p.wg.Done()

	for {
		p.mu.Lock()
		for len(p.tasks) == 0 && !p.closed {
			p.cond.Wait()
		}

		if p.closed {
			for len(p.tasks) > 0 {
				taskInterface := heap.Pop((*taskHeap)(&p.tasks))
				if taskInterface == nil {
					break
				}
				task := taskInterface.(*TaskExt)
				if !time.Now().After(task.ExpireTime) {
					p.processTask(task)
				}
			}
			p.mu.Unlock()
			return
		}

		if len(p.tasks) == 0 {
			p.mu.Unlock()
			continue
		}

		if time.Now().After(p.tasks[0].ExpireTime) {
			heap.Pop((*taskHeap)(&p.tasks))
			p.mu.Unlock()
			continue
		}

		taskInterface := heap.Pop((*taskHeap)(&p.tasks))
		if taskInterface == nil {
			p.mu.Unlock()
			continue
		}
		task := taskInterface.(*TaskExt)
		p.mu.Unlock()
		p.processTask(task)
	}
}

func (p *PriorityScheduler) processTask(task *TaskExt) {
	if task == nil {
		return
	}
	if task.RetryCount > 0 && task.Task.UID != "" {
		task.RetryCount--
	}
}
