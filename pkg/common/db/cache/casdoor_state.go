// Copyright © 2023 OpenIM open source community. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openimsdk/tools/errs"
	"github.com/redis/go-redis/v9"
)

const (
	casdoorStatePrefix = "CASDOOR_STATE:"
	defaultStateTTL    = 10 * time.Minute
)

type CasdoorStateData struct {
	RedirectURI string    `json:"redirect_uri"`
	Client      string    `json:"client"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type CasdoorStateInterface interface {
	SaveState(ctx context.Context, state string, data *CasdoorStateData) error
	GetState(ctx context.Context, state string) (*CasdoorStateData, error)
	DeleteState(ctx context.Context, state string) error
	ValidateState(ctx context.Context, state string, redirectURI string, client string) error
}

type CasdoorStateRedis struct {
	rdb redis.UniversalClient
}

func NewCasdoorStateCache(rdb redis.UniversalClient) CasdoorStateInterface {
	return &CasdoorStateRedis{rdb: rdb}
}

func (c *CasdoorStateRedis) SaveState(ctx context.Context, state string, data *CasdoorStateData) error {
	if data == nil {
		return errs.New("casdoor state data is nil")
	}
	key := casdoorStatePrefix + state
	
	jsonData, err := json.Marshal(data)
	if err != nil {
		return errs.Wrap(err)
	}
	
	ttl := time.Until(data.ExpiresAt)
	if ttl <= 0 {
		ttl = defaultStateTTL
	}
	
	return errs.Wrap(c.rdb.Set(ctx, key, jsonData, ttl).Err())
}

func (c *CasdoorStateRedis) GetState(ctx context.Context, state string) (*CasdoorStateData, error) {
	key := casdoorStatePrefix + state
	
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, errs.Wrap(errs.New("casdoor state not found"))
		}
		return nil, errs.Wrap(err)
	}
	
	var data CasdoorStateData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, errs.Wrap(err)
	}
	
	return &data, nil
}

func (c *CasdoorStateRedis) DeleteState(ctx context.Context, state string) error {
	key := casdoorStatePrefix + state
	return errs.Wrap(c.rdb.Del(ctx, key).Err())
}

func (c *CasdoorStateRedis) ValidateState(ctx context.Context, state string, redirectURI string, client string) error {
	data, err := c.GetState(ctx, state)
	if err != nil {
		return err
	}
	
	if data.RedirectURI != redirectURI {
		_ = c.DeleteState(ctx, state)
		return errs.New("casdoor redirect uri mismatch")
	}
	
	if data.Client != client {
		_ = c.DeleteState(ctx, state)
		return errs.New("casdoor client mismatch")
	}
	
	if time.Now().After(data.ExpiresAt) {
		_ = c.DeleteState(ctx, state)
		return errs.New("casdoor state expired")
	}
	
	_ = c.DeleteState(ctx, state)
	return nil
}
