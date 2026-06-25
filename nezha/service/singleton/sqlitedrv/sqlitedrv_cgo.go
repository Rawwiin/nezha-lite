//go:build !modernc

package sqlitedrv

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Open 使用 CGO 版本的 go-sqlite3 驱动打开 SQLite 数据库
func Open(path string) gorm.Dialector {
	return sqlite.Open(path)
}
