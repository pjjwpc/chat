package chat

import (
	"context"
	"time"
)

type CasdoorBinding struct {
	CasdoorUserID string    `bson:"casdoor_user_id"`
	CasdoorName   string    `bson:"casdoor_name"`
	OpenIMUserID  string    `bson:"openim_user_id"`
	Email         string    `bson:"email"`
	Phone         string    `bson:"phone"`
	Nickname      string    `bson:"nickname"`
	FaceURL       string    `bson:"face_url"`
	Disabled      bool      `bson:"disabled"`
	CreateTime    time.Time `bson:"create_time"`
	UpdateTime    time.Time `bson:"update_time"`
	LastSyncTime  time.Time `bson:"last_sync_time"`
}

func (CasdoorBinding) TableName() string {
	return "casdoor_bindings"
}

type CasdoorBindingInterface interface {
	TakeByCasdoorUserID(ctx context.Context, casdoorUserID string) (*CasdoorBinding, error)
	TakeByOpenIMUserID(ctx context.Context, openIMUserID string) (*CasdoorBinding, error)
	Create(ctx context.Context, binding *CasdoorBinding) error
	UpdateSyncFields(ctx context.Context, binding *CasdoorBinding) error
	SetDisabled(ctx context.Context, casdoorUserID string, disabled bool) error
}
