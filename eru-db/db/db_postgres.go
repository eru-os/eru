package db

import (
	"context"
	"fmt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"strings"
)

type DbPostgres struct {
	Db
}

func (DbPostgres *DbPostgres) GetDbQuery(ctx context.Context, query string) (finalQuery string) {
	logs.WithContext(ctx).Info("inside GetDbQuery of postgres")
	placeHolderCount := strings.Count(query, "???")

	for i := 0; i <= placeHolderCount; i++ {
		query = strings.Replace(query, "???", fmt.Sprint("$", i+1), 1)
	}
	logs.WithContext(ctx).Info(query)
	return query
}
