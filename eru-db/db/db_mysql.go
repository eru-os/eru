package db

import (
	"context"
	"strings"
)

type DbMysql struct {
	Db
}

func (dbMysql *DbMysql) GetDbQuery(ctx context.Context, query string) (finalQuery string) {
	return strings.Replace(query, "???", "?", -1)
}
