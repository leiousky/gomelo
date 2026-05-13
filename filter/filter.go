package filter

import (
	"context"
	"fmt"
	"sync"

	"github.com/chuhongliang/gomelo/lib"
)

type FilterType int

const (
	FilterTypeBefore FilterType = iota
	FilterTypeAfter
)

type Filter interface {
	Name() string
	Process(ctx *lib.Context) bool
	After(ctx *lib.Context)
}

type FilterFunc func(*lib.Context) bool

func (f FilterFunc) Name() string {
	return fmt.Sprintf("func-filter[%p]", f)
}
func (f FilterFunc) Process(ctx *lib.Context) bool { return f(ctx) }
func (f FilterFunc) After(ctx *lib.Context)        {}

type FilterChain struct {
	filters []Filter
	mu      sync.RWMutex
}

func NewFilterChain() *FilterChain {
	return &FilterChain{
		filters: make([]Filter, 0),
	}
}

func (c *FilterChain) Add(f Filter) {
	c.mu.Lock()
	c.filters = append(c.filters, f)
	c.mu.Unlock()
}

func (c *FilterChain) AddFunc(fn FilterFunc) {
	c.Add(fn)
}

func (c *FilterChain) Remove(name string) {
	c.mu.Lock()
	for i, f := range c.filters {
		if f.Name() == name {
			c.filters = append(c.filters[:i], c.filters[i+1:]...)
			break
		}
	}
	c.mu.Unlock()
}

func (c *FilterChain) Process(ctx *lib.Context) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, f := range c.filters {
		if !f.Process(ctx) {
			return false
		}
	}
	return true
}

func (c *FilterChain) After(ctx *lib.Context) {
	c.mu.RLock()
	filters := make([]Filter, len(c.filters))
	copy(filters, c.filters)
	c.mu.RUnlock()

	for _, f := range filters {
		f.After(ctx)
	}
}

func (c *FilterChain) Clear() {
	c.mu.Lock()
	c.filters = make([]Filter, 0)
	c.mu.Unlock()
}

func (c *FilterChain) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.filters)
}

type FilterManager struct {
	globalBefore *FilterChain
	globalAfter  *FilterChain
	routeBefore  map[string]*FilterChain
	routeAfter   map[string]*FilterChain
	rpcBefore    *FilterChain
	rpcAfter     *FilterChain
	mu           sync.RWMutex
}

func NewFilterManager() *FilterManager {
	return &FilterManager{
		globalBefore: NewFilterChain(),
		globalAfter:  NewFilterChain(),
		routeBefore:  make(map[string]*FilterChain),
		routeAfter:   make(map[string]*FilterChain),
		rpcBefore:    NewFilterChain(),
		rpcAfter:     NewFilterChain(),
	}
}

func (m *FilterManager) AddGlobalBefore(f Filter) {
	m.mu.Lock()
	m.globalBefore.Add(f)
	m.mu.Unlock()
}

func (m *FilterManager) AddGlobalAfter(f Filter) {
	m.mu.Lock()
	m.globalAfter.Add(f)
	m.mu.Unlock()
}

func (m *FilterManager) AddRouteBefore(route string, f Filter) {
	m.mu.Lock()
	if m.routeBefore[route] == nil {
		m.routeBefore[route] = NewFilterChain()
	}
	m.routeBefore[route].Add(f)
	m.mu.Unlock()
}

func (m *FilterManager) AddRouteAfter(route string, f Filter) {
	m.mu.Lock()
	if m.routeAfter[route] == nil {
		m.routeAfter[route] = NewFilterChain()
	}
	m.routeAfter[route].Add(f)
	m.mu.Unlock()
}

func (m *FilterManager) AddRpcBefore(f Filter) {
	m.mu.Lock()
	m.rpcBefore.Add(f)
	m.mu.Unlock()
}

func (m *FilterManager) AddRpcAfter(f Filter) {
	m.mu.Lock()
	m.rpcAfter.Add(f)
	m.mu.Unlock()
}

func (m *FilterManager) ProcessGlobalBefore(ctx *lib.Context) bool {
	return m.globalBefore.Process(ctx)
}

func (m *FilterManager) ProcessGlobalAfter(ctx *lib.Context) bool {
	return m.globalAfter.Process(ctx)
}

func (m *FilterManager) ProcessRouteBefore(route string, ctx *lib.Context) bool {
	m.mu.RLock()
	chain := m.routeBefore[route]
	m.mu.RUnlock()

	if chain == nil || chain.Count() == 0 {
		return true
	}
	return chain.Process(ctx)
}

func (m *FilterManager) ProcessRouteAfter(route string, ctx *lib.Context) bool {
	m.mu.RLock()
	chain := m.routeAfter[route]
	m.mu.RUnlock()

	if chain == nil || chain.Count() == 0 {
		return true
	}
	return chain.Process(ctx)
}

func (m *FilterManager) AfterGlobalAfter(ctx *lib.Context) {
	m.globalAfter.After(ctx)
}

func (m *FilterManager) AfterRouteAfter(route string, ctx *lib.Context) {
	m.mu.RLock()
	chain := m.routeAfter[route]
	m.mu.RUnlock()

	if chain != nil {
		chain.After(ctx)
	}
}

func (m *FilterManager) ProcessRpcBefore(ctx context.Context, service, method string) bool {
	fakeCtx := lib.NewContext(nil)
	fakeCtx.Set("service", service)
	fakeCtx.Set("method", method)
	return m.rpcBefore.Process(fakeCtx)
}

func (m *FilterManager) ProcessRpcAfter(ctx context.Context, service, method string) bool {
	fakeCtx := lib.NewContext(nil)
	fakeCtx.Set("service", service)
	fakeCtx.Set("method", method)
	return m.rpcAfter.Process(fakeCtx)
}

func (m *FilterManager) Clear() {
	m.mu.Lock()
	m.globalBefore.Clear()
	m.globalAfter.Clear()
	m.routeBefore = make(map[string]*FilterChain)
	m.routeAfter = make(map[string]*FilterChain)
	m.rpcBefore.Clear()
	m.rpcAfter.Clear()
	m.mu.Unlock()
}

type CompositeFilter struct {
	name   string
	before func(*lib.Context) bool
	after  func(*lib.Context)
}

func NewCompositeFilter(name string, before func(*lib.Context) bool, after func(*lib.Context)) *CompositeFilter {
	return &CompositeFilter{
		name:   name,
		before: before,
		after:  after,
	}
}

func (f *CompositeFilter) Name() string { return f.name }
func (f *CompositeFilter) Process(ctx *lib.Context) bool {
	if f.before != nil {
		return f.before(ctx)
	}
	return true
}
func (f *CompositeFilter) After(ctx *lib.Context) {
	if f.after != nil {
		f.after(ctx)
	}
}

func FilterFuncToFilter(name string, fn func(*lib.Context) bool) Filter {
	return &funcFilter{name: name, fn: fn}
}

type funcFilter struct {
	name string
	fn   func(*lib.Context) bool
}

func (f *funcFilter) Name() string { return f.name }
func (f *funcFilter) Process(ctx *lib.Context) bool {
	if f.fn != nil {
		return f.fn(ctx)
	}
	return true
}
func (f *funcFilter) After(ctx *lib.Context) {}
