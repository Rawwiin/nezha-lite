package model

type ServiceForm struct {
	Name                string          `json:"name,omitempty" minLength:"1"`
	Target              string          `json:"target,omitempty"`
	Type                uint8           `json:"type,omitempty"`
	Cover               uint8           `json:"cover,omitempty"`
	DisplayIndex        int             `json:"display_index,omitempty" default:"0"` // 展示排序，越大越靠前
	Notify              bool            `json:"notify,omitempty" validate:"optional"`
	Duration            uint64          `json:"duration,omitempty"`
	MinLatency          float32         `json:"min_latency,omitempty" default:"0.0"`
	MaxLatency          float32         `json:"max_latency,omitempty" default:"0.0"`
	LatencyNotify       bool            `json:"latency_notify,omitempty" validate:"optional"`
	HideForGuest        bool            `json:"hide_for_guest,omitempty" validate:"optional"`
	SkipServers         map[uint64]bool `json:"skip_servers,omitempty"`
	NotificationGroupID uint64          `json:"notification_group_id,omitempty"`
}

type ServiceResponseItem struct {
	ServiceName string       `json:"service_name,omitempty"`
	CurrentUp   uint64       `json:"current_up"`
	CurrentDown uint64       `json:"current_down"`
	TotalUp     uint64       `json:"total_up"`
	TotalDown   uint64       `json:"total_down"`
	Delay       *[30]float64 `json:"delay,omitempty"`
	Up          *[30]uint64  `json:"up,omitempty"`
	Down        *[30]uint64  `json:"down,omitempty"`
}

func (r ServiceResponseItem) TotalUptime() float32 {
	if r.TotalUp+r.TotalDown == 0 {
		return 0
	}
	return float32(r.TotalUp) / (float32(r.TotalUp + r.TotalDown)) * 100
}

type ServiceResponse struct {
	Services map[uint64]ServiceResponseItem `json:"services,omitempty"`
}
