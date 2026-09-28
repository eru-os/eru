package config_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
)

const statusPath = "/config/status"

type InstanceStatus struct {
	InstanceId     string     `json:"instance_id"`
	Address        string     `json:"address"`
	ConfigUpdateAt *time.Time `json:"config_update_at"`
	Reloaded       *bool      `json:"reloaded,omitempty"`
	Error          string     `json:"error,omitempty"`
}

type ServiceStatus struct {
	Service   string           `json:"service"`
	InSync    bool             `json:"in_sync"`
	Instances []InstanceStatus `json:"instances"`
}

type Report struct {
	InSync   bool            `json:"in_sync"`
	Services []ServiceStatus `json:"services"`
}

type configStatusResponse struct {
	Service        string    `json:"service"`
	InstanceId     string    `json:"instance_id"`
	ConfigUpdateAt time.Time `json:"config_update_at"`
}

func (n *Notifier) Status(ctx context.Context, serviceName string) (report Report, err error) {
	return n.collect(ctx, serviceName, false)
}

func (n *Notifier) ForceSync(ctx context.Context, serviceName string) (report Report, err error) {
	return n.collect(ctx, serviceName, true)
}

func (n *Notifier) collect(ctx context.Context, serviceName string, reload bool) (report Report, err error) {
	if n == nil {
		return report, fmt.Errorf("config sync is not configured")
	}

	instances, err := n.instancesFor(ctx, serviceName)
	if err != nil {
		return
	}

	grouped := make(map[string][]eru_models.ServiceInstance)
	for _, instance := range instances {
		grouped[instance.Name] = append(grouped[instance.Name], instance)
	}

	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)

	report.InSync = true
	for _, name := range names {
		serviceStatus := n.collectService(ctx, name, grouped[name], reload)
		if !serviceStatus.InSync {
			report.InSync = false
		}
		report.Services = append(report.Services, serviceStatus)
	}
	if report.Services == nil {
		report.Services = []ServiceStatus{}
	}
	return
}

func (n *Notifier) instancesFor(ctx context.Context, serviceName string) ([]eru_models.ServiceInstance, error) {
	if serviceName != "" {
		return n.registry.ListServices(ctx, serviceName)
	}
	return n.registry.ListAllServices(ctx)
}

func (n *Notifier) collectService(ctx context.Context, serviceName string, instances []eru_models.ServiceInstance, reload bool) ServiceStatus {
	statuses := make([]InstanceStatus, len(instances))

	sem := make(chan struct{}, n.concurrency)
	var wg sync.WaitGroup
	for i, instance := range instances {
		wg.Add(1)
		go func(idx int, inst eru_models.ServiceInstance) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			statuses[idx] = n.collectInstance(ctx, inst, reload)
		}(i, instance)
	}
	wg.Wait()

	sort.Slice(statuses, func(i, j int) bool { return statuses[i].InstanceId < statuses[j].InstanceId })

	return ServiceStatus{
		Service:   serviceName,
		InSync:    inSync(statuses),
		Instances: statuses,
	}
}

func (n *Notifier) collectInstance(ctx context.Context, instance eru_models.ServiceInstance, reload bool) InstanceStatus {
	status := InstanceStatus{InstanceId: instance.Id, Address: instance.Address}
	base := strings.TrimRight(instance.Address, "/")

	if reload {
		reloaded := false
		if err := n.load(ctx, base+loadPath); err != nil {
			status.Error = err.Error()
			status.Reloaded = &reloaded
			return status
		}
		reloaded = true
		status.Reloaded = &reloaded
	}

	updatedAt, err := n.configStatus(ctx, base+statusPath)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.ConfigUpdateAt = &updatedAt
	return status
}

func (n *Notifier) configStatus(ctx context.Context, url string) (updatedAt time.Time, err error) {
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
		return updatedAt, fmt.Errorf("%s returned status %d", url, response.StatusCode)
	}
	var parsed configStatusResponse
	if err = json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("could not decode config status from ", url, " : ", err.Error()))
		return
	}
	return parsed.ConfigUpdateAt, nil
}

func inSync(statuses []InstanceStatus) bool {
	var reference *time.Time
	for _, status := range statuses {
		if status.Error != "" || status.ConfigUpdateAt == nil {
			return false
		}
		if reference == nil {
			reference = status.ConfigUpdateAt
			continue
		}
		if !reference.Equal(*status.ConfigUpdateAt) {
			return false
		}
	}
	return true
}
