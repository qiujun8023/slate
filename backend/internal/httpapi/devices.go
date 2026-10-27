package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"slate/internal/api"
	"slate/internal/blob"
	"slate/internal/store"
	"slate/internal/store/db"
)

const registerThrottle = time.Minute

var (
	macPattern      = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}$`)
	pairCodePattern = regexp.MustCompile(`^[A-Za-z0-9]{6}$`)
)

func deviceView(d db.Device) api.Device {
	return api.Device{
		ID: d.ID, MAC: d.Mac, Name: d.Name, GroupID: d.GroupID, SortOrder: d.SortOrder,
		BatteryPct: d.BatteryPct, RSSIDbm: d.RssiDbm, FWVersion: d.FwVersion, LastSeenAt: utc(d.LastSeenAt),
	}
}

func (a *API) ownedDevice(ctx context.Context, id, uid string) (db.Device, error) {
	d, err := a.Store.Q.GetDevice(ctx, id)
	if store.Missing(err) || (err == nil && (d.OwnerID == nil || *d.OwnerID != uid)) {
		return d, notFound("设备")
	}
	return d, err
}

func (a *API) listDevices(w http.ResponseWriter, r *http.Request) error {
	uid := userID(r)
	rows, err := a.Store.Q.ListDevices(r.Context(), &uid)
	if err != nil {
		return err
	}
	out := make([]api.Device, len(rows))
	for i, d := range rows {
		out[i] = deviceView(d)
	}
	return ok(w, out)
}

func (a *API) getDevice(w http.ResponseWriter, r *http.Request) error {
	d, err := a.ownedDevice(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	return ok(w, deviceView(d))
}

func (a *API) patchDevice(w http.ResponseWriter, r *http.Request) error {
	var req api.PatchDeviceRequest
	if err := decodeJSON(r, 16<<10, &req); err != nil {
		return err
	}
	uid := userID(r)
	id := r.PathValue("id")
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len([]rune(name)) > 64 {
			return badRequest("设备名称须为 1-64 个字符")
		}
		req.Name = &name
	}
	if req.GroupID.Set && req.GroupID.Value != nil {
		if _, err := a.ownedGroup(r.Context(), *req.GroupID.Value, uid); err != nil {
			return err
		}
	}
	err := a.Store.Tx(r.Context(), func(q *db.Queries) error {
		// The row lock keeps an unbind or re-register from slipping between this check
		// and the updates below.
		if _, err := q.LockOwnedDevice(r.Context(), db.LockOwnedDeviceParams{ID: id, OwnerID: &uid}); store.Missing(err) {
			return notFound("设备")
		} else if err != nil {
			return err
		}
		if req.Name != nil {
			if err := q.RenameDevice(r.Context(), db.RenameDeviceParams{ID: id, Name: req.Name}); err != nil {
				return err
			}
		}
		if req.GroupID.Set {
			return q.SetDeviceGroup(r.Context(), db.SetDeviceGroupParams{ID: id, GroupID: req.GroupID.Value})
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.getDevice(w, r)
}

func (a *API) claimDevice(w http.ResponseWriter, r *http.Request) error {
	uid := userID(r)
	if err := a.limits.check("claim:"+clientIP(r)+":"+uid, 5, "设备绑定尝试过于频繁，请稍后重试"); err != nil {
		return err
	}
	var req api.ClaimDeviceRequest
	if err := decodeJSON(r, 16<<10, &req); err != nil {
		return err
	}
	if !pairCodePattern.MatchString(req.PairCode) {
		return badRequest("配对码为 6 位字母或数字")
	}
	code := strings.ToUpper(req.PairCode)
	var claimed db.Device
	err := a.Store.Tx(r.Context(), func(q *db.Queries) error {
		d, err := q.GetDeviceByPairCode(r.Context(), code)
		if store.Missing(err) {
			return fail(404, "配对码无效")
		}
		if err != nil {
			return err
		}
		if d.OwnerID != nil {
			if *d.OwnerID == uid {
				claimed = d
				return nil
			}
			return fail(403, "设备已被他人绑定")
		}
		if _, err := q.LockUser(r.Context(), uid); err != nil {
			return err
		}
		sort, err := q.NextDeviceSortOrder(r.Context(), &uid)
		if err != nil {
			return err
		}
		next, err := uniquePairCode(r.Context(), q)
		if err != nil {
			return err
		}
		var group *string
		if first, err := q.FirstGroupID(r.Context(), uid); err == nil {
			group = &first
		} else if !store.Missing(err) {
			return err
		}
		claimed, err = q.ClaimDevice(r.Context(), db.ClaimDeviceParams{ID: d.ID, OwnerID: &uid, SortOrder: sort, PairCode: next, GroupID: group})
		if store.Missing(err) || store.Unique(err, "devices_pair_code_key") {
			return fail(409, "配对码已被使用，请查看设备屏幕上的最新配对码")
		}
		if err == nil {
			slog.Info("device claimed", "deviceId", d.ID, "userId", uid)
		}
		return err
	})
	if err != nil {
		return err
	}
	return ok(w, deviceView(claimed))
}

func (a *API) unbindDevice(w http.ResponseWriter, r *http.Request) error {
	uid := userID(r)
	id := r.PathValue("id")
	err := a.Store.Tx(r.Context(), func(q *db.Queries) error {
		// Rotating the pair code stops a leaked screenshot of the old code from claiming
		// the device; the secret stays, so the device just shows the new code.
		code, err := uniquePairCode(r.Context(), q)
		if err != nil {
			return err
		}
		n, err := q.UnbindDevice(r.Context(), db.UnbindDeviceParams{ID: id, OwnerID: &uid, PairCode: code})
		if err == nil && n == 0 {
			return notFound("设备")
		}
		return err
	})
	if err != nil {
		return err
	}
	slog.Info("device unbound", "deviceId", id, "userId", uid)
	return ok(w, nil)
}

func (a *API) reorderDevices(w http.ResponseWriter, r *http.Request) error {
	var req api.OrderRequest
	if err := decodeJSON(r, 64<<10, &req); err != nil {
		return err
	}
	uid := userID(r)
	err := a.Store.Tx(r.Context(), func(q *db.Queries) error {
		if _, err := q.LockUser(r.Context(), uid); err != nil {
			return err
		}
		devices, err := q.ListDevices(r.Context(), &uid)
		if err != nil {
			return err
		}
		ids := make([]string, len(devices))
		for i, d := range devices {
			ids[i] = d.ID
		}
		if err := validateOrder(ids, req.Order, "设备"); err != nil {
			return err
		}
		return q.SetDeviceSortOrders(r.Context(), db.SetDeviceSortOrdersParams{Ids: req.Order, OwnerID: &uid})
	})
	if err != nil {
		return err
	}
	return ok(w, nil)
}

func validateOrder(all, order []string, what string) error {
	known := make(map[string]bool, len(all))
	for _, id := range all {
		known[id] = true
	}
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		if !known[id] || seen[id] {
			return badRequest("排序列表须包含全部%s且不重复", what)
		}
		seen[id] = true
	}
	if len(order) != len(all) {
		return badRequest("排序列表须包含全部%s且不重复", what)
	}
	return nil
}

func uniquePairCode(ctx context.Context, q *db.Queries) (string, error) {
	for range 8 {
		candidates := make([]string, 8)
		for i := range candidates {
			candidates[i] = randomString(pairAlphabet, 6, false)
		}
		used, err := q.PairCodesInUse(ctx, candidates)
		if err != nil {
			return "", err
		}
		taken := map[string]bool{}
		for _, c := range used {
			taken[c] = true
		}
		for _, c := range candidates {
			if !taken[c] {
				return c, nil
			}
		}
	}
	return "", fail(409, "配对码生成冲突，请重试")
}

func checkProtocol(v int) error {
	if v != api.DeviceProtocol {
		return fail(426, "固件协议版本不匹配，请升级固件")
	}
	return nil
}

// registerDevice treats a second registration from the same MAC as a physical reset:
// owner and group are cleared and the secret and pair code rotate. Holding the device is
// the last-resort way back in when its storage is wiped or its owner account is lost; the
// one-minute throttle only stops a failed flash write from looping resets.
func (a *API) registerDevice(w http.ResponseWriter, r *http.Request) error {
	if err := a.limits.check("device-register:"+clientIP(r), 20, "设备注册过于频繁，请稍后重试"); err != nil {
		return err
	}
	var req api.DeviceRegisterRequest
	if err := decodeJSON(r, 4<<10, &req); err != nil {
		return err
	}
	if err := checkProtocol(req.Protocol); err != nil {
		return err
	}
	if !macPattern.MatchString(req.MAC) {
		return badRequest("MAC 地址格式错误")
	}
	mac := strings.ToUpper(strings.ReplaceAll(req.MAC, "-", ":"))
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	secret := hex.EncodeToString(raw)
	now := time.Now().UTC().Truncate(time.Millisecond)
	var (
		resp    api.DeviceRegisterResponse
		oldHash string
	)
	err := a.Store.Tx(r.Context(), func(q *db.Queries) error {
		code, err := uniquePairCode(r.Context(), q)
		if err != nil {
			return err
		}
		resp = api.DeviceRegisterResponse{Secret: secret, PairCode: code, ServerTime: now.UnixMilli()}
		existing, err := q.GetDeviceForRegister(r.Context(), mac)
		if store.Missing(err) {
			resp.DeviceID = newID()
			slog.Info("device registered", "deviceId", resp.DeviceID, "mac", mac)
			return q.CreateDevice(r.Context(), db.CreateDeviceParams{ID: resp.DeviceID, Mac: mac, SecretHash: hashSecret(secret), PairCode: code, RegisteredAt: now})
		}
		if err != nil {
			return err
		}
		if elapsed := now.Sub(existing.RegisteredAt); elapsed < registerThrottle {
			return &httpError{status: 409, message: "设备刚刚注册过，请稍后再重置", retryAfter: max(1, int(math.Ceil((registerThrottle - elapsed).Seconds())))}
		}
		resp.DeviceID, oldHash = existing.ID, existing.SecretHash
		slog.Info("device reset", "deviceId", existing.ID, "mac", mac, "previousOwner", existing.OwnerID)
		return q.ResetDevice(r.Context(), db.ResetDeviceParams{ID: existing.ID, SecretHash: hashSecret(secret), PairCode: code, RegisteredAt: now})
	})
	if store.Unique(err, "devices_mac_key") {
		return &httpError{status: 409, message: "设备并发注册中，请重试", retryAfter: 1}
	}
	if err != nil {
		return err
	}
	forgetDeviceSecret(oldHash)
	return ok(w, resp)
}

func (a *API) syncDevice(w http.ResponseWriter, r *http.Request) error {
	var req api.DeviceSyncRequest
	if err := decodeJSON(r, 16<<10, &req); err != nil {
		return err
	}
	if err := checkProtocol(req.Protocol); err != nil {
		return err
	}
	id := deviceID(r)
	t := req.Telemetry
	params := db.RecordTelemetryParams{ID: id, FwVersion: t.FWVersion}
	if t.BatteryPct != nil && *t.BatteryPct >= 0 && *t.BatteryPct <= 100 {
		v := int16(*t.BatteryPct)
		params.BatteryPct = &v
	}
	if t.RSSIDbm != nil && *t.RSSIDbm >= -150 && *t.RSSIDbm <= 0 {
		v := int16(*t.RSSIDbm)
		params.RssiDbm = &v
	}
	if params.FwVersion != nil && len(*params.FwVersion) > 32 {
		v := (*params.FwVersion)[:32]
		params.FwVersion = &v
	}
	if err := a.Store.Q.RecordTelemetry(r.Context(), params); err != nil {
		return err
	}
	if t.WakeReason == "timer" && t.CurrentContentID != "" {
		a.refreshCurrentFrame(r.Context(), id, t.CurrentContentID)
	}
	state, err := a.deviceState(r.Context(), id, req.ManifestVersion)
	if err != nil {
		return err
	}
	return ok(w, state)
}

// refreshCurrentFrame renders the frame a timer-woken device is showing when its refresh
// is due, so the device gets the new frame in this wake instead of the next one.
func (a *API) refreshCurrentFrame(ctx context.Context, deviceID, contentID string) {
	d, err := a.Store.Q.GetDevice(ctx, deviceID)
	if err != nil || d.GroupID == nil {
		return
	}
	c, err := a.Store.Q.GetContent(ctx, contentID)
	if err != nil || c.GroupID != *d.GroupID || c.RefreshDueAt == nil || c.RefreshDueAt.After(time.Now()) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := a.Dynamic.Render(ctx, contentID, nil); err != nil {
		slog.Warn("current frame refresh failed", "deviceId", deviceID, "contentId", contentID, "error", err.Error())
	}
}

func (a *API) changeDeviceGroup(w http.ResponseWriter, r *http.Request) error {
	var req api.DeviceGroupRequest
	if err := decodeJSON(r, 4<<10, &req); err != nil {
		return err
	}
	if err := checkProtocol(req.Protocol); err != nil {
		return err
	}
	id := deviceID(r)
	err := a.Store.Tx(r.Context(), func(q *db.Queries) error {
		d, err := q.GetDevice(r.Context(), id)
		if err != nil {
			return err
		}
		if d.OwnerID == nil {
			return fail(403, "未绑定设备无法切换内容组")
		}
		if _, err := q.LockUser(r.Context(), *d.OwnerID); err != nil {
			return err
		}
		// Re-read under the user lock: a concurrent switch, unbind or rebind may have
		// committed since the first read.
		d, err = q.LockOwnedDevice(r.Context(), db.LockOwnedDeviceParams{ID: id, OwnerID: d.OwnerID})
		if store.Missing(err) {
			return fail(409, "设备绑定已变化，请重试")
		}
		if err != nil {
			return err
		}
		ids, err := q.ListGroupIDs(r.Context(), *d.OwnerID)
		if err != nil || len(ids) == 0 {
			return err
		}
		var target string
		switch req.Action {
		case "select":
			for _, gid := range ids {
				if gid == req.GroupID {
					target = gid
				}
			}
			if target == "" {
				return fail(403, "所选内容组不属于该设备的拥有者")
			}
		case "next", "prev":
			cur := -1
			for i, gid := range ids {
				if d.GroupID != nil && gid == *d.GroupID {
					cur = i
				}
			}
			step := 1
			if req.Action == "prev" {
				step = -1
			}
			next := (cur + step + len(ids)) % len(ids)
			if cur < 0 {
				next = 0
				if step < 0 {
					next = len(ids) - 1
				}
			}
			target = ids[next]
		default:
			return badRequest("action 须为 next、prev 或 select")
		}
		return q.SetDeviceGroup(r.Context(), db.SetDeviceGroupParams{ID: id, GroupID: &target})
	})
	if err != nil {
		return err
	}
	state, err := a.deviceState(r.Context(), id, req.ManifestVersion)
	if err != nil {
		return err
	}
	return ok(w, state)
}

func (a *API) deviceState(ctx context.Context, id, knownVersion string) (api.DeviceState, error) {
	d, err := a.Store.Q.GetDevice(ctx, id)
	if err != nil {
		return api.DeviceState{}, err
	}
	state := api.DeviceState{
		ServerTime: time.Now().UnixMilli(),
		Device:     api.DeviceInfo{ID: d.ID, Name: d.Name, Bound: d.OwnerID != nil},
	}
	if d.OwnerID == nil {
		state.Device.PairCode = &d.PairCode
		return state, nil
	}
	if d.GroupID == nil {
		return state, nil
	}
	g, err := a.Store.Q.GetGroup(ctx, *d.GroupID)
	if store.Missing(err) || (err == nil && g.OwnerID != *d.OwnerID) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	ids, err := a.Store.Q.ListGroupIDs(ctx, g.OwnerID)
	if err != nil {
		return state, err
	}
	pos := api.GroupPosition{Current: 1, Total: max(1, len(ids))}
	for i, gid := range ids {
		if gid == g.ID {
			pos.Current = i + 1
		}
	}
	m, err := a.manifest(ctx, g)
	if err != nil {
		return state, err
	}
	state.Group = &api.DeviceGroup{ID: g.ID, Name: g.Name, Position: pos, ContentCount: len(m.Frames), ManifestVersion: m.Version}
	if knownVersion != m.Version {
		state.Manifest = &m
	}
	return state, nil
}

func (a *API) getDeviceBlob(w http.ResponseWriter, r *http.Request) error {
	hash := r.PathValue("hash")
	if !blob.ValidHash(hash) {
		return notFound("文件")
	}
	visible, err := a.Store.Q.BlobVisibleToDevice(r.Context(), db.BlobVisibleToDeviceParams{DeviceID: deviceID(r), Hash: hash})
	if err != nil {
		return err
	}
	return a.serveBlob(w, hash, visible)
}

func (a *API) serveBlob(w http.ResponseWriter, hash string, visible bool) error {
	if !visible {
		return notFound("文件")
	}
	data, err := a.Blobs.Read(hash)
	if err != nil {
		slog.Error("referenced blob unreadable", "hash", hash, "error", err.Error())
		return notFound("文件")
	}
	return writeBinary(w, data, "private, max-age=31536000, immutable")
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}
