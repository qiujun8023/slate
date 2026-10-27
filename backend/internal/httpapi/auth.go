package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"slate/internal/api"
	"slate/internal/store"
	"slate/internal/store/db"
)

const (
	passwordCost       = 12
	deviceCacheTTL     = 30 * time.Second
	deviceMissTTL      = 5 * time.Second
	deviceCacheMax     = 10000
	maxTokenChars      = 4096
	timingSafeFallback = "$2b$12$CuSHZRvMs3E1H4thc835jOrgn1306YQVah/7ePhzdlkVSZcniq/Am"
)

var (
	secretPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
	emailPattern    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

func (a *API) authenticate(r *http.Request, mode auth) (*http.Request, error) {
	if mode == public {
		return r, nil
	}
	token := bearer(r)
	if mode == userAuth {
		if uid := a.verifyToken(token); uid != "" {
			return r.WithContext(context.WithValue(r.Context(), userKey, uid)), nil
		}
	}
	if mode == deviceAuth && secretPattern.MatchString(token) {
		id, err := a.deviceBySecret(r.Context(), token)
		if err != nil {
			return r, err
		}
		if id != "" {
			return r.WithContext(context.WithValue(r.Context(), deviceKey, id)), nil
		}
	}
	if mode == deviceAuth {
		return r, fail(401, "设备认证失败")
	}
	return r, fail(401, "未登录或登录已过期")
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return ""
	}
	t := strings.TrimSpace(h[7:])
	if len(t) > maxTokenChars {
		return ""
	}
	return t
}

func (a *API) signToken(uid string) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   uid,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(a.Config.JWTExpiration)),
	}).SignedString([]byte(a.Config.JWTSecret))
}

func (a *API) verifyToken(token string) string {
	if token == "" {
		return ""
	}
	claims := jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return []byte(a.Config.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid {
		return ""
	}
	return claims.Subject
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

type deviceCacheEntry struct {
	id      string
	expires time.Time
}

var (
	deviceCacheMu sync.Mutex
	deviceCache   = map[string]deviceCacheEntry{}
)

// deviceBySecret caches lookups for 30s and misses for 5s; registration evicts the
// replaced secret.
func (a *API) deviceBySecret(ctx context.Context, secret string) (string, error) {
	h := hashSecret(secret)
	now := time.Now()
	deviceCacheMu.Lock()
	entry, hit := deviceCache[h]
	deviceCacheMu.Unlock()
	if hit && now.Before(entry.expires) {
		return entry.id, nil
	}
	id, err := a.Store.Q.GetDeviceIDBySecret(ctx, h)
	ttl := deviceCacheTTL
	if store.Missing(err) {
		id, err, ttl = "", nil, deviceMissTTL
	}
	if err != nil {
		return "", err
	}
	deviceCacheMu.Lock()
	if len(deviceCache) >= deviceCacheMax {
		for k, v := range deviceCache {
			if now.After(v.expires) || len(deviceCache) >= deviceCacheMax {
				delete(deviceCache, k)
			}
		}
	}
	deviceCache[h] = deviceCacheEntry{id, now.Add(ttl)}
	deviceCacheMu.Unlock()
	return id, nil
}

func forgetDeviceSecret(secretHash string) {
	deviceCacheMu.Lock()
	delete(deviceCache, secretHash)
	deviceCacheMu.Unlock()
}

func (a *API) register(w http.ResponseWriter, r *http.Request) error {
	if err := a.limits.check("register:"+clientIP(r), 5, "请求过于频繁，请稍后重试"); err != nil {
		return err
	}
	var req api.RegisterRequest
	if err := decodeJSON(r, 16<<10, &req); err != nil {
		return err
	}
	req.Email = strings.TrimSpace(req.Email)
	switch {
	case !emailPattern.MatchString(req.Email) || len(req.Email) > 254:
		return badRequest("邮箱格式不正确")
	case !usernamePattern.MatchString(req.Username):
		return badRequest("用户名只能包含字母、数字、下划线，3-32 位")
	case len(req.Password) < 8 || len(req.Password) > 72:
		return badRequest("密码长度须为 8-72 位")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), passwordCost)
	if err != nil {
		return err
	}
	u, err := a.Store.Q.CreateUser(r.Context(), db.CreateUserParams{ID: newID(), Email: req.Email, Username: &req.Username, PasswordHash: string(hash)})
	switch {
	case store.Unique(err, "users_email_key"):
		return fail(409, "该邮箱已被注册")
	case store.Unique(err, "users_username_key"):
		return fail(409, "该用户名已被占用")
	case err != nil:
		return err
	}
	return a.session(w, 201, api.User{ID: u.ID, Email: u.Email, Username: u.Username})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) error {
	if err := a.limits.check("login:"+clientIP(r), 10, "请求过于频繁，请稍后重试"); err != nil {
		return err
	}
	var req api.LoginRequest
	if err := decodeJSON(r, 16<<10, &req); err != nil {
		return err
	}
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" || req.Password == "" {
		return badRequest("请输入账号和密码")
	}
	var (
		u   db.GetUserByEmailRow
		err error
	)
	if strings.Contains(identifier, "@") {
		u, err = a.Store.Q.GetUserByEmail(r.Context(), identifier)
	} else {
		var row db.GetUserByUsernameRow
		row, err = a.Store.Q.GetUserByUsername(r.Context(), &identifier)
		u = db.GetUserByEmailRow(row)
	}
	if err != nil && !store.Missing(err) {
		return err
	}
	hash := u.PasswordHash
	if hash == "" {
		hash = timingSafeFallback
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil || u.ID == "" {
		return fail(401, "账号或密码错误")
	}
	return a.session(w, 201, api.User{ID: u.ID, Email: u.Email, Username: u.Username})
}

func (a *API) session(w http.ResponseWriter, status int, u api.User) error {
	token, err := a.signToken(u.ID)
	if err != nil {
		return err
	}
	return writeJSON(w, status, api.Envelope{Success: true, Message: "操作成功", Data: api.Session{Token: token, User: u}})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) error {
	u, err := a.Store.Q.GetUser(r.Context(), userID(r))
	if store.Missing(err) {
		return fail(401, "未登录或登录已过期")
	}
	if err != nil {
		return err
	}
	return ok(w, api.User{ID: u.ID, Email: u.Email, Username: u.Username})
}
