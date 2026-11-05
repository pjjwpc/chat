package chat

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openimsdk/chat/pkg/common/apistruct"
	"github.com/openimsdk/chat/pkg/common/config"
	"github.com/openimsdk/chat/pkg/common/constant"
	"github.com/openimsdk/chat/pkg/common/db/cache"
	"github.com/openimsdk/chat/pkg/common/mctx"
	"github.com/openimsdk/chat/pkg/protocol/admin"
	chatpb "github.com/openimsdk/chat/pkg/protocol/chat"
	"github.com/openimsdk/protocol/sdkws"
	"github.com/openimsdk/tools/a2r"
	"github.com/openimsdk/tools/apiresp"
	"github.com/openimsdk/tools/errs"
	"golang.org/x/oauth2"
)

const casdoorStateTTL = 10 * time.Minute

type CasdoorProfile struct {
	UserID   string
	Name     string
	Email    string
	Phone    string
	Nickname string
	FaceURL  string
	Disabled bool
}

type CasdoorClient interface {
	LoginURL(state string, redirectURI string) string
	ExchangeCode(ctx context.Context, code string, redirectURI string) (*CasdoorProfile, error)
	ExchangePassword(ctx context.Context, username string, password string) (*CasdoorProfile, error)
	LogoutURL(postLogoutRedirectURI string) string
}

type defaultCasdoorClient struct {
	cfg        config.Casdoor
	httpClient *http.Client
}

func NewCasdoorClient(cfg config.Casdoor) CasdoorClient {
	return &defaultCasdoorClient{cfg: cfg, httpClient: http.DefaultClient}
}

func (c *defaultCasdoorClient) LoginURL(state string, redirectURI string) string {
	if !c.cfg.Enabled || c.cfg.Endpoint == "" || c.cfg.ClientID == "" {
		return ""
	}
	base, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return ""
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/login/oauth/authorize"
	query := base.Query()
	query.Set("client_id", c.cfg.ClientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", "openid profile email phone")
	query.Set("state", state)
	base.RawQuery = query.Encode()
	return base.String()
}

func (c *defaultCasdoorClient) ExchangeCode(ctx context.Context, code string, redirectURI string) (*CasdoorProfile, error) {
	if !c.cfg.Enabled || c.cfg.Endpoint == "" || c.cfg.ClientID == "" || c.cfg.ClientSecret == "" {
		return nil, errs.ErrArgs.WrapMsg("casdoor is not configured")
	}
	tokenURL, err := casdoorURL(c.cfg.Endpoint, "/api/login/oauth/access_token")
	if err != nil {
		return nil, err
	}
	oauthCfg := oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL:  tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURI,
		Scopes:      []string{"openid", "profile", "email", "phone"},
	}
	token, err := c.oauthToken(ctx, oauthCfg, func(ctx context.Context, cfg oauth2.Config) (*oauth2.Token, error) {
		return cfg.Exchange(ctx, code)
	})
	if err != nil {
		return nil, err
	}
	return c.profileFromOAuthToken(ctx, token)
}

func (c *defaultCasdoorClient) ExchangePassword(ctx context.Context, username string, password string) (*CasdoorProfile, error) {
	if !c.cfg.Enabled || c.cfg.Endpoint == "" || c.cfg.ClientID == "" || c.cfg.ClientSecret == "" {
		return nil, errs.ErrArgs.WrapMsg("casdoor is not configured")
	}
	tokenURL, err := casdoorURL(c.cfg.Endpoint, "/api/login/oauth/access_token")
	if err != nil {
		return nil, err
	}
	oauthCfg := oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL:  tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		Scopes: []string{"openid", "profile", "email", "phone"},
	}
	token, err := c.oauthToken(ctx, oauthCfg, func(ctx context.Context, cfg oauth2.Config) (*oauth2.Token, error) {
		return cfg.PasswordCredentialsToken(ctx, username, password)
	})
	if err != nil {
		return nil, err
	}
	return c.profileFromOAuthToken(ctx, token)
}

