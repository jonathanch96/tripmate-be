package users

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	Email        string         `gorm:"column:email;type:citext;not null"`
	Name         string         `gorm:"column:name;not null"`
	PasswordHash string         `gorm:"column:password_hash;not null"`
	AvatarURL    *string        `gorm:"column:avatar_url"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (User) TableName() string { return "tripmate.users" }
