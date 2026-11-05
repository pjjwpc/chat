package chat

import (
	"context"
	"testing"
	"time"

	chatdb "github.com/openimsdk/chat/pkg/common/db/table/chat"
	"github.com/openimsdk/chat/pkg/protocol/admin"
	chatpb "github.com/openimsdk/chat/pkg/protocol/chat"
	"github.com/openimsdk/tools/db/pagination"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestCasdoorLoginCreatesBindingOnFirstLogin(t *testing.T) {
	db := newFakeCasdoorDatabase()
	svr := &chatSvr{
		Database: db,
		Admin:    fakeCasdoorAdmin{token: "chat-token"},
	}

	resp, err := svr.CasdoorLogin(context.Background(), &chatpb.CasdoorLoginReq{
		Profile: &chatpb.CasdoorProfile{
			UserID:   "casdoor-1",
			Name:     "alice",
			Nickname: "Alice",
			FaceURL:  "https://example.com/avatar.png",
			Email:    "alice@example.com",
		},
		Platform: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.UserID == "" {
		t.Fatal("expected generated userID")
	}
	if resp.ChatToken != "chat-token" {
		t.Fatalf("unexpected chat token: %q", resp.ChatToken)
	}
	if db.bindings["casdoor-1"] == nil {
		t.Fatal("expected casdoor binding to be created")
	}
}

func TestCasdoorLoginReusesExistingBinding(t *testing.T) {
	db := newFakeCasdoorDatabase()
	db.bindings["casdoor-1"] = &chatdb.CasdoorBinding{
		CasdoorUserID: "casdoor-1",
		OpenIMUserID:  "openim-existing",
	}
	svr := &chatSvr{
		Database: db,
		Admin:    fakeCasdoorAdmin{token: "chat-token"},
	}

	resp, err := svr.CasdoorLogin(context.Background(), &chatpb.CasdoorLoginReq{
		Profile:  &chatpb.CasdoorProfile{UserID: "casdoor-1", Nickname: "Alice"},
		Platform: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.UserID != "openim-existing" {
		t.Fatalf("expected existing userID, got %q", resp.UserID)
	}
	if db.registerCount != 0 {
		t.Fatalf("expected no new registration, got %d", db.registerCount)
	}
}

func TestCasdoorLoginBindsExistingUserByAccount(t *testing.T) {
	db := newFakeCasdoorDatabase()
	db.attributesByAccount["alice"] = &chatdb.Attribute{
		UserID:  "openim-existing",
		Account: "alice",
		Email:   "alice@example.com",
	}
	svr := &chatSvr{
		Database: db,
		Admin:    fakeCasdoorAdmin{token: "chat-token"},
	}

	resp, err := svr.CasdoorLogin(context.Background(), &chatpb.CasdoorLoginReq{
		Profile: &chatpb.CasdoorProfile{
			UserID: "casdoor-1",
			Name:   "alice",
			Email:  "alice@example.com",
		},
		Platform: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.UserID != "openim-existing" {
		t.Fatalf("expected existing userID, got %q", resp.UserID)
	}
	if db.registerCount != 0 {
		t.Fatalf("expected no new registration, got %d", db.registerCount)
	}
	binding := db.bindings["casdoor-1"]
	if binding == nil {
		t.Fatal("expected casdoor binding to be created")
	}
	if binding.OpenIMUserID != "openim-existing" {
		t.Fatalf("expected binding to existing user, got %q", binding.OpenIMUserID)
	}
}

func TestCasdoorLoginRejectsDisabledUser(t *testing.T) {
	db := newFakeCasdoorDatabase()
	svr := &chatSvr{
		Database: db,
		Admin:    fakeCasdoorAdmin{token: "chat-token"},
	}

	_, err := svr.CasdoorLogin(context.Background(), &chatpb.CasdoorLoginReq{
		Profile:  &chatpb.CasdoorProfile{UserID: "casdoor-1", Disabled: true},
		Platform: 5,
	})
	if err == nil {
		t.Fatal("expected disabled casdoor user to be rejected")
	}
	if db.registerCount != 0 {
		t.Fatalf("expected disabled user not to register, got %d", db.registerCount)
	}
}

func TestCasdoorSyncUpdatesBindingAndInvalidatesDisabledUser(t *testing.T) {
	db := newFakeCasdoorDatabase()
	db.bindings["casdoor-1"] = &chatdb.CasdoorBinding{
		CasdoorUserID: "casdoor-1",
		OpenIMUserID:  "openim-existing",
	}
	var invalidatedUserID string
	svr := &chatSvr{
		Database: db,
		Admin:    fakeCasdoorAdmin{invalidatedUserID: &invalidatedUserID},
	}

	_, err := svr.CasdoorSync(context.Background(), &chatpb.CasdoorSyncReq{
		Profile: &chatpb.CasdoorProfile{
			UserID:   "casdoor-1",
			Name:     "alice",
			Nickname: "Alice Updated",
			FaceURL:  "https://example.com/new.png",
			Disabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	binding := db.bindings["casdoor-1"]
	if binding.Nickname != "Alice Updated" || binding.FaceURL != "https://example.com/new.png" || !binding.Disabled {
		t.Fatalf("expected binding sync fields to update, got %#v", binding)
	}
	if invalidatedUserID != "openim-existing" {
		t.Fatalf("expected disabled sync to invalidate token for openim-existing, got %q", invalidatedUserID)
	}
}

func TestCasdoorLoginDisablesExistingBindingWhenProfileDisabled(t *testing.T) {
	db := newFakeCasdoorDatabase()
	db.bindings["casdoor-1"] = &chatdb.CasdoorBinding{
		CasdoorUserID: "casdoor-1",
		OpenIMUserID:  "openim-existing",
	}
	svr := &chatSvr{
		Database: db,
		Admin:    fakeCasdoorAdmin{token: "chat-token"},
	}

	_, err := svr.CasdoorLogin(context.Background(), &chatpb.CasdoorLoginReq{
		Profile:  &chatpb.CasdoorProfile{UserID: "casdoor-1", Disabled: true},
		Platform: 5,
	})
	if err == nil {
		t.Fatal("expected disabled casdoor user to be rejected")
	}
	if !db.bindings["casdoor-1"].Disabled {
		t.Fatal("expected existing binding to be marked disabled")
	}
}

type fakeCasdoorAdmin struct {
	token             string
	invalidatedUserID *string
}

func (f fakeCasdoorAdmin) GetConfig(context.Context) (map[string]string, error) { return nil, nil }
func (f fakeCasdoorAdmin) CheckInvitationCode(context.Context, string) error    { return nil }
func (f fakeCasdoorAdmin) CheckRegister(context.Context, string) error          { return nil }
func (f fakeCasdoorAdmin) CheckLogin(context.Context, string, string) error     { return nil }
func (f fakeCasdoorAdmin) UseInvitationCode(context.Context, string, string) error {
	return nil
}
func (f fakeCasdoorAdmin) CheckNilOrAdmin(context.Context) (bool, error) { return true, nil }
func (f fakeCasdoorAdmin) CreateToken(context.Context, string, int32) (*admin.CreateTokenResp, error) {
	return &admin.CreateTokenResp{Token: f.token}, nil
}
func (f fakeCasdoorAdmin) InvalidateToken(_ context.Context, userID string) error {
	if f.invalidatedUserID != nil {
		*f.invalidatedUserID = userID
	}
	return nil
}

type fakeCasdoorDatabase struct {
	bindings            map[string]*chatdb.CasdoorBinding
	accounts            map[string]*chatdb.Account
	attributesByAccount map[string]*chatdb.Attribute
	attributesByEmail   map[string]*chatdb.Attribute
	registerCount       int
}

func newFakeCasdoorDatabase() *fakeCasdoorDatabase {
	return &fakeCasdoorDatabase{
		bindings:            map[string]*chatdb.CasdoorBinding{},
		accounts:            map[string]*chatdb.Account{},
		attributesByAccount: map[string]*chatdb.Attribute{},
		attributesByEmail:   map[string]*chatdb.Attribute{},
	}
}

func (f *fakeCasdoorDatabase) GetUser(_ context.Context, userID string) (*chatdb.Account, error) {
	account := f.accounts[userID]
	if account == nil {
		return nil, mongo.ErrNoDocuments
	}
	return account, nil
}
func (f *fakeCasdoorDatabase) UpdateUseInfo(context.Context, string, map[string]any, []*chatdb.Credential, []*chatdb.Credential) error {
	return nil
}
func (f *fakeCasdoorDatabase) FindAttribute(context.Context, []string) ([]*chatdb.Attribute, error) {
	return nil, nil
}
func (f *fakeCasdoorDatabase) FindAttributeByAccount(context.Context, []string) ([]*chatdb.Attribute, error) {
	return nil, nil
}
func (f *fakeCasdoorDatabase) TakeAttributeByPhone(context.Context, string, string) (*chatdb.Attribute, error) {
	return nil, mongo.ErrNoDocuments
}
func (f *fakeCasdoorDatabase) TakeAttributeByEmail(_ context.Context, email string) (*chatdb.Attribute, error) {
	if attribute := f.attributesByEmail[email]; attribute != nil {
		return attribute, nil
	}
	return nil, mongo.ErrNoDocuments
}
func (f *fakeCasdoorDatabase) TakeAttributeByAccount(_ context.Context, account string) (*chatdb.Attribute, error) {
	if attribute := f.attributesByAccount[account]; attribute != nil {
		return attribute, nil
	}
	return nil, mongo.ErrNoDocuments
}
func (f *fakeCasdoorDatabase) TakeAttributeByUserID(context.Context, string) (*chatdb.Attribute, error) {
	return nil, mongo.ErrNoDocuments
}
func (f *fakeCasdoorDatabase) TakeAccount(context.Context, string) (*chatdb.Account, error) {
	return nil, mongo.ErrNoDocuments
}
func (f *fakeCasdoorDatabase) TakeCredentialByAccount(context.Context, string) (*chatdb.Credential, error) {
	return nil, mongo.ErrNoDocuments
}
func (f *fakeCasdoorDatabase) TakeCredentialsByUserID(context.Context, string) ([]*chatdb.Credential, error) {
	return []*chatdb.Credential{{UserID: "user", Type: 0, Account: "account"}}, nil
}
func (f *fakeCasdoorDatabase) TakeLastVerifyCode(context.Context, string) (*chatdb.VerifyCode, error) {
	return nil, nil
}
func (f *fakeCasdoorDatabase) Search(context.Context, int32, string, int32, pagination.Pagination) (int64, []*chatdb.Attribute, error) {
	return 0, nil, nil
}
func (f *fakeCasdoorDatabase) SearchUser(context.Context, string, []string, []int32, pagination.Pagination) (int64, []*chatdb.Attribute, error) {
	return 0, nil, nil
}
func (f *fakeCasdoorDatabase) CountVerifyCodeRange(context.Context, string, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeCasdoorDatabase) AddVerifyCode(context.Context, *chatdb.VerifyCode, func() error) error {
	return nil
}
func (f *fakeCasdoorDatabase) UpdateVerifyCodeIncrCount(context.Context, string) error { return nil }
func (f *fakeCasdoorDatabase) DelVerifyCode(context.Context, string) error             { return nil }
func (f *fakeCasdoorDatabase) RegisterUser(_ context.Context, _ *chatdb.Register, account *chatdb.Account, _ *chatdb.Attribute, _ []*chatdb.Credential) error {
	f.registerCount++
	f.accounts[account.UserID] = account
	return nil
}
func (f *fakeCasdoorDatabase) LoginRecord(context.Context, *chatdb.UserLoginRecord, *string) error {
	return nil
}
func (f *fakeCasdoorDatabase) UpdatePassword(context.Context, string, string) error { return nil }
func (f *fakeCasdoorDatabase) UpdatePasswordAndDeleteVerifyCode(context.Context, string, string, string) error {
	return nil
}
func (f *fakeCasdoorDatabase) NewUserCountTotal(context.Context, *time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeCasdoorDatabase) UserLoginCountTotal(context.Context, *time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeCasdoorDatabase) UserLoginCountRangeEverydayTotal(context.Context, *time.Time, *time.Time) (map[string]int64, int64, error) {
	return nil, 0, nil
}
func (f *fakeCasdoorDatabase) DelUserAccount(context.Context, []string) error { return nil }
func (f *fakeCasdoorDatabase) TakeCasdoorBindingByCasdoorUserID(_ context.Context, casdoorUserID string) (*chatdb.CasdoorBinding, error) {
	binding := f.bindings[casdoorUserID]
	if binding == nil {
		return nil, mongo.ErrNoDocuments
	}
	return binding, nil
}
func (f *fakeCasdoorDatabase) TakeCasdoorBindingByOpenIMUserID(_ context.Context, openIMUserID string) (*chatdb.CasdoorBinding, error) {
	for _, binding := range f.bindings {
		if binding.OpenIMUserID == openIMUserID {
			return binding, nil
		}
	}
	return nil, mongo.ErrNoDocuments
}
func (f *fakeCasdoorDatabase) CreateCasdoorBinding(_ context.Context, binding *chatdb.CasdoorBinding) error {
	f.bindings[binding.CasdoorUserID] = binding
	return nil
}
func (f *fakeCasdoorDatabase) UpdateCasdoorBindingSyncFields(_ context.Context, binding *chatdb.CasdoorBinding) error {
	f.bindings[binding.CasdoorUserID] = binding
	return nil
}
func (f *fakeCasdoorDatabase) SetCasdoorBindingDisabled(_ context.Context, casdoorUserID string, disabled bool) error {
	f.bindings[casdoorUserID].Disabled = disabled
	return nil
}