func (c *defaultCasdoorClient) oauthToken(ctx context.Context, oauthCfg oauth2.Config, fetch func(context.Context, oauth2.Config) (*oauth2.Token, error)) (*oauth2.Token, error) {
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	token, err := fetch(context.WithValue(ctx, oauth2.HTTPClient, httpClient), oauthCfg)
	if err != nil {
		return nil, errs.Wrap(err)
	}
	if token.AccessToken == "" {
		return nil, errs.ErrArgs.WrapMsg("casdoor access token is empty")
	}
	if strings.HasPrefix(token.AccessToken, "error:") {
		return nil, errs.ErrArgs.WrapMsg(strings.TrimSpace(strings.TrimPrefix(token.AccessToken, "error:")))
	}
	return token, nil
}

func (c *defaultCasdoorClient) profileFromOAuthToken(ctx context.Context, token *oauth2.Token) (*CasdoorProfile, error) {
	profile, err := c.profileFromAccessToken(token.AccessToken)
	if err == nil {
		return profile, nil
	}
	return c.profileFromUserinfo(ctx, token.AccessToken)
}

func (c *defaultCasdoorClient) LogoutURL(postLogoutRedirectURI string) string {
	if !c.cfg.Enabled || c.cfg.Endpoint == "" {
		return ""
	}
	base, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return ""
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/logout"
	query := base.Query()
	query.Set("post_logout_redirect_uri", postLogoutRedirectURI)
	base.RawQuery = query.Encode()
	return base.String()
}

type CasdoorLoginURLReq struct {
	Platform    int32  `json:"platform"`
	Client      string `json:"client"`
	RedirectURI string `json:"redirectURI"`
}

type CasdoorLoginURLResp struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

type CasdoorCallbackReq struct {
	Code        string `json:"code"`
	State       string `json:"state"`
	Platform    int32  `json:"platform"`
	Client      string `json:"client"`
	DeviceID    string `json:"deviceID"`
	RedirectURI string `json:"redirectURI"`
}

type CasdoorPasswordReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Platform int32  `json:"platform"`
	Client   string `json:"client"`
	DeviceID string `json:"deviceID"`
}

type CasdoorLogoutReq struct {
	RedirectURI string `json:"redirectURI"`
}

type CasdoorLogoutResp struct {
	LogoutURL string `json:"logoutURL"`
}

type CasdoorSyncReq struct {
	Profile CasdoorProfile `json:"profile"`
	Secret  string         `json:"secret"`
}

type casdoorState struct {
	RedirectURI string
	Client      string
	ExpiresAt   time.Time
}

func (o *Api) CasdoorLoginURL(c *gin.Context) {
	req, err := a2r.ParseRequest[CasdoorLoginURLReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.Client == "" || req.RedirectURI == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("client and redirectURI must be set"))
		return
	}
	if o.casdoorClient == nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("casdoor client is not configured"))
		return
	}
	state, err := newCasdoorState()
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	o.saveCasdoorState(state, &casdoorState{
		RedirectURI: req.RedirectURI,
		Client:      req.Client,
		ExpiresAt:   time.Now().Add(casdoorStateTTL),
	})
	apiresp.GinSuccess(c, &CasdoorLoginURLResp{
		URL:   o.casdoorClient.LoginURL(state, req.RedirectURI),
		State: state,
	})
}

func (o *Api) CasdoorCallback(c *gin.Context) {
	req, err := a2r.ParseRequest[CasdoorCallbackReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.Code == "" || req.State == "" || req.RedirectURI == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("code, state and redirectURI must be set"))
		return
	}
	if o.casdoorClient == nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("casdoor client is not configured"))
		return
	}
	if err := o.consumeCasdoorState(req.State, req.RedirectURI, req.Client); err != nil {
		apiresp.GinError(c, err)
		return
	}
	profile, err := o.casdoorClient.ExchangeCode(c, req.Code, req.RedirectURI)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	o.completeCasdoorLogin(c, profile, req.Platform, req.DeviceID)
}

func (o *Api) CasdoorPassword(c *gin.Context) {
	req, err := a2r.ParseRequest[CasdoorPasswordReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.Username == "" || req.Password == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("username and password must be set"))
		return
	}
	if o.casdoorClient == nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("casdoor client is not configured"))
		return
	}
	profile, err := o.casdoorClient.ExchangePassword(c, req.Username, req.Password)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	o.completeCasdoorLogin(c, profile, req.Platform, req.DeviceID)
}

