//go:build modernc

package sqlitedrv

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

// Open 使用纯 Go 实现的 modernc.org/sqlite 驱动打开 SQLite 数据库
// 编译时需要添加 -tags modernc，且无需 CGO
// 注意：modernc 不支持 mattn 风格的 _busy_timeout / _journal_mode DSN 参数，
// PRAGMA 通过 InitDB 中的 Exec 设置
func Open(path string) gorm.Dialector {
	return sqlite.Dialector{DriverName: "sqlite", DSN: path}
}
