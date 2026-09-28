package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eru-os/eru/eru-cache/cache"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
)

const (
	// ServiceKeyPrefix is the prefix for all service instance keys in the cache.
	ServiceKeyPrefix = "service-registry:"
)

// Registry is a client for the service registry stored in a cache.
type Registry struct {
	cache        cache.CacheStoreI
	heartbeatTTL time.Duration
}

// NewRegistry creates a new service registry client.
func NewRegistry(cache cache.CacheStoreI, ttl time.Duration) *Registry {
	return &Registry{
		cache:        cache,
		heartbeatTTL: ttl,
	}
}

// generateKey creates the standard key format for a service instance.
func generateKey(serviceID string) string {
	return fmt.Sprintf("%s%s", ServiceKeyPrefix, serviceID)
}

// Register adds a new service instance to the registry with a TTL.
// AliveUntil is stamped here rather than trusted from the caller so that
// every write path - register, re-register and heartbeat - records the same
// thing: the instant this instance stops counting as alive.
func (r *Registry) Register(ctx context.Context, instance eru_models.ServiceInstance) error {
	key := generateKey(instance.Id)
	instance.AliveUntil = time.Now().Add(r.heartbeatTTL)
	return r.cache.SetWithTTL(ctx, key, instance, r.heartbeatTTL)
}

// Deregister removes a service instance from the registry.
func (r *Registry) Deregister(ctx context.Context, serviceId string) error {
	key := generateKey(serviceId)
	if _, err := r.cache.Get(ctx, key); err != nil {
		err = fmt.Errorf("service with id %s not found", serviceId)
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return r.cache.Delete(ctx, key)
}

// Heartbeat refreshes the TTL for a service instance.
func (r *Registry) Heartbeat(ctx context.Context, serviceID string) error {
	key := generateKey(serviceID)
	val, err := r.cache.Get(ctx, key)
	if err != nil {
		err = fmt.Errorf("service with Id %s not found, please re-register", serviceID)
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	var instance eru_models.ServiceInstance
	if err := json.Unmarshal([]byte(val), &instance); err != nil {
		err = fmt.Errorf("failed to unmarshal service instance data: %w", err)
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	instance.AliveUntil = time.Now().Add(r.heartbeatTTL)
	// Set it again to refresh the TTL
	return r.cache.SetWithTTL(ctx, key, instance, r.heartbeatTTL)
}

// ListServices returns all instances for a given service name.
// Note: This is less efficient as it requires fetching all keys.
func (r *Registry) ListServices(ctx context.Context, serviceName string) ([]eru_models.ServiceInstance, error) {
	pattern := fmt.Sprintf("%s*", ServiceKeyPrefix)
	keys, err := r.cache.GetKeys(ctx, pattern)
	if err != nil {
		return nil, err
	}

	var instances []eru_models.ServiceInstance
	instances = []eru_models.ServiceInstance{}
	for _, key := range keys {
		val, err := r.cache.Get(ctx, key)
		if err != nil {
			// Key might have expired between GetKeys and Get, so just log and continue
			logs.WithContext(ctx).Warn(fmt.Sprintf("Could not get key %s: %v", key, err))
			continue
		}
		var instance eru_models.ServiceInstance
		if err := json.Unmarshal([]byte(val), &instance); err != nil {
			logs.WithContext(ctx).Warn(fmt.Sprintf("Could not unmarshal service data for key %s: %v", key, err))
			continue
		}
		if instance.Name == serviceName {
			instances = append(instances, instance)
		}
	}
	return instances, nil
}

// ListAllServices returns all registered instances.
func (r *Registry) ListAllServices(ctx context.Context) ([]eru_models.ServiceInstance, error) {
	pattern := fmt.Sprintf("%s*", ServiceKeyPrefix)
	keys, err := r.cache.GetKeys(ctx, pattern)
	if err != nil {
		return nil, err
	}

	var instances []eru_models.ServiceInstance
	instances = []eru_models.ServiceInstance{}
	for _, key := range keys {
		val, err := r.cache.Get(ctx, key)
		if err != nil {
			logs.WithContext(ctx).Warn(fmt.Sprintf("Could not get key %s: %v", key, err))
			continue
		}
		var instance eru_models.ServiceInstance
		if err := json.Unmarshal([]byte(val), &instance); err != nil {
			logs.WithContext(ctx).Warn(fmt.Sprintf("Could not unmarshal service data for key %s: %v", key, err))
			continue
		}
		instances = append(instances, instance)
	}
	return instances, nil
}