func (o *Api) completeCasdoorLogin(c *gin.Context, profile *CasdoorProfile, platform int32, deviceID string) {
	ip, err := o.GetClientIP(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.chatClient.CasdoorLogin(c, &chatpb.CasdoorLoginReq{
		Profile: &chatpb.CasdoorProfile{
			CasdoorUserId: profile.UserID,
			Name:          profile.Name,
			Email:         profile.Email,
			Phone:         profile.Phone,
			Nickname:      profile.Nickname,
			FaceUrl:       profile.FaceURL,
			Disabled:      profile.Disabled,
		},
		Platform:  platform,
		DeviceId:  deviceID,
		IpAddress: ip,
	})
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	adminToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiCtx := mctx.WithApiToken(c, adminToken)
	nickname := profile.Nickname
	if nickname == "" {
		nickname = profile.Name
	}
	if nickname == "" {
		nickname = resp.UserId
	}
	isUserNotExist, err := o.imApiCaller.AccountCheckSingle(apiCtx, resp.UserId)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if isUserNotExist {
		err = o.imApiCaller.RegisterUser(apiCtx, []*sdkws.UserInfo{{
			UserID:     resp.UserId,
			Nickname:   nickname,
			FaceURL:    profile.FaceURL,
			CreateTime: time.Now().UnixMilli(),
		}})
	} else {
		err = o.imApiCaller.UpdateUserInfo(apiCtx, resp.UserId, nickname, profile.FaceURL)
	}
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	imToken, err := o.imApiCaller.GetUserToken(apiCtx, resp.UserId, platform)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &apistruct.LoginResp{
		ImToken:   imToken,
		UserID:    resp.UserId,
		ChatToken: resp.ChatToken,
	})
}

func (o *Api) CasdoorLogout(c *gin.Context) {
	req, err := a2r.ParseRequest[CasdoorLogoutReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	userID, userType, err := mctx.Check(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if userType != constant.NormalUser {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("not user"))
		return
	}
	if _, err := o.adminClient.InvalidateToken(c, &admin.InvalidateTokenReq{UserID: userID}); err != nil {
		apiresp.GinError(c, err)
		return
	}
	logoutURL := ""
	if o.casdoorClient != nil && req.RedirectURI != "" {
		logoutURL = o.casdoorClient.LogoutURL(req.RedirectURI)
	}
	apiresp.GinSuccess(c, &CasdoorLogoutResp{LogoutURL: logoutURL})
}

func (o *Api) CasdoorSync(c *gin.Context) {
	req, err := a2r.ParseRequest[CasdoorSyncReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.Profile.UserID == "" {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("casdoor userID is empty"))
		return
	}
	if o.casdoorSyncSecret == "" || req.Secret != o.casdoorSyncSecret {
		apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("casdoor sync secret is invalid"))
		return
	}
	_, err = o.chatClient.CasdoorSync(c, &chatpb.CasdoorSyncReq{
		Profile: &chatpb.CasdoorProfile{
			CasdoorUserId: req.Profile.UserID,
			Name:          req.Profile.Name,
			Email:         req.Profile.Email,
			Phone:         req.Profile.Phone,
			Nickname:      req.Profile.Nickname,
			FaceUrl:       req.Profile.FaceURL,
			Disabled:      req.Profile.Disabled,
		},
	})
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, struct{}{})
}

func newCasdoorState() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", errs.Wrap(err)
	}
	return hex.EncodeToString(buf[:]), nil
}

func (o *Api) saveCasdoorState(state string, data *casdoorState) {
	// Try Redis first if available
	if o.useRedisStateCache && o.casdoorStateCache != nil {
		redisData := &cache.CasdoorStateData{
			RedirectURI: data.RedirectURI,
			Client:      data.Client,
			ExpiresAt:   data.ExpiresAt,
		}
		if err := o.casdoorStateCache.SaveState(context.Background(), state, redisData); err == nil {
			return
		}
		// Fall back to in-memory if Redis fails
	}
	
	// In-memory fallback
	o.casdoorStateMu.Lock()
	defer o.casdoorStateMu.Unlock()
	if o.casdoorStates == nil {
		o.casdoorStates = make(map[string]*casdoorState)
	}
	now := time.Now()
	for key, item := range o.casdoorStates {
		if now.After(item.ExpiresAt) {
			delete(o.casdoorStates, key)
		}
	}
	o.casdoorStates[state] = data
}

