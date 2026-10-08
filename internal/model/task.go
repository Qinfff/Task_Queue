package model

const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusDead    = "dead"
)

type Task struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Type        string `gorm:"size:64;not null;index" json:"type"`
	Payload     string `gorm:"type:text" json:"payload"`
	Status      string `gorm:"size:32;not null;index" json:"status"`
	Retries     int    `gorm:"not null;default:0" json:"retries"`
	LastError   string `gorm:"type:text" json:"last_error"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
	NextRetryAt int64  `gorm:"default:0" json:"next_retry_at"`
	UpdatedAt   int64  `gorm:"autoUpdateTime" json:"updated_at"`
}
