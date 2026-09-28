package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agents "github.com/eru-os/eru/eru-ai/agents"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_utils "github.com/eru-os/eru/eru-utils"
)

const (
	forwarderQueue     = 512
	forwarderBatchSize = 64
	forwarderInterval  = 100 * time.Millisecond
	forwarderTimeout   = 2 * time.Second
	// forwarderCloseWait bounds how long a finished sub-agent waits for its last
	// batch to go out. Relaying must never hold up a response, so past this the
	// handler returns and the flush finishes in the background.
	forwarderCloseWait = 500 * time.Millisecond
)

type streamEventBatch struct {
	Events []agents.StreamEvent `json:"events"`
}

// streamForwarder relays a sub-agent's live events to the pod holding the
// client's SSE connection. Delivery is batched and best-effort: a dropped batch
// costs some thinking output, whereas blocking here would stall the sub-agent's
// model loop, so the queue drops rather than waits and the POST is never awaited
// by the agent.
//
// Events are posted straight to the originating pod, not relayed hop by hop, so a
// grandchild's events reach the client with one network call and their original
// attribution intact.
type streamForwarder struct {
	url       string
	agentName string
	chain     []string
	events    chan agents.StreamEvent
	stop      chan struct{}
	wg        sync.WaitGroup
	seq       int64
	dropped   int64
	stopOnce  sync.Once
}

func newStreamForwarder(target agents.StreamTarget, projectId string, tenantId string, agentName string, chain []string) *streamForwarder {
	f := &streamForwarder{
		url:       fmt.Sprint(strings.TrimSuffix(target.CallbackUrl, "/"), "/", projectId, "/", tenantId, "/stream/", target.StreamId, "/event"),
		agentName: agentName,
		chain:     chain,
		events:    make(chan agents.StreamEvent, forwarderQueue),
		stop:      make(chan struct{}),
	}
	f.wg.Add(1)
	go f.run()
	return f
}

func (f *streamForwarder) Callback() agents.StreamCallback {
	return func(event agents.StreamEvent) {
		event = event.Attribute(f.agentName, f.chain)
		event.Seq = atomic.AddInt64(&f.seq, 1)
		select {
		case f.events <- event:
		default:
			atomic.AddInt64(&f.dropped, 1)
		}
	}
}

func (f *streamForwarder) run() {
	defer f.wg.Done()
	ticker := time.NewTicker(forwarderInterval)
	defer ticker.Stop()

	var batch []agents.StreamEvent
	flush := func() {
		if len(batch) == 0 {
			return
		}
		f.post(batch)
		batch = nil
	}

	for {
		select {
		case event := <-f.events:
			batch = append(batch, event)
			if len(batch) >= forwarderBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-f.stop:
			for {
				select {
				case event := <-f.events:
					batch = append(batch, event)
					if len(batch) >= forwarderBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (f *streamForwarder) post(batch []agents.StreamEvent) {
	// A detached context: the agent's request may already be returning, and the
	// relay must not be cancelled with it.
	ctx, cancel := context.WithTimeout(context.Background(), forwarderTimeout)
	defer cancel()
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	if _, _, _, statusCode, err := eru_utils.CallHttp(ctx, http.MethodPost, f.url, headers, nil, nil, nil, streamEventBatch{Events: batch}); err != nil {
		logs.WithContext(ctx).Debug(fmt.Sprint("stream forward failed (", len(batch), " event(s) dropped) : ", err.Error()))
	} else if statusCode >= 400 {
		logs.WithContext(ctx).Debug(fmt.Sprint("stream forward rejected with status ", statusCode, " (", len(batch), " event(s) dropped)"))
	}
}

func (f *streamForwarder) Close() {
	f.stopOnce.Do(func() { close(f.stop) })
	drained := make(chan struct{})
	go func() {
		f.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(forwarderCloseWait):
	}
	if dropped := atomic.LoadInt64(&f.dropped); dropped > 0 {
		logs.WithContext(context.Background()).Info(fmt.Sprint("stream forwarder for agent ", f.agentName, " dropped ", dropped, " event(s) under load"))
	}
}
