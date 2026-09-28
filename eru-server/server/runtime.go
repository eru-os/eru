package server

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const (
	MaxRestartAttempts = 5
	BaseRetryDelay     = time.Second * 2
	MaxRetryDelay      = time.Minute
)

type RestartBehavior int

const (
	// ShutdownOnMaxRetries - shutdown the entire service if max retries exceeded (for critical services)
	ShutdownOnMaxRetries RestartBehavior = iota
	// ContinueOnMaxRetries - stop restarting but keep service alive (for non-critical services)
	ContinueOnMaxRetries
)

var (
	globalGM *GoroutineManager
	gmMutex  sync.Mutex
)

type GoroutineManager struct {
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	shutdown chan struct{}
}

func NewGoroutineManager(ctx context.Context) *GoroutineManager {
	childCtx, cancel := context.WithCancel(ctx)
	return &GoroutineManager{
		ctx:      childCtx,
		cancel:   cancel,
		shutdown: make(chan struct{}),
	}
}

func GetGlobalGoroutineManager(ctx context.Context) *GoroutineManager {
	gmMutex.Lock()
	defer gmMutex.Unlock()

	if globalGM == nil {
		globalGM = NewGoroutineManager(ctx)
	}
	return globalGM
}

func ResetGlobalGoroutineManager() {
	gmMutex.Lock()
	defer gmMutex.Unlock()
	globalGM = nil
}

func (gm *GoroutineManager) SafeGoWithRestart(name string, fn func(ctx context.Context)) {
	gm.SafeGoWithRestartBehavior(name, fn, ContinueOnMaxRetries)
}

func (gm *GoroutineManager) SafeGoWithRestartBehavior(name string, fn func(ctx context.Context), behavior RestartBehavior) {
	gm.wg.Add(1)

	go func() {
		defer gm.wg.Done()

		attempts := 0
		for {
			select {
			case <-gm.ctx.Done():
				exitMsg := fmt.Sprintf("Worker %s exiting (context cancelled)", name)
				if logs.Logger != nil {
					logs.Logger.Info(exitMsg)
				} else {
					log.Println("INFO:", exitMsg)
				}
				return
			default:
			}

			attempts++
			panicked := false

			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
						err := fmt.Errorf("worker %s panicked (attempt %d/%d): %v\nstack trace:\n%s",
							name, attempts, MaxRestartAttempts, r, string(debug.Stack()))

						if logs.Logger != nil {
							_ = logs.Err(gm.ctx, err, "")
						} else {
							log.Println("ERROR:", err.Error())
						}
					}
				}()
				fn(gm.ctx)
			}()

			if !panicked {
				completedMsg := fmt.Sprintf("Worker %s completed normally", name)
				if logs.Logger != nil {
					logs.Logger.Info(completedMsg)
				} else {
					log.Println("INFO:", completedMsg)
				}
				return
			}

			if attempts >= MaxRestartAttempts {
				stopMsg := fmt.Sprintf("Worker %s exceeded maximum restart attempts (%d)", name, MaxRestartAttempts)

				if behavior == ShutdownOnMaxRetries {
					criticalMsg := fmt.Sprintf("%s - CRITICAL SERVICE FAILED, shutting down entire service", stopMsg)
					if logs.Logger != nil {
						logs.Logger.Error(criticalMsg)
					} else {
						log.Println("ERROR:", criticalMsg)
					}
					gm.cancel()
					return
				} else {
					nonCriticalMsg := fmt.Sprintf("%s - stopping restart attempts, service continues", stopMsg)
					if logs.Logger != nil {
						logs.Logger.Warn(nonCriticalMsg)
					} else {
						log.Println("WARN:", nonCriticalMsg)
					}
					return
				}
			}

			delay := BaseRetryDelay * time.Duration(attempts)
			if delay > MaxRetryDelay {
				delay = MaxRetryDelay
			}

			restartMsg := fmt.Sprintf("Restarting worker %s in %v (attempt %d/%d)", name, delay, attempts+1, MaxRestartAttempts)
			if logs.Logger != nil {
				logs.Logger.Info(restartMsg)
			} else {
				log.Println("INFO:", restartMsg)
			}

			select {
			case <-time.After(delay):
			case <-gm.ctx.Done():
				exitMsg := fmt.Sprintf("Worker %s exiting (context cancelled during retry delay)", name)
				if logs.Logger != nil {
					logs.Logger.Info(exitMsg)
				} else {
					log.Println("INFO:", exitMsg)
				}
				return
			}
		}
	}()
}

func (gm *GoroutineManager) SafeGo(name string, fn func(ctx context.Context)) {
	gm.wg.Add(1)

	go func() {
		defer gm.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("Worker %s panicked: %v\nStack trace:\n%s", name, r, string(debug.Stack()))
				if logs.Logger != nil {
					logs.Logger.Error(errMsg)
				} else {
					log.Println("ERROR:", errMsg)
				}
			}
		}()

		fn(gm.ctx)
		completedMsg := fmt.Sprintf("Worker %s completed normally", name)
		if logs.Logger != nil {
			logs.Logger.Info(completedMsg)
		} else {
			log.Println("INFO:", completedMsg)
		}
	}()
}

func (gm *GoroutineManager) Shutdown(timeout time.Duration) error {
	shutdownMsg := "Initiating graceful shutdown of all goroutines..."
	if logs.Logger != nil {
		logs.Logger.Info(shutdownMsg)
	} else {
		log.Println("INFO:", shutdownMsg)
	}

	gm.cancel()

	done := make(chan struct{})
	go func() {
		gm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		successMsg := "All goroutines shut down gracefully"
		if logs.Logger != nil {
			logs.Logger.Info(successMsg)
		} else {
			log.Println("INFO:", successMsg)
		}
		return nil
	case <-time.After(timeout):
		timeoutMsg := fmt.Sprintf("Graceful shutdown timeout (%v) exceeded, some goroutines may still be running", timeout)
		if logs.Logger != nil {
			logs.Logger.Warn(timeoutMsg)
		} else {
			log.Println("WARN:", timeoutMsg)
		}
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

func (gm *GoroutineManager) Context() context.Context {
	return gm.ctx
}
