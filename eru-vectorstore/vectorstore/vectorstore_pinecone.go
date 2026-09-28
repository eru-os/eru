package vectorstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	utils "github.com/eru-os/eru/eru-utils"
)

const baseUrl = "https://api.pinecone.io"
const pinecone_api_version = "2025-04"

type PineconeVectorStore struct {
	VectorStore
	APIKey string              `json:"api_key" eru:"required"`
	Index  PineconeVectorIndex `json:"index"`
}
type PineconeVectorIndex struct {
	Dimension int    `json:"dimension"`
	Metric    string `json:"metric"`
	Status    struct {
		Ready bool   `json:"ready"`
		State string `json:"state"`
	} `json:"status"`
	Host           string `json:"host"`
	ServerlessSpec struct {
		Region           string `json:"region"`
		Cloud            string `json:"cloud"`
		SourceCollection string `json:"source_collection"`
	} `json:"serverless_spec"`
	PodSpec struct {
		Environment      string                 `json:"environment"`
		PodType          string                 `json:"pod_type"`
		Replicas         int                    `json:"replicas"`
		Shards           int                    `json:"shards"`
		Pods             int                    `json:"pods"`
		MetadataConfig   map[string]interface{} `json:"metadata_config"`
		SourceCollection string                 `json:"source_collection"`
	} `json:"pod_spec"`
	DeletionProtection string            `json:"deletion_protection"`
	Tags               map[string]string `json:"tags"`
	Embed              struct {
		Model           string                 `json:"model"`
		FieldMap        map[string]string      `json:"field_map"`
		Metric          string                 `json:"metric"`
		Dimension       int                    `json:"dimension"`
		ReadParameters  map[string]interface{} `json:"read_parameters"`
		WriteParameters map[string]interface{} `json:"write_parameters"`
		Cloud           string                 `json:"cloud"`
		Region          string                 `json:"region"`
	} `json:"embed"`
}
type PineconeVectorResultHit struct {
	Id     string                 `json:"_id"`
	Values []float64              `json:"values"`
	Score  float64                `json:"_score"`
	Fields map[string]interface{} `json:"fields"`
}
type PineconeVectorResult struct {
	Namespace string `json:"namespace"`
	Result    struct {
		Hits []PineconeVectorResultHit `json:"hits,omitempty"`
	} `json:"result,omitempty"`
	Usage struct {
		ReadUnits        int `json:"read_units"`
		RerankUnits      int `json:"rerank_units"`
		EmbedTotalTokens int `json:"embed_total_tokens"`
	} `json:"usage"`
}
type PineconeVectorMatch struct {
	Namespace string `json:"namespace"`
	Matches   []struct {
		Id           string    `json:"id"`
		Values       []float64 `json:"values"`
		SparceValues struct {
			Indices []int     `json:"indices"`
			Values  []float64 `json:"values"`
		} `json:"sparce_values"`
		Score    float64                `json:"score"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	} `json:"matches,omitempty"`
	Usage struct {
		ReadUnits        int `json:"read_units"`
		RerankUnits      int `json:"rerank_units"`
		EmbedTotalTokens int `json:"embed_total_tokens"`
	} `json:"usage"`
}

func (pvs *PineconeVectorStore) SearchVectors(ctx context.Context, vectorRecordsSearch VectorRecordsSearch) (VectorResults, error) {
	logs.WithContext(ctx).Debug("PineconeVectorStore Search - Start")
	if pvs.Index.Host == "" {
		err := pvs.SyncIndexDefinition(ctx, pvs)
		if err != nil {
			return VectorResults{}, err
		}
		if pvs.Index.Host == "" {
			return VectorResults{}, logs.Err(ctx, fmt.Errorf("index host is empty"), "")
		}
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Api-Key", pvs.APIKey)
	headers.Set("X-Pinecone-API-Version", pinecone_api_version)
	url := fmt.Sprintf("https://%s/query", pvs.Index.Host)
	vectorSearchBody := map[string]interface{}{}
	isQuery := false
	if vectorRecordsSearch.Inputs != nil {
		isQuery = true

		headers.Set("Accept", "application/json")

		url = fmt.Sprintf("https://%s/records/namespaces/%s/search", pvs.Index.Host, vectorRecordsSearch.Namespace)
		vectorSearchBodyQuery := make(map[string]interface{})
		vectorSearchBodyQuery["inputs"] = vectorRecordsSearch.Inputs
		vectorSearchBodyQuery["top_k"] = vectorRecordsSearch.TopK
		vectorSearchBodyQuery["filter"] = vectorRecordsSearch.Filter
		//vectorSearchBodyQueryVector := make(map[string]interface{})
		//vectorSearchBodyQueryVector["values"] = vectorRecordsSearch.Vector
		//vectorSearchBodyQueryVector["sparce_values"] = vectorRecordsSearch.SparceVector
		//vectorSearchBodyQueryVector["sparse_indices"] = vectorRecordsSearch.SparceVector.Indices
		//vectorSearchBodyQuery["vector"] = vectorSearchBodyQueryVector
		//vectorSearchBodyQuery["id"] = vectorRecordsSearch.Id
		vectorSearchBody["query"] = vectorSearchBodyQuery
		vectorSearchBody["fields"] = vectorRecordsSearch.Fields
		if vectorRecordsSearch.ReRank.Model != "" {
			vectorSearchBodyReRank := map[string]interface{}{
				"model":       vectorRecordsSearch.ReRank.Model,
				"rank_fields": vectorRecordsSearch.ReRank.RankFields,
			}
			if vectorRecordsSearch.ReRank.TopN > 0 {
				vectorSearchBodyReRank["top_n"] = vectorRecordsSearch.ReRank.TopN
			}
			if vectorRecordsSearch.ReRank.Parameters != nil {
				vectorSearchBodyReRank["parameters"] = vectorRecordsSearch.ReRank.Parameters
			}
			if vectorRecordsSearch.ReRank.Query != "" {
				vectorSearchBodyReRank["query"] = vectorRecordsSearch.ReRank.Query
			}
			vectorSearchBody["rerank"] = vectorSearchBodyReRank
		}
	} else {

		vectorSearchBody["topK"] = vectorRecordsSearch.TopK
		if vectorRecordsSearch.Id != "" {
			vectorSearchBody["id"] = vectorRecordsSearch.Id
		}
		if vectorRecordsSearch.Namespace != "" {
			vectorSearchBody["namespace"] = vectorRecordsSearch.Namespace
		}
		if vectorRecordsSearch.Filter != nil {
			vectorSearchBody["filter"] = vectorRecordsSearch.Filter
		}
		vectorSearchBody["includeValues"] = vectorRecordsSearch.ReturnValues
		vectorSearchBody["includeMetadata"] = vectorRecordsSearch.ReturnMetadata
		if len(vectorRecordsSearch.Vector) > 0 {
			vectorSearchBody["vector"] = vectorRecordsSearch.Vector
		}
		if len(vectorRecordsSearch.SparceVector.Values) > 0 {
			vectorSearchBody["sparceValues"] = vectorRecordsSearch.SparceVector
		}
	}
	resp, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, nil, nil, nil, vectorSearchBody)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return VectorResults{}, err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(resp))
	var records []VectorResult
	var vectorResults VectorResults
	if isQuery {
		// Parse response into PineconeVectorResult
		var pineconeResult PineconeVectorResult
		resultBytes, err := json.Marshal(resp)
		if err != nil {
			err = logs.Err(ctx, err, "Failed to marshal response")
			return VectorResults{}, err
		}

		if err := json.Unmarshal(resultBytes, &pineconeResult); err != nil {
			err = logs.Err(ctx, err, "Failed to unmarshal response into PineconeVectorResult")
			return VectorResults{}, err
		}

		for _, hit := range pineconeResult.Result.Hits {
			metadata := make(map[string]interface{})
			metadata["score"] = hit.Score
			metadata["fields"] = hit.Fields
			record := VectorResult{
				Id:       hit.Id,
				Values:   hit.Values,
				Metadata: metadata,
			}
			records = append(records, record)
		}
		usage := map[string]string{}
		usage["read_units"] = fmt.Sprintf("%d", pineconeResult.Usage.ReadUnits)
		usage["rerank_units"] = fmt.Sprintf("%d", pineconeResult.Usage.RerankUnits)
		usage["embed_total_tokens"] = fmt.Sprintf("%d", pineconeResult.Usage.EmbedTotalTokens)
		vectorResults = VectorResults{
			Records: records,
			Usage:   usage,
		}
	} else {
		var pineconeMatch PineconeVectorMatch
		resultBytes, err := json.Marshal(resp)
		if err != nil {
			err = logs.Err(ctx, err, "Failed to marshal response")
			return VectorResults{}, err
		}

		if err := json.Unmarshal(resultBytes, &pineconeMatch); err != nil {
			err = logs.Err(ctx, err, "Failed to unmarshal response into PineconeVectorResult")
			return VectorResults{}, err
		}

		// Convert PineconeVectorResult to []VectorResult
		for _, match := range pineconeMatch.Matches {
			match.Metadata["score"] = match.Score
			record := VectorResult{
				Id:       match.Id,
				Values:   match.Values,
				Metadata: match.Metadata,
			}
			records = append(records, record)
		}
		usage := map[string]string{}
		usage["read_units"] = fmt.Sprintf("%d", pineconeMatch.Usage.ReadUnits)
		usage["rerank_units"] = fmt.Sprintf("%d", pineconeMatch.Usage.RerankUnits)
		usage["embed_total_tokens"] = fmt.Sprintf("%d", pineconeMatch.Usage.EmbedTotalTokens)
		vectorResults = VectorResults{
			Records: records,
			Usage:   usage,
		}
	}
	return vectorResults, nil
}

func (pvs *PineconeVectorStore) SaveVectors(ctx context.Context, vectorRecords VectorRecords) error {
	logs.WithContext(ctx).Debug("PineconeVectorStore SaveVectors - Start")
	if pvs.Index.Host == "" {
		err := pvs.SyncIndexDefinition(ctx, pvs)
		if err != nil {
			return err
		}
		if pvs.Index.Host == "" {
			return logs.Err(ctx, fmt.Errorf("index host is empty"), "")
		}
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Api-Key", pvs.APIKey)
	headers.Set("X-Pinecone-API-Version", pinecone_api_version)
	url := fmt.Sprintf("https://%s/records/namespaces/%s/upsert", pvs.Index.Host, vectorRecords.Namespace)
	var vectorSaveBody interface{}
	useEmbed := true
	if len(vectorRecords.Vectors[0].Values) > 0 || len(vectorRecords.Vectors[0].SparceValues.Values) > 0 {
		// if vectors (dense or sparse) are provided, don't use embed
		useEmbed = false
	}

	if useEmbed {
		if pvs.Index.Embed.Model == "" {
			return logs.Err(ctx, fmt.Errorf("embed model is empty"), "")
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		// Optional: stable key order for deterministic output
		enc.SetEscapeHTML(false)
		for _, vector := range vectorRecords.Vectors {
			vectorMap := map[string]interface{}{}
			vectorMap["_id"] = vector.Id
			for k, v := range vector.Metadata {
				vectorMap[k] = v
			}
			// writes one object + '\n'
			if err := enc.Encode(vectorMap); err != nil {
				return logs.Err(ctx, fmt.Errorf("encode record: %w", err), "")
			}
		}
		vectorSaveBody = buf.Bytes()
		headers.Set("Content-Type", "application/x-ndjson")
	} else {
		if pvs.Index.Embed.Model != "" {
			return logs.Err(ctx, fmt.Errorf("expected text but recevied vectors for embed model"), "")
		}
		url = fmt.Sprintf("https://%s/vectors/upsert", pvs.Index.Host)
		vectors := []map[string]interface{}{}
		for _, vector := range vectorRecords.Vectors {
			vectors = append(vectors, map[string]interface{}{
				"id":       vector.Id,
				"values":   vector.Values,
				"metadata": vector.Metadata,
			})
		}
		vectorSaveBody = map[string]interface{}{
			"vectors":   vectors,
			"namespace": vectorRecords.Namespace,
		}
	}
	// Use Pinecone API endpoint directly

	resp, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, nil, nil, nil, vectorSaveBody)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(resp))
	return nil
}

func (pvs *PineconeVectorStore) DeleteVectors(ctx context.Context, vectorRecordsDelete VectorRecordsDelete) error {
	logs.WithContext(ctx).Debug("PineconeVectorStore Delete - Start")
	if pvs.Index.Host == "" {
		err := pvs.SyncIndexDefinition(ctx, pvs)
		if err != nil {
			return err
		}
		if pvs.Index.Host == "" {
			return logs.Err(ctx, fmt.Errorf("index host is empty"), "")
		}
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Api-Key", pvs.APIKey)
	headers.Set("X-Pinecone-API-Version", pinecone_api_version)
	url := fmt.Sprintf("https://%s/vectors/delete", pvs.Index.Host)
	vectorDeleteBody := map[string]interface{}{}
	if vectorRecordsDelete.DeleteAll {
		vectorDeleteBody["deleteAll"] = true
	}
	if len(vectorRecordsDelete.Ids) > 0 {
		vectorDeleteBody["ids"] = vectorRecordsDelete.Ids
	}
	if vectorRecordsDelete.Filter != nil {
		vectorDeleteBody["filter"] = vectorRecordsDelete.Filter
	}
	if vectorRecordsDelete.Namespace != "" {
		vectorDeleteBody["namespace"] = vectorRecordsDelete.Namespace
	}
	resp, _, _, _, err := utils.CallHttp(ctx, http.MethodDelete, url, headers, nil, nil, nil, vectorDeleteBody)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(resp))
	return nil
}

func (pvs *PineconeVectorStore) CreateIndex(ctx context.Context, cloneVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Debug("CreateIndex - Start")

	// Create index using Pinecone API directly
	createIndexPayload := map[string]interface{}{
		"name":                pvs.VectorName,
		"deletion_protection": pvs.Index.DeletionProtection,
		"tags":                pvs.Index.Tags,
	}
	embedUrl := ""
	if pvs.Index.Embed.Model != "" {
		embedUrl = "/create-for-model"
		createIndexPayload["embed"] = map[string]interface{}{
			"model":            pvs.Index.Embed.Model,
			"field_map":        pvs.Index.Embed.FieldMap,
			"metric":           pvs.Index.Embed.Metric,
			"dimension":        pvs.Index.Embed.Dimension,
			"read_parameters":  pvs.Index.Embed.ReadParameters,
			"write_parameters": pvs.Index.Embed.WriteParameters,
		}
		createIndexPayload["cloud"] = pvs.Index.Embed.Cloud
		createIndexPayload["region"] = pvs.Index.Embed.Region
		/* createIndexPayload["spec"] = map[string]interface{}{
			"serverless": map[string]interface{}{
				"cloud":  pvs.Index.Embed.Cloud,
				"region": pvs.Index.Embed.Region,
			},
		} */
	} else if pvs.Index.ServerlessSpec.Cloud != "" {
		createIndexPayload["spec"] = map[string]interface{}{
			"serverless": map[string]interface{}{
				"cloud":  pvs.Index.ServerlessSpec.Cloud,
				"region": pvs.Index.ServerlessSpec.Region,
			},
		}
		createIndexPayload["dimension"] = pvs.Index.Dimension
		createIndexPayload["metric"] = pvs.Index.Metric
	} else if pvs.Index.PodSpec.PodType != "" {
		createIndexPayload["spec"] = map[string]interface{}{
			"pod": map[string]interface{}{
				"pod_type":        pvs.Index.PodSpec.PodType,
				"replicas":        pvs.Index.PodSpec.Replicas,
				"shards":          pvs.Index.PodSpec.Shards,
				"pods":            pvs.Index.PodSpec.Pods,
				"environment":     pvs.Index.PodSpec.Environment,
				"metadata_config": pvs.Index.PodSpec.MetadataConfig,
			},
		}
	}

	// Use Pinecone API endpoint directly
	url := fmt.Sprintf("%s/indexes%s", baseUrl, embedUrl)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Api-Key", cloneVectorStore.GetAttribute(ctx, "api_key"))
	headers.Set("X-Pinecone-API-Version", pinecone_api_version)

	resp, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, nil, nil, nil, createIndexPayload)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return err
	}
	respMap, ok := resp.(map[string]interface{})
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("resp is not a map"), "")
		return err
	}

	if host, hostOk := respMap["host"].(string); hostOk {
		pvs.Index.Host = host
	}
	if status, statusOk := respMap["status"]; statusOk {
		if statusMap, statusMapOk := status.(map[string]interface{}); statusMapOk {
			if ready, readyOk := statusMap["ready"].(bool); readyOk {
				pvs.Index.Status.Ready = ready
			}
			if state, stateOk := statusMap["state"].(string); stateOk {
				pvs.Index.Status.State = state
			}
		}
	}
	return nil
}

func (pvs *PineconeVectorStore) DeleteIndex(ctx context.Context, indexName string) error {
	logs.WithContext(ctx).Debug("PineconeVectorStore DeleteIndex - Start")

	// Delete index using Pinecone API directly
	url := fmt.Sprintf("%s/indexes/%s", baseUrl, indexName)
	headers := http.Header{}
	headers.Set("Api-Key", pvs.APIKey)
	headers.Set("X-Pinecone-API-Version", pinecone_api_version)

	_, _, _, _, err := utils.CallHttp(ctx, "DELETE", url, headers, nil, nil, nil, nil)
	if err != nil {
		if !strings.Contains(err.Error(), "NOT_FOUND") {
			err = logs.Err(ctx, err, "")
			return err
		} else {
			err = nil
		}
	}
	return nil
}

func (pvs *PineconeVectorStore) GetStats(ctx context.Context) (VectorStats, error) {
	logs.WithContext(ctx).Debug("PineconeVectorStore GetStats - Start")

	return VectorStats{
		//IndexName: pvs.IndexName,
	}, nil
}
func (pvs *PineconeVectorStore) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &pvs)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return err
	}
	return nil
}

func (pvs *PineconeVectorStore) EditIndex(ctx context.Context, cloneVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Debug("PineconeVectorStore Edit - Start")

	// Create index using Pinecone API directly
	editIndexPayload := map[string]interface{}{
		"deletion_protection": pvs.Index.DeletionProtection,
		"tags":                pvs.Index.Tags,
	}
	if pvs.Index.PodSpec.PodType != "" {
		editIndexPayload["spec"] = map[string]interface{}{
			"pod": map[string]interface{}{
				"pod_type":        pvs.Index.PodSpec.PodType,
				"replicas":        pvs.Index.PodSpec.Replicas,
				"shards":          pvs.Index.PodSpec.Shards,
				"pods":            pvs.Index.PodSpec.Pods,
				"environment":     pvs.Index.PodSpec.Environment,
				"metadata_config": pvs.Index.PodSpec.MetadataConfig,
			},
		}
	}

	// Use Pinecone API endpoint directly
	url := fmt.Sprintf("%s/indexes/%s", baseUrl, pvs.VectorName)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Api-Key", cloneVectorStore.GetAttribute(ctx, "api_key"))
	headers.Set("X-Pinecone-API-Version", pinecone_api_version)

	_, _, _, _, err := utils.CallHttp(ctx, http.MethodPatch, url, headers, nil, nil, nil, editIndexPayload)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return err
	}

	logs.WithContext(ctx).Info("PineconeVectorStore edit completed")
	return nil
}
func (pvs *PineconeVectorStore) UpdateVectorStore(ctx context.Context, updatedVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Debug("PineconeVectorStore Edit - Start")
	updatedPineconeVectorStore, ok := updatedVectorStore.(*PineconeVectorStore)
	if !ok {
		return logs.Err(ctx, fmt.Errorf("invalid vector store type"), "")
	}
	pvs.Index.DeletionProtection = updatedPineconeVectorStore.Index.DeletionProtection
	pvs.Index.Tags = updatedPineconeVectorStore.Index.Tags
	pvs.APIKey = updatedPineconeVectorStore.APIKey
	return nil
}
func (pvs *PineconeVectorStore) GetBytes(ctx context.Context) ([]byte, error) {
	logs.WithContext(ctx).Debug("PineconeVectorStore GetBytes - Start")
	vectorStoreJson, err := json.Marshal(pvs)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return vectorStoreJson, nil
}
func (pvs *PineconeVectorStore) BytesToVectorStore(ctx context.Context, vectorStoreObjJson []byte) (VectorStoreI, error) {
	logs.WithContext(ctx).Debug("PineconeVectorStore BytesToVectorStore - Start")
	iCloneI := reflect.New(reflect.TypeOf(pvs))
	vectorStoreObjCloneErr := json.Unmarshal(vectorStoreObjJson, iCloneI.Interface())
	if vectorStoreObjCloneErr != nil {
		err := logs.Err(ctx, vectorStoreObjCloneErr, "error while cloning vectorStoreObj(unmarshal)")
		return nil, err
	}
	return iCloneI.Elem().Interface().(VectorStoreI), nil
}
func (pvs *PineconeVectorStore) SyncIndexDefinition(ctx context.Context, cloneVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Debug("PineconeVectorStore SyncIndexDefinition - Start")

	// Use Pinecone API endpoint directly
	url := fmt.Sprintf("%s/indexes/%s", baseUrl, pvs.VectorName)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Api-Key", cloneVectorStore.GetAttribute(ctx, "api_key"))
	headers.Set("X-Pinecone-API-Version", pinecone_api_version)

	resp, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, nil, nil, nil, nil)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return err
	}

	respMap, ok := resp.(map[string]interface{})
	if !ok {
		err = logs.Err(ctx, fmt.Errorf("resp is not a map"), "")
		return err
	}

	if host, hostOk := respMap["host"].(string); hostOk {
		pvs.Index.Host = host
	}
	if status, statusOk := respMap["status"]; statusOk {
		if statusMap, statusMapOk := status.(map[string]interface{}); statusMapOk {
			if ready, readyOk := statusMap["ready"].(bool); readyOk {
				pvs.Index.Status.Ready = ready
			}
			if state, stateOk := statusMap["state"].(string); stateOk {
				pvs.Index.Status.State = state
			}
		}
	}
	logs.WithContext(ctx).Info("PineconeVectorStore edit completed")
	return nil
}
func (pvs *PineconeVectorStore) GetAttribute(ctx context.Context, attributeName string) string {
	switch attributeName {
	case "vector_name":
		return pvs.VectorName
	case "vector_type":
		return pvs.VectorType
	case "api_key":
		return pvs.APIKey
	default:
		return ""
	}
}