func (o *Api) consumeCasdoorState(state string, redirectURI string, client string) error {
	// Try Redis first if available
	if o.useRedisStateCache && o.casdoorStateCache != nil {
		if err := o.casdoorStateCache.ValidateState(context.Background(), state, redirectURI, client); err == nil {
			return nil
		}
		// Fall back to in-memory if Redis fails
	}
	
	// In-memory fallback
	o.casdoorStateMu.Lock()
	defer o.casdoorStateMu.Unlock()
	item, ok := o.casdoorStates[state]
	if !ok {
		return errs.ErrArgs.WrapMsg("casdoor state is invalid")
	}
	delete(o.casdoorStates, state)
	if time.Now().After(item.ExpiresAt) {
		return errs.ErrArgs.WrapMsg("casdoor state is expired")
	}
	if item.RedirectURI != redirectURI {
		return errs.ErrArgs.WrapMsg("casdoor redirectURI mismatch")
	}
	if item.Client != "" && client != "" && item.Client != client {
		return errs.ErrArgs.WrapMsg("casdoor client mismatch")
	}
	return nil
}

func casdoorURL(endpoint string, path string) (string, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return "", errs.Wrap(err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	base.RawQuery = ""
	return base.String(), nil
}

type casdoorClaims struct {
	Sub             string `json:"sub"`
	ID              string `json:"id"`
	Name            string `json:"name"`
	DisplayName     string `json:"displayName"`
	PreferredName   string `json:"preferred_username"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Avatar          string `json:"avatar"`
	Picture         string `json:"picture"`
	PermanentAvatar string `json:"permanentAvatar"`
	IsForbidden     bool   `json:"isForbidden"`
	IsDeleted       bool   `json:"isDeleted"`
}

func (c *defaultCasdoorClient) profileFromAccessToken(accessToken string) (*CasdoorProfile, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return nil, errs.ErrArgs.WrapMsg("casdoor access token is not jwt")
	}
	payload, err := jwtDecodeSegment(parts[1])
	if err != nil {
		return nil, err
	}
	var claims casdoorClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errs.Wrap(err)
	}
	return profileFromClaims(claims)
}

func (c *defaultCasdoorClient) profileFromUserinfo(ctx context.Context, accessToken string) (*CasdoorProfile, error) {
	userinfoURL, err := casdoorURL(c.cfg.Endpoint, "/api/userinfo")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, errs.Wrap(err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errs.Wrap(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, errs.Wrap(err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errs.ErrArgs.WrapMsg("casdoor userinfo failed: " + string(body))
	}
	var claims casdoorClaims
	if err := json.Unmarshal(body, &claims); err != nil {
		return nil, errs.Wrap(err)
	}
	return profileFromClaims(claims)
}

func profileFromClaims(claims casdoorClaims) (*CasdoorProfile, error) {
	userID := firstNonEmpty(claims.ID, claims.Sub, claims.Name, claims.PreferredName)
	if userID == "" {
		return nil, errs.ErrArgs.WrapMsg("casdoor userID is empty")
	}
	return &CasdoorProfile{
		UserID:   userID,
		Name:     firstNonEmpty(claims.Name, claims.PreferredName),
		Email:    claims.Email,
		Phone:    claims.Phone,
		Nickname: firstNonEmpty(claims.DisplayName, claims.Name, claims.PreferredName),
		FaceURL:  firstNonEmpty(claims.Avatar, claims.PermanentAvatar, claims.Picture),
		Disabled: claims.IsForbidden || claims.IsDeleted,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func jwtDecodeSegment(segment string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(segment)
		if err != nil {
			return nil, errs.Wrap(err)
		}
	}
	return decoded, nil
}
