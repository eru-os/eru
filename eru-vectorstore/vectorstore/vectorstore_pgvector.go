package vectorstore

import (
	"context"
	"encoding/json"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/jmoiron/sqlx"
)

type PGVectorStore struct {
	VectorStore
	DB       *sqlx.DB
	Host     string
	Port     string
	Database string
	User     string
	Password string
	Indexes  []PGVectorIndex
}

type PGVectorIndex struct {
	Name string `json:"name"`
}

func (pgvs *PGVectorStore) SearchVectors(ctx context.Context, vectorRecordsSearch VectorRecordsSearch) (VectorResults, error) {
	logs.WithContext(ctx).Info("Searching vectors in PGVector")

	return VectorResults{
		Records: []VectorResult{},
		Usage:   map[string]string{},
	}, nil
}

func (pgvs *PGVectorStore) SaveVectors(ctx context.Context, vectorRecords VectorRecords) error {
	logs.WithContext(ctx).Info("Saving vectors to PGVector")

	return nil
}

func (pgvs *PGVectorStore) DeleteVectors(ctx context.Context, vectorRecordsDelete VectorRecordsDelete) error {
	logs.WithContext(ctx).Info("Deleting vectors from PGVector")

	return nil
}

func (pgvs *PGVectorStore) CreateIndex(ctx context.Context, cloneVectorStore VectorStoreI) error {
	logs.WithContext(ctx).Info("Creating PGVector index")
	//pgvs.IndexName = indexName

	return nil
}

func (pgvs *PGVectorStore) DeleteIndex(ctx context.Context, indexName string) error {
	logs.WithContext(ctx).Info("Deleting PGVector index")

	return nil
}

func (pgvs *PGVectorStore) GetStats(ctx context.Context) (VectorStats, error) {
	logs.WithContext(ctx).Info("Getting PGVector stats")

	return VectorStats{
		//IndexName: pgvs.IndexName,
	}, nil
}
func (pgvs *PGVectorStore) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &pgvs)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
