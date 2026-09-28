package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	chroma "github.com/amikos-tech/chroma-go"
	"github.com/amikos-tech/chroma-go/types"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type ChromaDBVectorStore struct {
	VectorStore
	Host       string             `json:"host" eru:"required"` // ChromaDB server host
	Port       int                `json:"port" eru:"required"` // ChromaDB server port
	Collection ChromaDBCollection `json:"collection"`          // Collection configuration
	Tenant     string             `json:"tenant,omitempty"`    // ChromaDB tenant (optional)
	Database   string             `json:"database,omitempty"`  // ChromaDB database (optional)
	client     *chroma.Client     `json:"-"`                   // ChromaDB client (not serialized)
}

type ChromaDBCollection struct {
	Name              string                 `json:"name"`                         // Collection name
	Metadata          map[string]interface{} `json:"metadata,omitempty"`           // Collection metadata
	EmbeddingFunction string                 `json:"embedding_function,omitempty"` // Embedding function name
	Distance          string                 `json:"distance,omitempty"`           // Distance function (l2, ip, cosine)
}

func (cvs *ChromaDBVectorStore) initClient(ctx context.Context) error {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore initClient - Start")

	if cvs.client != nil {
		return nil // Client already initialized
	}

	// Set default values
	if cvs.Tenant == "" {
		cvs.Tenant = "default"
	}
	if cvs.Database == "" {
		cvs.Database = "default"
	}

	// Create ChromaDB client with basic configuration
	url := fmt.Sprintf("http://%s:%d", cvs.Host, cvs.Port)
	client, err := chroma.NewClient(url)
	if err != nil {
		return logs.Err(ctx, err, "Failed to create ChromaDB client")
	}

	cvs.client = client
	logs.WithContext(ctx).Info("ChromaDB client initialized successfully")
	return nil
}

func (cvs *ChromaDBVectorStore) SearchVectors(ctx context.Context, vectorRecordsSearch VectorRecordsSearch) (VectorResults, error) {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore Search - Start")

	/* if err := cvs.initClient(ctx); err != nil {
		return nil, logs.Err(ctx, err, "Failed to initialize ChromaDB client")
	}

	// Get collection
	col, err := cvs.client.GetCollection(ctx, cvs.Collection.Name, nil)
	if err != nil {
		return nil, logs.Err(ctx, err, "Failed to get collection")
	}

	// For now, implement a basic search using Get with filters (simplified approach)
	// The ChromaDB Go client API is evolving, so this uses a minimal working approach
	getResult, err := col.Get(ctx, filter, nil, nil, nil)
	if err != nil {
		return nil, logs.Err(ctx, err, "Failed to search collection")
	}

	// Convert results to VectorResult format
	var results []VectorResult
	if getResult.Documents != nil {
		for i, docList := range getResult.Documents {
			for j, doc := range docList {
				result := VectorResult{
					Vector: Vector{
						Id: string(doc), // Document content as name
					},
					Values: make([]float64, 0), // ChromaDB doesn't return embeddings by default
					Score:  1.0,                // Default score for Get operation
				}

				// Add metadata if available - simplified approach
				if getResult.Metadatas != nil && i < len(getResult.Metadatas) && len(getResult.Metadatas[i]) > j {
					// ChromaDB metadata handling - simplified for compatibility
					result.Vector.Metadata = make(map[string]interface{})
					result.Vector.Metadata["doc_index"] = fmt.Sprintf("%d_%d", i, j)
				}

				results = append(results, result)

				// Limit results to topK
				if len(results) >= topK {
					break
				}
			}
			if len(results) >= topK {
				break
			}
		}
	} */

	//logs.WithContext(ctx).Info(fmt.Sprintf("ChromaDB search returned %d results", len(results)))
	return VectorResults{
		Records: []VectorResult{},
		Usage:   map[string]string{},
	}, nil
}

