package eru_models

import "time"

type Queries struct {
	Query string
	Vals  []interface{}
	Rank  int
}

type QueriesSorter []*Queries

func (a QueriesSorter) Len() int {
	return len(a)
}
func (a QueriesSorter) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a QueriesSorter) Less(i, j int) bool {
	return a[i].Rank < a[j].Rank
}

// JSONSchema represents a JSON schema definition
type JSONSchema struct {
	Type                 string                `json:"type"`
	Properties           map[string]JSONSchema `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Items                *JSONSchema           `json:"items,omitempty"`  // For arrays
	Enum                 []interface{}         `json:"enum,omitempty"`   // For enums
	Format               string                `json:"format,omitempty"` // For strings
	Description          string                `json:"description,omitempty"`
	AdditionalProperties interface{}           `json:"additionalProperties,omitempty"`
}
type SampleRequest struct {
	RequestId    string                 `json:"request_id" eru:"required"`
	RequestName  string                 `json:"request_name" eru:"required"`
	RequestBody  map[string]interface{} `json:"request_body" eru:"required"`
	ResourceName string                 `json:"resource_name" eru:"required"`
}

// ServiceInstance represents the data sent to the registry.
type ServiceInstance struct {
	Id         string    `json:"id"`
	Name       string    `json:"name"`
	Address    string    `json:"address"`
	AliveUntil time.Time `json:"alive_until"`
}
