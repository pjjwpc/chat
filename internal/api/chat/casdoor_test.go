package chat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openimsdk/chat/pkg/common/config"
)

func TestCasdoorRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	SetChatRoute(router, &Api{}, nil)

	routes := router.Routes()
	want := map[string]bool{
		"POST /account/casdoor/login_url": false,
		"POST /account/casdoor/callback":  false,
		"POST /account/casdoor/password":  false,
		"POST /account/casdoor/logout":    false,
		"POST /account/casdoor/sync":      false,
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Fatalf("expected route %s to be registered; got routes: %#v", route, routes)
		}
	}
}

func TestCasdoorLoginURLRequiresRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := &Api{}
	router.POST("/account/casdoor/login_url", api.CasdoorLoginURL)

	req, err := http.NewRequest(http.MethodPost, "/account/casdoor/login_url", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := performRequest(router, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected apiresp error to use HTTP 200, got %d", rec.Code)
	}
	var body struct {
		ErrCode int `json:"errCode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ErrCode == 0 {
		t.Fatal("expected missing request body to produce non-zero errCode")
	}
}

func TestCasdoorLoginURLReturnsGeneratedURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := &Api{
		casdoorClient: fakeCasdoorClient{
			loginURL: "https://casdoor.example.com/login/oauth/authorize?state=fixed-state",
		},
	}
	router.POST("/account/casdoor/login_url", api.CasdoorLoginURL)

	reqBody := []byte(`{"platform":5,"client":"h5","redirectURI":"https://h5.example.com/login/casdoor/callback"}`)
	req, err := http.NewRequest(http.MethodPost, "/account/casdoor/login_url", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := performRequest(router, req)

	var body struct {
		ErrCode int `json:"errCode"`
		Data    struct {
			URL   string `json:"url"`
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ErrCode != 0 {
		t.Fatalf("expected success, got errCode %d body %s", body.ErrCode, rec.Body.String())
	}
	if body.Data.URL != "https://casdoor.example.com/login/oauth/authorize?state=fixed-state" {
		t.Fatalf("unexpected login url: %q", body.Data.URL)
	}
	if body.Data.State == "" {
		t.Fatal("expected generated state")
	}
	if _, ok := api.casdoorStates[body.Data.State]; !ok {
		t.Fatal("expected generated state to be stored")
	}
	if err := api.consumeCasdoorState(body.Data.State, "https://h5.example.com/login/casdoor/callback", "h5"); err != nil {
		t.Fatal(err)
	}
	if err := api.consumeCasdoorState(body.Data.State, "https://h5.example.com/login/casdoor/callback", "h5"); err == nil {
		t.Fatal("expected state to be single-use")
	}
}

func TestCasdoorStateRejectsRedirectMismatch(t *testing.T) {
	api := &Api{}
	api.saveCasdoorState("state-1", casdoorState{
		RedirectURI: "https://h5.example.com/login/casdoor/callback",
		Client:      "h5",
		ExpiresAt:   time.Now().Add(time.Minute),
	})

	if err := api.consumeCasdoorState("state-1", "openim://auth/callback", "h5"); err == nil {
		t.Fatal("expected redirect mismatch to fail")
	}
}

func TestCasdoorSyncRequiresSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := &Api{casdoorSyncSecret: "sync-secret"}
	router.POST("/account/casdoor/sync", api.CasdoorSync)

	reqBody := []byte(`{"profile":{"userID":"casdoor-1","disabled":true},"secret":"wrong"}`)
	req, err := http.NewRequest(http.MethodPost, "/account/casdoor/sync", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := performRequest(router, req)

	var body struct {
		ErrCode int `json:"errCode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ErrCode == 0 {
		t.Fatal("expected invalid sync secret to fail")
	}
}

func TestDefaultCasdoorClientExchangeCodeParsesJWTAccessToken(t *testing.T) {
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{
		"id":"casdoor-1",
		"name":"alice",
		"displayName":"Alice",
		"email":"alice@example.com",
		"phone":"13800138000",
		"avatar":"https://example.com/a.png",
		"isForbidden":true
	}`)) + ".sig"
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/login/oauth/access_token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "secret" || r.Form.Get("code") != "code-1" {
			t.Fatalf("unexpected token form: %#v", r.Form)
		}
		body := `{"access_token":` + strconv.Quote(token) + `,"token_type":"Bearer"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}

	casdoorClient := &defaultCasdoorClient{
		cfg: config.Casdoor{
			Enabled:      true,
			Endpoint:     "https://casdoor.example.com",
			ClientID:     "client-id",
			ClientSecret: "secret",
		},
		httpClient: httpClient,
	}

	profile, err := casdoorClient.ExchangeCode(context.Background(), "code-1", "https://h5.example.com/login/casdoor/callback")
	if err != nil {
		t.Fatal(err)
	}
	if profile.UserID != "casdoor-1" || profile.Nickname != "Alice" || profile.FaceURL != "https://example.com/a.png" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	if !profile.Disabled {
		t.Fatal("expected forbidden casdoor user to map to disabled")
	}
}

func TestDefaultCasdoorClientExchangePasswordUsesPasswordGrant(t *testing.T) {
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{
		"id":"casdoor-2",
		"name":"bob",
		"displayName":"Bob",
		"email":"bob@example.com"
	}`)) + ".sig"
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/login/oauth/access_token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "password" {
			t.Fatalf("expected password grant, got form: %#v", r.Form)
		}
		if r.Form.Get("username") != "bob@example.com" || r.Form.Get("password") != "plain-secret" {
			t.Fatalf("unexpected credentials form: %#v", r.Form)
		}
		if r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "secret" {
			t.Fatalf("unexpected client form: %#v", r.Form)
		}
		body := `{"access_token":` + strconv.Quote(token) + `,"token_type":"Bearer"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}

	casdoorClient := &defaultCasdoorClient{
		cfg: config.Casdoor{
			Enabled:      true,
			Endpoint:     "https://casdoor.example.com",
			ClientID:     "client-id",
			ClientSecret: "secret",
		},
		httpClient: httpClient,
	}

	profile, err := casdoorClient.ExchangePassword(context.Background(), "bob@example.com", "plain-secret")
	if err != nil {
		t.Fatal(err)
	}
	if profile.UserID != "casdoor-2" || profile.Nickname != "Bob" || profile.Email != "bob@example.com" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fakeCasdoorClient struct {
	loginURL string
}

func (f fakeCasdoorClient) LoginURL(state string, redirectURI string) string {
	return f.loginURL
}

func (f fakeCasdoorClient) ExchangeCode(_ context.Context, _ string, _ string) (*CasdoorProfile, error) {
	return nil, nil
}

func (f fakeCasdoorClient) ExchangePassword(_ context.Context, _ string, _ string) (*CasdoorProfile, error) {
	return nil, nil
}

func (f fakeCasdoorClient) LogoutURL(postLogoutRedirectURI string) string {
	return postLogoutRedirectURI
}

func performRequest(router http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