func (cvs *ChromaDBVectorStore) SaveVectors(ctx context.Context, vectorRecords VectorRecords) error {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore SaveVectors - Start")

	/* if err := cvs.initClient(ctx); err != nil {
		return logs.Err(ctx, err, "Failed to initialize ChromaDB client")
	}

	// Get collection
	col, err := cvs.client.GetCollection(ctx, cvs.Collection.Name, nil)
	if err != nil {
		return logs.Err(ctx, err, "Failed to get collection")
	}

	// Prepare data for insertion
	var ids []string
	var documents []string
	var metadatas []map[string]interface{}

	for i, vector := range vectors {
		// Generate ID if not present in metadata
		id := strconv.Itoa(i)
		if vector.Metadata != nil {
			if idVal, exists := vector.Metadata["id"]; exists {
				if idStr, ok := idVal.(string); ok {
					id = idStr
				}
			}
		}

		ids = append(ids, id)
		documents = append(documents, vector.Name) // Use Name as document content
		metadatas = append(metadatas, vector.Metadata)
	}

	// Insert documents (ChromaDB Add expects documents without embeddings for auto-embedding)
	_, err = col.Add(ctx, nil, metadatas, documents, ids)
	if err != nil {
		return logs.Err(ctx, err, "Failed to insert vectors")
	} */

	//logs.WithContext(ctx).Info(fmt.Sprintf("Successfully inserted %d vectors", len(vectors)))
	return nil
}

