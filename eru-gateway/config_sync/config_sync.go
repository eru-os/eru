package config_sync

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eru-os/eru/eru-gateway/registry"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

const (
	ConfigUpdatedHeader  = "Eru-Config-Updated"
	ConfigInstanceHeader = "Eru-Config-Instance"
	loadPath             = "/store/load"
)

const (
	defaultTimeout     = 10 * time.Second
	defaultConcurrency = 8
)

type Notifier struct {
	registry    *registry.Registry
	client      *http.Client
	timeout     time.Duration
	concurrency int
}

func New(reg *registry.Registry) *Notifier {
	if reg == nil {
		return nil
	}
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv("CONFIG_SYNC_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	concurrency := defaultConcurrency
	if raw := strings.TrimSpace(os.Getenv("CONFIG_SYNC_CONCURRENCY")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			concurrency = parsed
		}
	}
	return &Notifier{
		registry:    reg,
		client:      &http.Client{Timeout: timeout},
		timeout:     timeout,
		concurrency: concurrency,
	}
}

func (n *Notifier) Notify(serviceName string, sourceInstanceId string) {
	if n == nil || serviceName == "" {
		return
	}
	go n.notify(serviceName, sourceInstanceId)
}

func (n *Notifier) notify(serviceName string, sourceInstanceId string) {
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("config sync fan-out panic: ", r))
		}
	}()

	instances, err := n.registry.ListServices(ctx, serviceName)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("config sync could not list instances of ", serviceName, " : ", err.Error()))
		return
	}

	targets := make([]string, 0, len(instances))
	for _, instance := range instances {
		if instance.Id == sourceInstanceId || instance.Address == "" {
			continue
		}
		targets = append(targets, fmt.Sprint(strings.TrimRight(instance.Address, "/"), loadPath))
	}
	if len(targets) == 0 {
		logs.WithContext(ctx).Info(fmt.Sprint("config sync - no peer instances of ", serviceName, " to refresh"))
		return
	}

	logs.WithContext(ctx).Info(fmt.Sprint("config sync - refreshing ", len(targets), " peer instance(s) of ", serviceName))

	sem := make(chan struct{}, n.concurrency)
	var wg sync.WaitGroup
	var failed int64
	var mu sync.Mutex

	for _, target := range targets {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := n.load(ctx, url); err != nil {
				mu.Lock()
				failed++
				mu.Unlock()
				logs.WithContext(ctx).Error(fmt.Sprint("config sync failed for ", url, " : ", err.Error()))
			}
		}(target)
	}
	wg.Wait()

	if failed > 0 {
		logs.WithContext(ctx).Error(fmt.Sprint("config sync - ", failed, " of ", len(targets), " instance(s) of ", serviceName, " failed to refresh"))
	}
}

func (n *Notifier) load(ctx context.Context, url string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	response, err := n.client.Do(req)
	if err != nil {
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned status %d", url, response.StatusCode)
	}
	return
}
