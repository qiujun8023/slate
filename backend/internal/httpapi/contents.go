package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"slate/internal/api"
	"slate/internal/blob"
	"slate/internal/dynamic"
	"slate/internal/store"
	"slate/internal/store/db"
)

const (
	maxAudioBytes  = 5 << 20
	maxIngestBytes = 64 << 10
	maxTTSChars    = 500
)

func listRow(c db.GetContentRow) db.ListContentsRow {
	return db.ListContentsRow{
		ID: c.ID, GroupID: c.GroupID, SortOrder: c.SortOrder, Kind: c.Kind, FrameName: c.FrameName, ImageHash: c.ImageHash,
		DynamicType: c.DynamicType, DynamicConfig: c.DynamicConfig, DynamicData: c.DynamicData, RenderedAt: c.RenderedAt,
		NextRunAt: c.NextRunAt, RefreshDueAt: c.RefreshDueAt, DynamicError: c.DynamicError, AudioSource: c.AudioSource,
		AudioStatus: c.AudioStatus, AudioVoice: c.AudioVoice, AudioText: c.AudioText, AudioHash: c.AudioHash,
		AudioSize: c.AudioSize, AudioError: c.AudioError, AudioUpdatedAt: c.AudioUpdatedAt,
	}
}

func statusBarText(c db.ListContentsRow) string {
	return dynamic.StatusBarText(c.FrameName, c.DynamicType, c.DynamicConfig, c.DynamicData, c.RenderedAt)
}

func contentView(c db.ListContentsRow) api.Content {
	out := api.Content{
		ID: c.ID, GroupID: c.GroupID, Seq: c.SortOrder, Kind: api.ContentKind(c.Kind), FrameName: c.FrameName,
		StatusBarText: statusBarText(c), ImageHash: c.ImageHash,
	}
	if c.AudioSource != nil && c.AudioStatus != nil {
		out.Audio = &api.ContentAudio{
			Source: api.AudioSource(*c.AudioSource), Status: api.AudioStatus(*c.AudioStatus), Voice: c.AudioVoice,
			Text: c.AudioText, Hash: c.AudioHash, Size: c.AudioSize, Error: c.AudioError,
		}
		if c.AudioUpdatedAt != nil {
			out.Audio.UpdatedAt = c.AudioUpdatedAt.UTC()
		}
	}
	if c.DynamicType != nil {
		data := c.DynamicData
		if len(data) == 0 {
			data = json.RawMessage("null")
		}
		out.Dynamic = &api.ContentDynamic{
			Type: api.DynamicType(*c.DynamicType), Config: c.DynamicConfig, Data: data,
			RenderedAt: utc(c.RenderedAt), NextRunAt: utc(c.NextRunAt), Error: c.DynamicError,
		}
	}
	return out
}

// manifest lists the frames a device should hold; its version changes whenever any
// field a device shows or downloads changes.
func (a *API) manifest(ctx context.Context, g db.Group) (api.Manifest, error) {
	rows, err := a.Store.Q.ListContents(ctx, g.ID)
	if err != nil {
		return api.Manifest{}, err
	}
	frames := make([]api.ManifestFrame, len(rows))
	for i, c := range rows {
		f := api.ManifestFrame{
			ID: c.ID, Seq: c.SortOrder, StatusBarText: statusBarText(c),
			Image: api.Blob{Hash: c.ImageHash, Size: api.FrameBytes},
		}
		if c.AudioStatus != nil && *c.AudioStatus == db.AudioStatusReady && c.AudioHash != nil && c.AudioSize != nil {
			f.Audio = &api.Blob{Hash: *c.AudioHash, Size: *c.AudioSize}
		}
		if c.DynamicType != nil && c.NextRunAt != nil {
			at := c.NextRunAt.Unix()
			f.RefreshAt = &at
		}
		frames[i] = f
	}
	// The group ID is hashed too, so two groups with identical frames never share a version.
	sum, _ := json.Marshal(struct {
		GroupID string              `json:"groupId"`
		Name    string              `json:"name"`
		Frames  []api.ManifestFrame `json:"frames"`
	}{g.ID, g.Name, frames})
	return api.Manifest{Version: blob.Hash(sum)[:32], Frames: frames}, nil
}

