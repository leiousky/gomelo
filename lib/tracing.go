package lib

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type TraceID [16]byte

func (t TraceID) String() string {
	return fmt.Sprintf("%032x", t[:])
}

var traceIDSeq atomic.Uint64

func NewTraceID() TraceID {
	var t TraceID
	now := uint64(time.Now().UnixNano())
	seq := traceIDSeq.Add(1)
	binary.LittleEndian.PutUint64(t[:8], now)
	binary.LittleEndian.PutUint64(t[8:16], seq)
	return t
}

type SpanKind int

const (
	SpanKindInternal SpanKind = iota
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

type Span interface {
	TraceID() TraceID
	SpanID() uint64
	Name() string
	Kind() SpanKind
	StartTime() time.Time
	EndTime() time.Time
	Attributes() map[string]any
	SetAttribute(key string, value any)
	SetStatus(code int, message string)
	RecordError(err error)
	AddEvent(name string, attrs ...map[string]any)
	End()
	IsRecording() bool
}

type span struct {
	traceID    TraceID
	spanID     uint64
	name       string
	kind       SpanKind
	startTime  time.Time
	endTime    time.Time
	attributes map[string]any
	events     []SpanEvent
	statusCode int
	statusMsg  string
	err        error
	recording  bool
	mu         sync.RWMutex
}

type SpanEvent struct {
	Name      string
	Timestamp time.Time
	Attrs     map[string]any
}

func newSpan(name string, kind SpanKind, traceID TraceID) *span {
	return &span{
		traceID:    traceID,
		spanID:     atomic.AddUint64(&spanIDCounter, 1),
		name:       name,
		kind:       kind,
		startTime:  time.Now(),
		attributes: make(map[string]any),
		recording:  true,
	}
}

var spanIDCounter uint64

func (s *span) TraceID() TraceID     { return s.traceID }
func (s *span) SpanID() uint64       { return s.spanID }
func (s *span) Name() string         { return s.name }
func (s *span) Kind() SpanKind       { return s.kind }
func (s *span) StartTime() time.Time { return s.startTime }
func (s *span) EndTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endTime
}
func (s *span) Attributes() map[string]any { return s.attributes }
func (s *span) IsRecording() bool          { return s.recording }

func (s *span) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes[key] = value
}

func (s *span) SetStatus(code int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusCode = code
	s.statusMsg = message
}

func (s *span) RecordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *span) AddEvent(name string, attrs ...map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attrMap := make(map[string]any)
	if len(attrs) > 0 {
		attrMap = attrs[0]
	}

	s.events = append(s.events, SpanEvent{
		Name:      name,
		Timestamp: time.Now(),
		Attrs:     attrMap,
	})
}

func (s *span) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recording {
		s.endTime = time.Now()
		s.recording = false
	}
}

type Tracer interface {
	Start(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)
}

type SpanOption func(*spanConfig)

type spanConfig struct {
	kind SpanKind
}

func WithSpanKind(kind SpanKind) SpanOption {
	return func(c *spanConfig) {
		c.kind = kind
	}
}

type tracer struct {
	name   string
	output SpanExporter
	mu     sync.RWMutex
}

var globalTracer Tracer = &tracer{name: "github.com/chuhongliang/gomelo"}

func SetGlobalTracer(t Tracer) {
	globalTracer = t
}

func GlobalTracer() Tracer {
	return globalTracer
}

func (t *tracer) Start(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	cfg := &spanConfig{kind: SpanKindInternal}
	for _, opt := range opts {
		opt(cfg)
	}

	traceID, ok := GetTraceIDFromContext(ctx)
	if !ok {
		traceID = NewTraceID()
	}

	s := newSpan(name, cfg.kind, traceID)

	if t.output != nil {
		t.export(s)
	}

	newCtx := ContextWithSpan(ctx, s)
	newCtx = ContextWithTraceID(newCtx, traceID)
	return newCtx, s
}

func (t *tracer) SetExporter(exp SpanExporter) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output = exp
}

func (t *tracer) export(s Span) {
	if t.output == nil {
		return
	}
	t.output.Export(s)
}

type SpanExporter interface {
	Export(span Span)
}

type NoOpExporter struct{}

func (e *NoOpExporter) Export(span Span) {}

type ConsoleExporter struct{}

func (e *ConsoleExporter) Export(span Span) {
	fmt.Printf("span: name=%s trace=%s span=%d kind=%d duration=%v\n",
		span.Name(),
		span.TraceID().String(),
		span.SpanID(),
		span.Kind(),
		span.EndTime().Sub(span.StartTime()),
	)
}

func Trace(ctx context.Context, name string, fn func(context.Context) error) error {
	_, span := GlobalTracer().Start(ctx, name)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

func TraceRPC(ctx context.Context, service, method string) (context.Context, Span) {
	return GlobalTracer().Start(ctx, fmt.Sprintf("%s/%s", service, method),
		WithSpanKind(SpanKindClient))
}

func TraceHandler(ctx context.Context, route string) (context.Context, Span) {
	return GlobalTracer().Start(ctx, route, WithSpanKind(SpanKindServer))
}

func ContextWithSpan(ctx context.Context, s Span) context.Context {
	return context.WithValue(ctx, spanKey{}, s)
}

func SpanFromContext(ctx context.Context) (Span, bool) {
	s, ok := ctx.Value(spanKey{}).(Span)
	return s, ok
}

type spanKey struct{}

func ContextWithTraceID(ctx context.Context, id TraceID) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

func TraceIDFromContext(ctx context.Context) (TraceID, bool) {
	id, ok := ctx.Value(traceIDKey{}).(TraceID)
	return id, ok
}

func GetTraceIDFromContext(ctx context.Context) (TraceID, bool) {
	return TraceIDFromContext(ctx)
}

type TraceIDKey struct{}

type traceIDKey struct{}
