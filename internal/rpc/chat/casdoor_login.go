package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/openimsdk/chat/pkg/common/constant"
	"github.com/openimsdk/chat/pkg/common/db/dbutil"
	chatdb "github.com/openimsdk/chat/pkg/common/db/table/chat"
	"github.com/openimsdk/chat/pkg/eerrs"
	"github.com/openimsdk/chat/pkg/protocol/chat"
	constantpb "github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/tools/errs"
)

func (o *chatSvr) CasdoorLogin(ctx context.Context, req *chat.CasdoorLoginReq) (*chat.CasdoorLoginResp, error) {
	profile := req.GetProfile()
	if profile.GetUserID() == "" {
		return nil, errs.ErrArgs.WrapMsg("casdoor user id is empty")
	}
	if profile.GetDisabled() {
		if binding, err := o.Database.TakeCasdoorBindingByCasdoorUserID(ctx, profile.GetUserID()); err == nil {
			_ = o.syncCasdoorBinding(ctx, binding.OpenIMUserID, profile, true)
		}
		return nil, eerrs.ErrForbidden.WrapMsg("casdoor user disabled")
	}

	binding, err := o.Database.TakeCasdoorBindingByCasdoorUserID(ctx, profile.GetUserID())
	if err == nil {
		if binding.Disabled {
			return nil, eerrs.ErrForbidden.WrapMsg("casdoor user disabled")
		}
		if err := o.syncCasdoorBinding(ctx, binding.OpenIMUserID, profile, binding.Disabled); err != nil {
			return nil, err
		}
		token, err := o.Admin.CreateToken(ctx, binding.OpenIMUserID, constant.NormalUser)
		if err != nil {
			return nil, err
		}
		return &chat.CasdoorLoginResp{UserID: binding.OpenIMUserID, ChatToken: token.Token}, nil
	}

	if userID, err := o.resolveExistingCasdoorUser(ctx, profile); err != nil {
		return nil, err
	} else if userID != "" {
		if err := o.createCasdoorBinding(ctx, userID, profile); err != nil {
			return nil, err
		}
		token, err := o.Admin.CreateToken(ctx, userID, constant.NormalUser)
		if err != nil {
			return nil, err
		}
		return &chat.CasdoorLoginResp{UserID: userID, ChatToken: token.Token}, nil
	}

	userID := buildCasdoorOpenIMUserID(profile.GetUserID())
	now := time.Now()
	register := &chatdb.Register{
		UserID:      userID,
		DeviceID:    req.GetDeviceID(),
		IP:          req.GetIp(),
		Platform:    constantpb.PlatformIDToName(int(req.Platform)),
		AccountType: "",
		Mode:        constant.UserMode,
		CreateTime:  now,
	}
	account := &chatdb.Account{
		UserID:         userID,
		Password:       "",
		OperatorUserID: "",
		CreateTime:     now,
		ChangeTime:     now,
	}
	attribute := &chatdb.Attribute{
		UserID:         userID,
		Account:        profile.GetName(),
		PhoneNumber:    profile.GetPhone(),
		Email:          profile.GetEmail(),
		Nickname:       casdoorNickname(req.Profile),
		FaceURL:        profile.GetFaceURL(),
		Gender:         constant.GenderUnknown,
		CreateTime:     now,
		ChangeTime:     now,
		BirthTime:      time.UnixMilli(0),
		AllowVibration: constant.DefaultAllowVibration,
		AllowBeep:      constant.DefaultAllowBeep,
		AllowAddFriend: constant.DefaultAllowAddFriend,
		RegisterType:   constant.AccountRegister,
	}
	credentials := []*chatdb.Credential{{
		UserID:      userID,
		Account:     "casdoor:" + profile.GetUserID(),
		Type:        constant.CredentialAccount,
		AllowChange: false,
	}}
	if err := o.Database.RegisterUser(ctx, register, account, attribute, credentials); err != nil {
		return nil, err
	}
	if err := o.createCasdoorBinding(ctx, userID, profile); err != nil {
		return nil, err
	}
	token, err := o.Admin.CreateToken(ctx, userID, constant.NormalUser)
	if err != nil {
		return nil, err
	}
	return &chat.CasdoorLoginResp{UserID: userID, ChatToken: token.Token}, nil
}

func (o *chatSvr) resolveExistingCasdoorUser(ctx context.Context, profile *chat.CasdoorProfile) (string, error) {
	if profile.GetName() != "" {
		attribute, err := o.Database.TakeAttributeByAccount(ctx, profile.GetName())
		if err == nil {
			return attribute.UserID, nil
		}
		if !dbutil.IsDBNotFound(err) {
			return "", err
		}
	}
	if profile.GetEmail() != "" {
		attribute, err := o.Database.TakeAttributeByEmail(ctx, profile.GetEmail())
		if err == nil {
			return attribute.UserID, nil
		}
		if !dbutil.IsDBNotFound(err) {
			return "", err
		}
	}
	return "", nil
}

func (o *chatSvr) createCasdoorBinding(ctx context.Context, userID string, profile *chat.CasdoorProfile) error {
	return o.Database.CreateCasdoorBinding(ctx, &chatdb.CasdoorBinding{
		CasdoorUserID: profile.GetUserID(),
		CasdoorName:   profile.GetName(),
		OpenIMUserID:  userID,
		Email:         profile.GetEmail(),
		Phone:         profile.GetPhone(),
		Nickname:      casdoorNickname(profile),
		FaceURL:       profile.GetFaceURL(),
		Disabled:      profile.GetDisabled(),
	})
}

func (o *chatSvr) CasdoorSync(ctx context.Context, req *chat.CasdoorSyncReq) (*chat.CasdoorSyncResp, error) {
	profile := req.GetProfile()
	if profile.GetUserID() == "" {
		return nil, errs.ErrArgs.WrapMsg("casdoor user id is empty")
	}
	binding, err := o.Database.TakeCasdoorBindingByCasdoorUserID(ctx, profile.GetUserID())
	if err != nil {
		return nil, err
	}
	if err := o.syncCasdoorBinding(ctx, binding.OpenIMUserID, profile, profile.GetDisabled()); err != nil {
		return nil, err
	}
	if profile.GetDisabled() {
		if err := o.Admin.InvalidateToken(ctx, binding.OpenIMUserID); err != nil {
			return nil, err
		}
	}
	return &chat.CasdoorSyncResp{}, nil
}

func (o *chatSvr) syncCasdoorBinding(ctx context.Context, userID string, profile *chat.CasdoorProfile, disabled bool) error {
	return o.Database.UpdateCasdoorBindingSyncFields(ctx, &chatdb.CasdoorBinding{
		CasdoorUserID: profile.GetUserID(),
		CasdoorName:   profile.GetName(),
		OpenIMUserID:  userID,
		Email:         profile.GetEmail(),
		Phone:         profile.GetPhone(),
		Nickname:      casdoorNickname(profile),
		FaceURL:       profile.GetFaceURL(),
		Disabled:      disabled,
	})
}

func buildCasdoorOpenIMUserID(casdoorUserID string) string {
	sum := sha256.Sum256([]byte(casdoorUserID))
	return "casdoor_" + hex.EncodeToString(sum[:])[:24]
}

func casdoorNickname(profile *chat.CasdoorProfile) string {
	if profile.GetNickname() != "" {
		return profile.GetNickname()
	}
	if profile.GetName() != "" {
		return profile.GetName()
	}
	return profile.GetUserID()
}

var _ = chat.RegisterUserInfo{}
