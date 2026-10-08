package model

type TaskResult struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	TaskID    uint   `gorm:"not null;uniqueIndex" json:"task_id"`
	Result    string `gorm:"size:255" json:"result"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}
