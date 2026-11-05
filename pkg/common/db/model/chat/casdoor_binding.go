package chat

import (
	"context"
	"time"

	table "github.com/openimsdk/chat/pkg/common/db/table/chat"
	"github.com/openimsdk/tools/db/mongoutil"
	"github.com/openimsdk/tools/errs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func NewCasdoorBinding(db *mongo.Database) (table.CasdoorBindingInterface, error) {
	coll := db.Collection(table.CasdoorBinding{}.TableName())
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "casdoor_user_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "openim_user_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &CasdoorBinding{coll: coll}, nil
}

type CasdoorBinding struct {
	coll *mongo.Collection
}

func (o *CasdoorBinding) TakeByCasdoorUserID(ctx context.Context, casdoorUserID string) (*table.CasdoorBinding, error) {
	return mongoutil.FindOne[*table.CasdoorBinding](ctx, o.coll, bson.M{"casdoor_user_id": casdoorUserID})
}

func (o *CasdoorBinding) TakeByOpenIMUserID(ctx context.Context, openIMUserID string) (*table.CasdoorBinding, error) {
	return mongoutil.FindOne[*table.CasdoorBinding](ctx, o.coll, bson.M{"openim_user_id": openIMUserID})
}

func (o *CasdoorBinding) Create(ctx context.Context, binding *table.CasdoorBinding) error {
	now := time.Now()
	if binding.CreateTime.IsZero() {
		binding.CreateTime = now
	}
	binding.UpdateTime = now
	binding.LastSyncTime = now
	return mongoutil.InsertMany(ctx, o.coll, []*table.CasdoorBinding{binding})
}

func (o *CasdoorBinding) UpdateSyncFields(ctx context.Context, binding *table.CasdoorBinding) error {
	now := time.Now()
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"casdoor_user_id": binding.CasdoorUserID}, bson.M{
		"$set": bson.M{
			"casdoor_name":   binding.CasdoorName,
			"email":          binding.Email,
			"phone":          binding.Phone,
			"nickname":       binding.Nickname,
			"face_url":       binding.FaceURL,
			"disabled":       binding.Disabled,
			"update_time":    now,
			"last_sync_time": now,
		},
	}, false)
}

func (o *CasdoorBinding) SetDisabled(ctx context.Context, casdoorUserID string, disabled bool) error {
	now := time.Now()
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"casdoor_user_id": casdoorUserID}, bson.M{
		"$set": bson.M{
			"disabled":       disabled,
			"update_time":    now,
			"last_sync_time": now,
		},
	}, false)
}
