package auth

import (
	"context"
	"strings"
)

type AuthDbMysql struct {
	AuthDb
}

func (authDbMysql *AuthDbMysql) GetDbQuery(ctx context.Context, query string) (finalQuery string) {
	return strings.Replace(query, "???", "?", -1)
}
