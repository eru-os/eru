package registration

import (
	"context"
	"fmt"
	"net/http"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	eru_utils "github.com/eru-os/eru/eru-utils"
)

// RegistryClient handles the communication with the service registry.
type RegistryClient struct {
	RegistryURL string
	Instance    eru_models.ServiceInstance
}

// NewRegistryClient creates a new client for the service registry.
func NewRegistryClient(registryURL, serviceName, port, instanceId string) (*RegistryClient, error) {
	// Automatically detect service address
	serviceAddress, err := eru_utils.GetServiceAddress(context.Background(), port)
	if err != nil {
		return nil, fmt.Errorf("failed to detect service address: %w", err)
	}

	return &RegistryClient{
		RegistryURL: registryURL,
		Instance: eru_models.ServiceInstance{
			Id:      instanceId,
			Name:    serviceName,
			Address: serviceAddress,
		},
	}, nil
}

// Register sends a registration request to the service registry.
func (c *RegistryClient) Register(ctx context.Context) error {
	url := fmt.Sprintf("%s/registry/register", c.RegistryURL)

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	_, _, _, statusCode, err := eru_utils.CallHttp(ctx, http.MethodPost, url, headers, nil, nil, nil, c.Instance)
	if err != nil {
		return fmt.Errorf("failed to send registration request: %w", err)
	}

	if statusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status code: %d", statusCode)
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("successfully registered with registry as %s", c.Instance.Id))
	return nil
}

// Deregister sends a deregistration request to the service registry.
func (c *RegistryClient) Deregister(ctx context.Context) error {
	url := fmt.Sprintf("%s/registry/deregister/%s", c.RegistryURL, c.Instance.Id)

	_, _, _, statusCode, err := eru_utils.CallHttp(ctx, http.MethodDelete, url, nil, nil, nil, nil, nil)
	if err != nil {
		// Don't return error on deregister, just log it
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to send deregistration request: %v", err))
		return nil
	}

	if statusCode != http.StatusOK {
		logs.WithContext(ctx).Error(fmt.Sprintf("Deregistration failed with status code: %d", statusCode))
	} else {
		logs.WithContext(ctx).Info("Successfully deregistered from registry.")
	}
	return nil
}

// StartHeartbeat starts a ticker to send periodic heartbeats.
func (c *RegistryClient) StartHeartbeat(ctx context.Context, interval time.Duration) (err error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if err = c.sendHeartbeat(ctx); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to send initial heartbeat: %v", err))
	}

	for {
		select {
		case <-ticker.C:
			if err = c.sendHeartbeat(ctx); err != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("Failed to send heartbeat, will retry on next tick: %v", err))
			}
		case <-ctx.Done():
			logs.WithContext(ctx).Info("Heartbeat stopped.")
			return nil
		}
	}
}

func (c *RegistryClient) sendHeartbeat(ctx context.Context) (err error) {
	url := fmt.Sprintf("%s/registry/heartbeat/%s", c.RegistryURL, c.Instance.Id)

	_, _, _, statusCode, err := eru_utils.CallHttp(ctx, http.MethodPost, url, nil, nil, nil, nil, nil)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("failed to send heartbeat: %v", err))
		//return err
	}

	if statusCode == http.StatusNotFound {
		// Service was evicted, try to re-register
		logs.WithContext(ctx).Warn("Heartbeat failed (service not found), attempting to re-register...")
		if err := c.Register(ctx); err != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("Failed to re-register after heartbeat failure: %v", err))
			return err
		}
	} else if statusCode != http.StatusOK {
		logs.WithContext(ctx).Error(fmt.Sprintf("Heartbeat failed with status code: %d", statusCode))
		return fmt.Errorf("heartbeat failed with status code: %d", statusCode)
	}
	return nil
}