func (a *API) ownedContent(ctx context.Context, id, uid string) (db.GetContentRow, error) {
	c, err := a.Store.Q.GetContent(ctx, id)
	if store.Missing(err) || (err == nil && c.OwnerID != uid) {
		return c, notFound("内容")
	}
	return c, err
}

func (a *API) respondContent(w http.ResponseWriter, r *http.Request, id string, status int) error {
	c, err := a.ownedContent(r.Context(), id, userID(r))
	if err != nil {
		return err
	}
	return writeJSON(w, status, api.Envelope{Success: true, Message: "操作成功", Data: contentView(listRow(c))})
}

func (a *API) listContents(w http.ResponseWriter, r *http.Request) error {
	g, err := a.ownedGroup(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	rows, err := a.Store.Q.ListContents(r.Context(), g.ID)
	if err != nil {
		return err
	}
	out := make([]api.Content, len(rows))
	for i, c := range rows {
		out[i] = contentView(c)
	}
	return ok(w, out)
}

func (a *API) getContent(w http.ResponseWriter, r *http.Request) error {
	return a.respondContent(w, r, r.PathValue("id"), 200)
}

func isMultipart(r *http.Request) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return mt == "multipart/form-data"
}

type upload struct {
	frame, audio []byte
	frameName    *string
	hasFrameName bool
}

// maxUploadBytes bounds the whole multipart body: both files plus headers and frameName.
const maxUploadBytes = api.FrameBytes + maxAudioBytes + 64<<10

func readUpload(w http.ResponseWriter, r *http.Request) (upload, error) {
	var u upload
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		return u, badRequest("请求格式错误，请使用 multipart/form-data")
	}
	seen := map[string]bool{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return u, nil
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return u, fail(413, "上传内容过大")
		}
		if err != nil {
			return u, badRequest("multipart 请求体无效")
		}
		name := part.FormName()
		if seen[name] {
			return u, badRequest("字段 %s 重复", name)
		}
		seen[name] = true
		switch name {
		case "frame":
			// The browser renders and dithers the frame; the server stores it as is.
			if u.frame, err = readPart(part, api.FrameBytes, "帧数据须为 15000 字节的 1bpp 位图"); err != nil {
				return u, err
			}
			if len(u.frame) != api.FrameBytes {
				return u, badRequest("帧数据须为 15000 字节的 1bpp 位图")
			}
		case "audio":
			// Browsers often send an empty or generic type for flac/opus/wma; ffmpeg decides.
			if u.audio, err = readPart(part, maxAudioBytes, "音频文件不能超过 5MB"); err != nil {
				return u, err
			}
		case "frameName":
			v, err := readPart(part, 1024, "frameName 过长")
			if err != nil {
				return u, err
			}
			u.hasFrameName = true
			if name := truncateRunes(strings.TrimSpace(string(v)), 64); name != "" {
				u.frameName = &name
			}
		default:
			return u, badRequest("未知字段 %s", name)
		}
	}
}

func readPart(p *multipart.Part, limit int64, message string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(p, limit+1))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return nil, fail(413, "上传内容过大")
	}
	if err != nil {
		return nil, badRequest("读取上传内容失败")
	}
	if int64(len(data)) > limit {
		return nil, fail(413, "%s", message)
	}
	return data, nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// storeUpload stores the frame and transcodes the audio, returning blob hashes.
func (a *API) storeUpload(ctx context.Context, u upload) (imageHash string, audioHash string, audioSize int32, err error) {
	if len(u.frame) > 0 {
		if imageHash, err = a.Blobs.Put(u.frame); err != nil {
			return "", "", 0, err
		}
	}
	if len(u.audio) > 0 {
		pcm, err := a.Media.Transcode(ctx, u.audio)
		if err != nil {
			return "", "", 0, err
		}
		if audioHash, err = a.Blobs.Put(pcm); err != nil {
			return "", "", 0, err
		}
		audioSize = int32(len(pcm))
	}
	return imageHash, audioHash, audioSize, nil
}

