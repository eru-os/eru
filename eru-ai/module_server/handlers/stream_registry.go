package handlers

import (
	"sync"

	agents "github.com/eru-os/eru/eru-ai/agents"
)

// streamSinkBuffer bounds how many events may queue for one client's SSE
// connection. Producers never block on a full buffer - thinking is high volume
// and best-effort, and a slow client must never stall an agent's model loop or a
// sub-agent's request goroutine.
const streamSinkBuffer = 512

// streamSink is the single writer queue for one client's SSE connection.
//
// Exactly one goroutine - the writer started by AgentExecuteHandler - reads from
// events and owns the http.ResponseWriter. Every producer (the local agent loop
// and every sub-agent callback, each arriving on its own inbound request
// goroutine) only sends here. http.ResponseWriter is not safe for concurrent use:
// two goroutines writing it interleave bytes mid-frame and the client receives
// malformed SSE. Funnelling through this channel keeps that impossible while
// leaving producers non-blocking.
//
// events is never closed, so a producer that raced past the closed check simply
// sends into a queue nobody drains rather than panicking on a closed channel.
type streamSink struct {
	events chan agents.StreamEvent
	closed chan struct{}
	once   sync.Once
}

func newStreamSink() *streamSink {
	return &streamSink{
		events: make(chan agents.StreamEvent, streamSinkBuffer),
		closed: make(chan struct{}),
	}
}

// send queues an event, dropping it when the stream has ended or the buffer is
// full. It never blocks.
func (s *streamSink) send(event agents.StreamEvent) bool {
	select {
	case <-s.closed:
		return false
	default:
	}
	select {
	case s.events <- event:
		return true
	default:
		return false
	}
}

func (s *streamSink) finish() {
	s.once.Do(func() { close(s.closed) })
}

// drain delivers queued events to write until the stream is finished, then
// flushes whatever is still buffered and returns. The caller may only touch the
// ResponseWriter again after this returns.
func (s *streamSink) drain(write func(agents.StreamEvent)) {
	for {
		select {
		case event := <-s.events:
			write(event)
		case <-s.closed:
			for {
				select {
				case event := <-s.events:
					write(event)
				default:
					return
				}
			}
		}
	}
}

type streamRegistry struct {
	mu    sync.RWMutex
	sinks map[string]*streamSink
}

var activeStreams = &streamRegistry{sinks: make(map[string]*streamSink)}

func (r *streamRegistry) open(streamId string) *streamSink {
	sink := newStreamSink()
	r.mu.Lock()
	r.sinks[streamId] = sink
	r.mu.Unlock()
	return sink
}

func (r *streamRegistry) close(streamId string) {
	r.mu.Lock()
	sink := r.sinks[streamId]
	delete(r.sinks, streamId)
	r.mu.Unlock()
	if sink != nil {
		sink.finish()
	}
}

// publish hands sub-agent events to the stream that owns them. It reports how
// many were accepted; the rest were dropped because the stream ended or its
// buffer was full.
func (r *streamRegistry) publish(streamId string, events []agents.StreamEvent) int {
	r.mu.RLock()
	sink := r.sinks[streamId]
	r.mu.RUnlock()
	if sink == nil {
		return 0
	}
	accepted := 0
	for _, event := range events {
		if sink.send(event) {
			accepted++
		}
	}
	return accepted
}
