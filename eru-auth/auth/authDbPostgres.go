package auth

import (
	"context"
	"fmt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"strings"
)

type AuthDbPostgres struct {
	AuthDb
}

func (authDbPostgres *AuthDbPostgres) GetDbQuery(ctx context.Context, query string) (finalQuery string) {
	placeHolderCount := strings.Count(query, "???")

	for i := 0; i <= placeHolderCount; i++ {
		query = strings.Replace(query, "???", fmt.Sprint("$", i+1), 1)
	}
	logs.WithContext(ctx).Info(query)
	return query
}