func (a *API) createContent(w http.ResponseWriter, r *http.Request) error {
	g, err := a.ownedGroup(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	if !isMultipart(r) {
		return a.createDynamic(w, r, g)
	}
	u, err := readUpload(w, r)
	if err != nil {
		return err
	}
	if len(u.frame) == 0 {
		return badRequest("请上传图片")
	}
	imageHash, audioHash, audioSize, err := a.storeUpload(r.Context(), u)
	if err != nil {
		return err
	}
	id := newID()
	err = a.Store.Tx(r.Context(), func(q *db.Queries) error {
		seq, err := lockNextSeq(r.Context(), q, g.ID)
		if err != nil {
			return err
		}
		if err := q.CreateContent(r.Context(), db.CreateContentParams{ID: id, GroupID: g.ID, SortOrder: seq, Kind: db.ContentKindImage, FrameName: u.frameName, ImageHash: imageHash}); err != nil {
			return err
		}
		if audioHash == "" {
			return nil
		}
		return q.SetAudio(r.Context(), db.SetAudioParams{ContentID: id, Source: db.AudioSourceUpload, Status: db.AudioStatusReady, Hash: &audioHash, Size: &audioSize})
	})
	if err != nil {
		return err
	}
	return a.respondContent(w, r, id, 201)
}

func lockNextSeq(ctx context.Context, q *db.Queries, groupID string) (int32, error) {
	if _, err := q.LockGroup(ctx, groupID); err != nil {
		return 0, err
	}
	return q.NextContentSortOrder(ctx, groupID)
}

func (a *API) createDynamic(w http.ResponseWriter, r *http.Request, g db.Group) error {
	var req api.CreateDynamicContentRequest
	if err := decodeJSON(r, maxIngestBytes*2, &req); err != nil {
		return err
	}
	cfg, err := dynamic.NormalizeConfig(req.Config)
	if err != nil {
		return err
	}
	if cfg.Type == api.DynamicTypeDashboard {
		if err := dynamic.ValidateDashboardData(req.InitialData); err != nil {
			return err
		}
	} else {
		req.InitialData = nil
	}
	frameName := req.FrameName
	if frameName == nil {
		frameName = dynamic.DefaultFrameName(cfg)
	}
	frame, err := a.Dynamic.Prepare(r.Context(), cfg, req.InitialData, nil, time.Now())
	if err != nil {
		return err
	}
	imageHash, err := a.Blobs.Put(frame.Image)
	if err != nil {
		return err
	}
	cfgJSON, _ := json.Marshal(cfg)
	id := newID()
	err = a.Store.Tx(r.Context(), func(q *db.Queries) error {
		seq, err := lockNextSeq(r.Context(), q, g.ID)
		if err != nil {
			return err
		}
		if err := q.CreateContent(r.Context(), db.CreateContentParams{ID: id, GroupID: g.ID, SortOrder: seq, Kind: db.ContentKindDynamic, FrameName: frameName, ImageHash: imageHash}); err != nil {
			return err
		}
		if err := q.CreateDynamic(r.Context(), db.CreateDynamicParams{ContentID: id, Type: string(cfg.Type), Config: cfgJSON, Data: frame.Data}); err != nil {
			return err
		}
		return q.SaveDynamicRender(r.Context(), frame.SaveParams(id))
	})
	if err != nil {
		return err
	}
	a.Dynamic.Wake()
	a.Dynamic.SyncAudio(r.Context(), id)
	return a.respondContent(w, r, id, 201)
}

func (a *API) patchContent(w http.ResponseWriter, r *http.Request) error {
	c, err := a.ownedContent(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	if isMultipart(r) {
		return a.patchImage(w, r, c)
	}
	var req api.PatchContentRequest
	if err := decodeJSON(r, maxIngestBytes*2, &req); err != nil {
		return err
	}
	if !req.FrameName.Set && len(req.Config) == 0 {
		return badRequest("没有可更新的字段")
	}
	if req.FrameName.Set && req.FrameName.Value != nil {
		name := truncateRunes(strings.TrimSpace(*req.FrameName.Value), 64)
		req.FrameName.Value = &name
		if name == "" {
			req.FrameName.Value = nil
		}
	}
	if c.Kind == db.ContentKindImage {
		if len(req.Config) > 0 {
			return badRequest("图片内容不支持动态配置")
		}
		if err := a.Store.Q.SetContentFrameName(r.Context(), db.SetContentFrameNameParams{ID: c.ID, FrameName: req.FrameName.Value}); err != nil {
			return err
		}
		return a.respondContent(w, r, c.ID, 200)
	}
	// Render the new config before saving anything, so a failed fetch or render leaves
	// the content as it was.
	err = a.Dynamic.Mutate(r.Context(), c.ID, func() error {
		// Re-read under the lock: a push or scheduled render may have saved since.
		c, err := a.ownedContent(r.Context(), c.ID, userID(r))
		if err != nil {
			return err
		}
		frameName := c.FrameName
		if req.FrameName.Set {
			frameName = req.FrameName.Value
		}
		if len(req.Config) == 0 {
			return a.Store.Q.SetContentFrameName(r.Context(), db.SetContentFrameNameParams{ID: c.ID, FrameName: frameName})
		}
		cfg, err := dynamic.NormalizeConfig(req.Config)
		if err != nil {
			return err
		}
		if c.DynamicType == nil || string(cfg.Type) != *c.DynamicType {
			return badRequest("不能修改动态内容的类型，请删除后重建")
		}
		if !req.FrameName.Set && cfg.Type != api.DynamicTypeDashboard {
			frameName = dynamic.DefaultFrameName(cfg)
		}
		var lastData json.RawMessage
		if old, err := dynamic.NormalizeConfig(c.DynamicConfig); err == nil && dynamic.SameSource(old, cfg) {
			lastData = c.DynamicData
		}
		frame, err := a.Dynamic.Prepare(r.Context(), cfg, nil, lastData, time.Now())
		if err != nil {
			return err
		}
		imageHash, err := a.Blobs.Put(frame.Image)
		if err != nil {
			return err
		}
		cfgJSON, _ := json.Marshal(cfg)
		return a.Store.Tx(r.Context(), func(q *db.Queries) error {
			if err := q.UpdateDynamicConfig(r.Context(), db.UpdateDynamicConfigParams{ContentID: c.ID, Config: cfgJSON}); err != nil {
				return err
			}
			if err := q.SetContentFrameName(r.Context(), db.SetContentFrameNameParams{ID: c.ID, FrameName: frameName}); err != nil {
				return err
			}
			if imageHash != c.ImageHash {
				if err := q.SetContentImage(r.Context(), db.SetContentImageParams{ID: c.ID, ImageHash: imageHash}); err != nil {
					return err
				}
			}
			return q.SaveDynamicRender(r.Context(), frame.SaveParams(c.ID))
		})
	})
	if err != nil {
		return err
	}
	if len(req.Config) > 0 {
		a.Dynamic.Wake()
		a.Dynamic.SyncAudio(r.Context(), c.ID)
	}
	return a.respondContent(w, r, c.ID, 200)
}

func (a *API) patchImage(w http.ResponseWriter, r *http.Request, c db.GetContentRow) error {
	if c.Kind != db.ContentKindImage {
		return badRequest("动态内容请使用 JSON 更新")
	}
	u, err := readUpload(w, r)
	if err != nil {
		return err
	}
	if len(u.frame) == 0 && len(u.audio) == 0 && !u.hasFrameName {
		return badRequest("没有可更新的字段")
	}
	imageHash, audioHash, audioSize, err := a.storeUpload(r.Context(), u)
	if err != nil {
		return err
	}
	err = a.Store.Tx(r.Context(), func(q *db.Queries) error {
		if u.hasFrameName {
			if err := q.SetContentFrameName(r.Context(), db.SetContentFrameNameParams{ID: c.ID, FrameName: u.frameName}); err != nil {
				return err
			}
		}
		if imageHash != "" {
			if err := q.SetContentImage(r.Context(), db.SetContentImageParams{ID: c.ID, ImageHash: imageHash}); err != nil {
				return err
			}
		}
		// A new picture keeps the existing audio; DELETE /audio is the only way to drop it.
		if audioHash != "" {
			return q.SetAudio(r.Context(), db.SetAudioParams{ContentID: c.ID, Source: db.AudioSourceUpload, Status: db.AudioStatusReady, Hash: &audioHash, Size: &audioSize})
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.respondContent(w, r, c.ID, 200)
}

func (a *API) deleteContent(w http.ResponseWriter, r *http.Request) error {
	c, err := a.ownedContent(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	err = a.Store.Tx(r.Context(), func(q *db.Queries) error {
		if _, err := q.LockGroup(r.Context(), c.GroupID); err != nil {
			return err
		}
		if err := q.DeleteContent(r.Context(), c.ID); err != nil {
			return err
		}
		return q.CompactContentSortOrders(r.Context(), c.GroupID)
	})
	if err != nil {
		return err
	}
	return ok(w, nil)
}

func (a *API) reorderContents(w http.ResponseWriter, r *http.Request) error {
	g, err := a.ownedGroup(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	var req api.OrderRequest
	if err := decodeJSON(r, 64<<10, &req); err != nil {
		return err
	}
	err = a.Store.Tx(r.Context(), func(q *db.Queries) error {
		if _, err := q.LockGroup(r.Context(), g.ID); err != nil {
			return err
		}
		ids, err := q.ListContentIDs(r.Context(), g.ID)
		if err != nil {
			return err
		}
		if err := validateOrder(ids, req.Order, "内容"); err != nil {
			return err
		}
		return q.SetContentSortOrders(r.Context(), db.SetContentSortOrdersParams{Ids: req.Order, GroupID: g.ID})
	})
	if err != nil {
		return err
	}
	return ok(w, nil)
}

func (a *API) deleteAudio(w http.ResponseWriter, r *http.Request) error {
	c, err := a.ownedContent(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	if c.Kind == db.ContentKindDynamic {
		return badRequest("动态内容的语音在配置中关闭")
	}
	if _, err := a.Store.Q.DeleteAudio(r.Context(), c.ID); err != nil {
		return err
	}
	return a.respondContent(w, r, c.ID, 200)
}

func (a *API) generateTTS(w http.ResponseWriter, r *http.Request) error {
	c, err := a.ownedContent(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	if c.Kind != db.ContentKindImage {
		return badRequest("只有图片内容支持手动输入 TTS 文案")
	}
	var req api.GenerateTTSRequest
	if err := decodeJSON(r, 16<<10, &req); err != nil {
		return err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" || len([]rune(text)) > maxTTSChars {
		return badRequest("TTS 文案须为 1-%d 字", maxTTSChars)
	}
	if !api.IsTTSVoice(req.Voice) {
		return badRequest("未知音色: %s", req.Voice)
	}
	if err := a.Store.Q.SetAudio(r.Context(), db.SetAudioParams{ContentID: c.ID, Source: db.AudioSourceTts, Status: db.AudioStatusPending, Voice: &req.Voice, Text: &text}); err != nil {
		return err
	}
	return a.respondContent(w, r, c.ID, 200)
}

func (a *API) refreshContent(w http.ResponseWriter, r *http.Request) error {
	c, err := a.ownedContent(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	if c.Kind != db.ContentKindDynamic {
		return badRequest("非动态内容不支持手动刷新")
	}
	if _, err := a.Dynamic.Render(r.Context(), c.ID, nil); err != nil {
		return err
	}
	return a.respondContent(w, r, c.ID, 200)
}

func (a *API) previewDynamic(w http.ResponseWriter, r *http.Request) error {
	var req api.PreviewDynamicRequest
	if err := decodeJSON(r, maxIngestBytes*2, &req); err != nil {
		return err
	}
	cfg, err := dynamic.NormalizeConfig(req.Config)
	if err != nil {
		return err
	}
	if cfg.Type == api.DynamicTypeDashboard {
		if err := dynamic.ValidateDashboardData(req.Data); err != nil {
			return err
		}
	} else {
		req.Data = nil
	}
	frame, err := a.Dynamic.Prepare(r.Context(), cfg, req.Data, nil, time.Now())
	if err != nil {
		return err
	}
	return writeBinary(w, frame.Image, "no-store")
}

func (a *API) previewDynamicContent(w http.ResponseWriter, r *http.Request) error {
	c, err := a.ownedContent(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	if c.DynamicType == nil {
		return badRequest("该内容不是动态类型")
	}
	var req api.PreviewDynamicRequest
	if err := decodeJSON(r, maxIngestBytes*2, &req); err != nil {
		return err
	}
	cfg, err := dynamic.NormalizeConfig(req.Config)
	if err != nil {
		return err
	}
	if string(cfg.Type) != *c.DynamicType {
		return badRequest("预览配置与内容类型不一致")
	}
	if len(req.Data) > 0 {
		if cfg.Type != api.DynamicTypeDashboard {
			return badRequest("只有外部数据预览支持 data 参数")
		}
		if err := dynamic.ValidateDashboardData(req.Data); err != nil {
			return err
		}
	}
	var lastData json.RawMessage
	if old, err := dynamic.NormalizeConfig(c.DynamicConfig); err == nil && dynamic.SameSource(old, cfg) {
		lastData = c.DynamicData
	}
	frame, err := a.Dynamic.Prepare(r.Context(), cfg, req.Data, lastData, time.Now())
	if err != nil {
		return err
	}
	return writeBinary(w, frame.Image, "no-store")
}

// ingest accepts dashboard data. The content ID is an unguessable capability URL, which
// keeps scripts simple; abuse is bounded by the body limit and a per-content rate limit.
func (a *API) ingest(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := a.limits.check("ingest:"+id, 30, "每分钟最多 30 次推送"); err != nil {
		return err
	}
	var req api.IngestRequest
	if err := decodeJSON(r, maxIngestBytes, &req); err != nil {
		return err
	}
	if req.Version != 1 {
		return badRequest("version 须为 1")
	}
	data, _ := json.Marshal(req.Data)
	if err := dynamic.ValidateDashboardData(data); err != nil {
		return err
	}
	c, err := a.Store.Q.GetContent(r.Context(), id)
	if store.Missing(err) || (err == nil && (c.DynamicType == nil || *c.DynamicType != string(api.DynamicTypeDashboard))) {
		return notFound("dashboard 内容")
	}
	if err != nil {
		return err
	}
	res, err := a.Dynamic.Render(r.Context(), id, data)
	if err != nil {
		return err
	}
	return ok(w, api.IngestResponse{ID: id, ImageHash: res.ImageHash, RenderedAt: res.RenderedAt.UTC()})
}

func (a *API) getBlob(w http.ResponseWriter, r *http.Request) error {
	hash := r.PathValue("hash")
	if !blob.ValidHash(hash) {
		return notFound("文件")
	}
	visible, err := a.Store.Q.BlobVisibleToUser(r.Context(), db.BlobVisibleToUserParams{UserID: userID(r), Hash: hash})
	if err != nil {
		return err
	}
	return a.serveBlob(w, hash, visible)
}

func (a *API) searchCities(w http.ResponseWriter, r *http.Request) error {
	if err := a.limits.check("weather-city:"+clientIP(r), 30, "城市搜索过于频繁，请稍后重试"); err != nil {
		return err
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return ok(w, []api.WeatherCity{})
	}
	if len([]rune(q)) > 32 {
		return badRequest("城市搜索关键词最多 32 个字符")
	}
	cities, err := a.Dynamic.SearchCities(r.Context(), q)
	if err != nil {
		return err
	}
	return ok(w, cities)
}
