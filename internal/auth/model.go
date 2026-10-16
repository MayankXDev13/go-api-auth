package auth

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name         string    `gorm:"size:100;not null" json:"name"`
	Email        string    `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"not null" json:"-"`
	TokenVersion int       `gorm:"not null;default:0" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RefreshToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"-"`
	UserID    uuid.UUID  `gorm:"type:uuid;index;not null" json:"-"`
	FamilyID  uuid.UUID  `gorm:"type:uuid;index;not null" json:"-"`
	TokenHash string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"-"`
	RevokedAt *time.Time `json:"-"`
	CreatedAt time.Time  `json:"-"`
}

type PasswordResetToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"-"`
	UserID    uuid.UUID  `gorm:"type:uuid;index;not null" json:"-"`
	TokenHash string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"-"`
	UsedAt    *time.Time `json:"-"`
	CreatedAt time.Time  `json:"-"`
}

func (u *User) BeforeCreate(_ *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

func (t *RefreshToken) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

func (t *PasswordResetToken) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// IssueResetToken clears old unused tokens, stores a new hashed one with the
// given lifetime, and returns the raw token. Used by the handler and by tests.
func IssueResetToken(db *gorm.DB, userID uuid.UUID, ttl time.Duration) (string, error) {
	_ = db.Where("user_id = ? AND used_at IS NULL", userID).Delete(&PasswordResetToken{}).Error
	raw, hash, err := NewOpaqueToken()
	if err != nil {
		return "", err
	}
	if err := db.Create(&PasswordResetToken{UserID: userID, TokenHash: hash, ExpiresAt: time.Now().Add(ttl)}).Error; err != nil {
		return "", err
	}
	return raw, nil
}
