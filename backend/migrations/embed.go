// Package migrations встраивает SQL-миграции в бинарник.
//
// Лежит рядом с .sql, а не в platform/db: //go:embed не умеет
// подниматься по каталогам (../migrations).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
