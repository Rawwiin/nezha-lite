package model

import "time"

// Transfer 流量统计记录
type Transfer struct {
	ServerID  uint64    `gorm:"index"`
	In        uint64
	Out       uint64
	CreatedAt time.Time `gorm:"index"`
}
