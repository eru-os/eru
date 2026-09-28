package auth

import (
	"context"
	"github.com/jmoiron/sqlx"
)

type AuthDbI interface {
	GetConn() *sqlx.DB
	SetConn(*sqlx.DB)
	GetDbQuery(ctx context.Context, query string) (finalQuery string)
}

func GetAuthDb(dbType string) AuthDbI {
	switch dbType {
	case "POSTGRES":
		return new(AuthDbPostgres)
	case "MYSQL":
		return new(AuthDbMysql)
	default:
		return new(AuthDb)
	}
}

func (authDb *AuthDb) GetDbQuery(ctx context.Context, query string) (finalQuery string) {
	return query
}

type AuthDb struct {
	Con       *sqlx.DB `json:"-"`
	ConStatus bool     `json:"-"`
}

func (authDb *AuthDb) GetConn() *sqlx.DB {
	return authDb.Con
}

func (authDb *AuthDb) SetConn(con *sqlx.DB) {
	authDb.Con = con
}
