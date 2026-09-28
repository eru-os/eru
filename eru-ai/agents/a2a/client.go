package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

type RemoteClient struct {
	BaseURL string
	Client  *http.Client
}

func NewRemoteClient(baseURL string) *RemoteClient {
	return &RemoteClient{BaseURL: baseURL, Client: http.DefaultClient}
}

func (c *RemoteClient) Submit(ctx context.Context, goal string, plan map[string]interface{}, contextObj map[string]interface{}) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"goal":    goal,
		"plan":    plan,
		"context": contextObj,
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/a2a/task.submit", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out, nil
}