func (cvs *ChromaDBVectorStore) Update(ctx context.Context, vectors []Vector) error {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore Update - Start")

	if err := cvs.initClient(ctx); err != nil {
		return logs.Err(ctx, err, "Failed to initialize ChromaDB client")
	}

	// Get collection
	col, err := cvs.client.GetCollection(ctx, cvs.Collection.Name, nil)
	if err != nil {
		return logs.Err(ctx, err, "Failed to get collection")
	}

	// Update individual documents
	for i, vector := range vectors {
		// Get ID from metadata
		id := strconv.Itoa(i)
		if vector.Metadata != nil {
			if idVal, exists := vector.Metadata["id"]; exists {
				if idStr, ok := idVal.(string); ok {
					id = idStr
				}
			}
		}

		// Update single document metadata only
		_, err := col.Update(ctx, id, &vector.Metadata)
		if err != nil {
			return logs.Err(ctx, err, fmt.Sprintf("Failed to update vector %s", id))
		}
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Successfully updated %d vectors", len(vectors)))
	return nil
}

func (cvs *ChromaDBVectorStore) DeleteVectors(ctx context.Context, vectorRecordsDelete VectorRecordsDelete) error {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore Delete - Start")

	/* if err := cvs.initClient(ctx); err != nil {
		return logs.Err(ctx, err, "Failed to initialize ChromaDB client")
	}

	// Get collection
	col, err := cvs.client.GetCollection(ctx, cvs.Collection.Name, nil)
	if err != nil {
		return logs.Err(ctx, err, "Failed to get collection")
	}

	// Delete documents by IDs
	_, err = col.Delete(ctx, ids, nil, nil)
	if err != nil {
		return logs.Err(ctx, err, "Failed to delete vectors")
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Successfully deleted %d vectors", len(ids))) */
	return nil
}

func (cvs *ChromaDBVectorStore) CreateIndex(ctx context.Context, cloneVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore CreateIndex - Start")

	if err := cvs.initClient(ctx); err != nil {
		return logs.Err(ctx, err, "Failed to initialize ChromaDB client")
	}

	// Prepare collection metadata
	metadata := cvs.Collection.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	// Set distance function if specified
	if cvs.Collection.Distance != "" {
		metadata["hnsw:space"] = cvs.Collection.Distance
	}

	// Create collection - ChromaDB requires all parameters with proper types
	var embeddingFunc types.EmbeddingFunction
	var distanceFunc types.DistanceFunction
	_, err := cvs.client.CreateCollection(ctx, cvs.Collection.Name, metadata, true, embeddingFunc, distanceFunc)
	if err != nil {
		return logs.Err(ctx, err, "Failed to create collection")
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Successfully created collection: %s", cvs.Collection.Name))
	return nil
}

func (cvs *ChromaDBVectorStore) DeleteIndex(ctx context.Context, indexName string) error {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore DeleteIndex - Start")

	if err := cvs.initClient(ctx); err != nil {
		return logs.Err(ctx, err, "Failed to initialize ChromaDB client")
	}

	// Delete collection (ChromaDB uses collections instead of indexes)
	collectionName := indexName
	if collectionName == "" {
		collectionName = cvs.Collection.Name
	}

	_, err := cvs.client.DeleteCollection(ctx, collectionName)
	if err != nil {
		return logs.Err(ctx, err, "Failed to delete collection")
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Successfully deleted collection: %s", collectionName))
	return nil
}

func (cvs *ChromaDBVectorStore) GetStats(ctx context.Context) (VectorStats, error) {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore GetStats - Start")

	if err := cvs.initClient(ctx); err != nil {
		return VectorStats{}, logs.Err(ctx, err, "Failed to initialize ChromaDB client")
	}

	// Get collection
	col, err := cvs.client.GetCollection(ctx, cvs.Collection.Name, nil)
	if err != nil {
		return VectorStats{}, logs.Err(ctx, err, "Failed to get collection")
	}

	// Get collection count
	count, err := col.Count(ctx)
	if err != nil {
		return VectorStats{}, logs.Err(ctx, err, "Failed to get collection count")
	}

	stats := VectorStats{
		TotalVectors: int64(count),
		IndexName:    cvs.Collection.Name,
		Dimension:    0, // ChromaDB doesn't expose dimension easily
	}

	return stats, nil
}

func (cvs *ChromaDBVectorStore) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore MakeFromJson - Start")

	err := json.Unmarshal(*rj, &cvs)
	if err != nil {
		return logs.Err(ctx, err, "Failed to unmarshal ChromaDBVectorStore")
	}

	return nil
}

func (cvs *ChromaDBVectorStore) EditIndex(ctx context.Context, cloneVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore EditIndex - Start")

	// Update configurable fields
	//cvs.Host = updatedChromaVectorStore.Host
	//cvs.Port = updatedChromaVectorStore.Port
	//cvs.Collection.Metadata = updatedChromaVectorStore.Collection.Metadata

	logs.WithContext(ctx).Info("ChromaDBVectorStore edit completed")
	return nil
}

func (cvs *ChromaDBVectorStore) GetBytes(ctx context.Context) ([]byte, error) {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore GetBytes - Start")

	vectorStoreJson, err := json.Marshal(cvs)
	if err != nil {
		return nil, logs.Err(ctx, err, "Failed to marshal ChromaDBVectorStore")
	}

	return vectorStoreJson, nil
}

func (cvs *ChromaDBVectorStore) BytesToVectorStore(ctx context.Context, vectorStoreObjJson []byte) (VectorStoreI, error) {
	logs.WithContext(ctx).Debug("ChromaDBVectorStore BytesToVectorStore - Start")

	iCloneI := reflect.New(reflect.TypeOf(cvs))
	err := json.Unmarshal(vectorStoreObjJson, iCloneI.Interface())
	if err != nil {
		return nil, logs.Err(ctx, err, "Failed to unmarshal ChromaDBVectorStore clone")
	}

	return iCloneI.Elem().Interface().(VectorStoreI), nil
}

func (cvs *ChromaDBVectorStore) GetAttribute(ctx context.Context, attributeName string) string {
	switch attributeName {
	case "vector_name":
		return cvs.VectorName
	case "vector_type":
		return cvs.VectorType
	case "collection_name":
		return cvs.Collection.Name
	case "host":
		return cvs.Host
	case "port":
		return strconv.Itoa(cvs.Port)
	default:
		return ""
	}
}
