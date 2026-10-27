// Package legacyimport copies a v0.1.x deployment (MySQL plus per-content blob files)
// into an empty PostgreSQL database. It is a one-time upgrade path; remove it in the next
// major version.
package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"slate/internal/api"
	"slate/internal/blob"
	"slate/internal/dynamic"
	"slate/internal/store"
	"slate/internal/store/db"
)

// Hot-list sources renamed before v1.
var sourceAliases = map[string]string{
	"baidutieba": "tieba", "hostloc": "nodeseek", "bilibili-hot-video": "bilibili", "chongbuluo": "chongbuluo-latest",
	"cls": "cls-telegraph", "iqiyi": "iqiyi-hot-ranklist", "mktnews": "mktnews-flash", "pcbeta": "pcbeta-windows11",
	"tencent": "tencent-hot", "wallstreetcn": "wallstreetcn-quick", "xueqiu": "xueqiu-hotstock", "qqvideo": "qqvideo-tv-hotsearch",
}

type Stats struct{ Users, Groups, Devices, Contents, Skipped int }

// Run imports mysqlURL (mysql://user:pass@host:3306/db) and the blob files under
// legacyBlobDir. The target must have no users; every row is written in one transaction.
func Run(ctx context.Context, mysqlURL, legacyBlobDir string, st *store.Store, blobs *blob.Store) (Stats, error) {
	var stats Stats
	dsn, err := mysqlDSN(mysqlURL)
	if err != nil {
		return stats, err
	}
	src, err := sql.Open("mysql", dsn)
	if err != nil {
		return stats, err
	}
	defer src.Close()
	if err := src.PingContext(ctx); err != nil {
		return stats, fmt.Errorf("connect MySQL: %w", err)
	}
	var existing int
	if err := st.Pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&existing); err != nil {
		return stats, err
	}
	if existing > 0 {
		return stats, errors.New("target database already has users; import needs an empty database")
	}
	err = st.Tx(ctx, func(q *db.Queries) error {
		users, err := importUsers(ctx, src, q)
		if err != nil {
			return err
		}
		stats.Users = len(users)
		groups, err := importGroups(ctx, src, q, users)
		if err != nil {
			return err
		}
		stats.Groups = len(groups)
		if stats.Devices, err = importDevices(ctx, src, q, users, groups); err != nil {
			return err
		}
		stats.Contents, stats.Skipped, err = importContents(ctx, src, q, groups, legacyBlobDir, blobs)
		return err
	})
	return stats, err
}

func mysqlDSN(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "mysql" {
		return "", errors.New("legacy database must be a mysql://user:password@host:3306/db URL")
	}
	cfg := mysql.NewConfig()
	cfg.User = u.User.Username()
	cfg.Passwd, _ = u.User.Password()
	cfg.Net, cfg.Addr = "tcp", u.Host
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	cfg.ParseTime, cfg.Loc = true, time.UTC
	return cfg.FormatDSN(), nil
}

func importUsers(ctx context.Context, src *sql.DB, q *db.Queries) (map[string]bool, error) {
	rows, err := src.QueryContext(ctx, "SELECT id, email, username, password, created_at, updated_at FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id, email, password string
		var username sql.NullString
		var created, updated time.Time
		if err := rows.Scan(&id, &email, &username, &password, &created, &updated); err != nil {
			return nil, err
		}
		if err := q.ImportUser(ctx, db.ImportUserParams{ID: id, Email: email, Username: nullable(username), PasswordHash: password, CreatedAt: created, UpdatedAt: updated}); err != nil {
			return nil, fmt.Errorf("user %s: %w", id, err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

func importGroups(ctx context.Context, src *sql.DB, q *db.Queries, users map[string]bool) (map[string]bool, error) {
	rows, err := src.QueryContext(ctx, "SELECT id, name, owner_user_id, sort_order, created_at, updated_at FROM `groups` ORDER BY owner_user_id, sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id, name string
		var owner sql.NullString
		var sort int32
		var created, updated time.Time
		if err := rows.Scan(&id, &name, &owner, &sort, &created, &updated); err != nil {
			return nil, err
		}
		if !owner.Valid || !users[owner.String] {
			slog.Warn("group without owner skipped", "groupId", id)
			continue
		}
		if err := q.ImportGroup(ctx, db.ImportGroupParams{ID: id, OwnerID: owner.String, Name: name, SortOrder: sort, CreatedAt: created, UpdatedAt: updated}); err != nil {
			return nil, fmt.Errorf("group %s: %w", id, err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

func importDevices(ctx context.Context, src *sql.DB, q *db.Queries, users, groups map[string]bool) (int, error) {
	rows, err := src.QueryContext(ctx, `SELECT id, mac, secret_hash, pair_code, name, owner_user_id, sort_order, selected_group_id,
		last_registered_at, last_seen_at, battery_pct, rssi_dbm, fw_version, created_at, updated_at FROM devices`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var p db.ImportDeviceParams
		var name, owner, group, fw sql.NullString
		var registered, seen sql.NullTime
		var battery, rssi sql.NullInt16
		if err := rows.Scan(&p.ID, &p.Mac, &p.SecretHash, &p.PairCode, &name, &owner, &p.SortOrder, &group,
			&registered, &seen, &battery, &rssi, &fw, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return n, err
		}
		p.Name, p.FwVersion = nullable(name), nullable(fw)
		if owner.Valid && users[owner.String] {
			p.OwnerID = &owner.String
			if group.Valid && groups[group.String] {
				p.GroupID = &group.String
			}
		} else {
			p.SortOrder = 0
		}
		p.RegisteredAt = p.CreatedAt
		if registered.Valid {
			p.RegisteredAt = registered.Time
		}
		if seen.Valid {
			p.LastSeenAt = &seen.Time
		}
		if battery.Valid {
			p.BatteryPct = &battery.Int16
		}
		if rssi.Valid {
			p.RssiDbm = &rssi.Int16
		}
		if err := q.ImportDevice(ctx, p); err != nil {
			return n, fmt.Errorf("device %s: %w", p.ID, err)
		}
		n++
	}
	return n, rows.Err()
}

func importContents(ctx context.Context, src *sql.DB, q *db.Queries, groups map[string]bool, blobDir string, blobs *blob.Store) (int, int, error) {
	rows, err := src.QueryContext(ctx, `SELECT id, group_id, sort_order, kind, frame_name, audio_etag, audio_status, audio_source,
		audio_voice, audio_text, audio_last_error, dynamic_type, dynamic_config, dynamic_data, dynamic_last_run_at, created_at, updated_at
		FROM contents ORDER BY group_id, sort_order`)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	imported, skipped := 0, 0
	for rows.Next() {
		var (
			id, groupID, kind, audioStatus                                        string
			sort                                                                  int32
			frameName, audioEtag, audioSource, voice, text, audioErr, dynamicType sql.NullString
			config, data                                                          []byte
			renderedAt                                                            sql.NullTime
			created, updated                                                      time.Time
		)
		if err := rows.Scan(&id, &groupID, &sort, &kind, &frameName, &audioEtag, &audioStatus, &audioSource, &voice, &text,
			&audioErr, &dynamicType, &config, &data, &renderedAt, &created, &updated); err != nil {
			return imported, skipped, err
		}
		if !groups[groupID] {
			skipped++
			continue
		}
		image, err := os.ReadFile(filepath.Join(blobDir, groupID, id+".img"))
		if err != nil && kind == "image" {
			slog.Warn("image content skipped: frame file missing", "contentId", id, "error", err.Error())
			skipped++
			continue
		}
		if err != nil {
			// The scheduler re-renders dynamic content right after import.
			image = blankFrame()
		}
		hash, err := blobs.Put(image)
		if err != nil {
			return imported, skipped, err
		}
		if err := q.ImportContent(ctx, db.ImportContentParams{ID: id, GroupID: groupID, SortOrder: sort, Kind: db.ContentKind(kind),
			FrameName: nullable(frameName), ImageHash: hash, CreatedAt: created, UpdatedAt: updated}); err != nil {
			return imported, skipped, fmt.Errorf("content %s: %w", id, err)
		}
		if kind == "dynamic" && dynamicType.Valid {
			cfg, err := convertConfig(config)
			if err != nil {
				slog.Warn("dynamic config kept unnormalized", "contentId", id, "error", err.Error())
			}
			var keep json.RawMessage
			if dynamicType.String == string(api.DynamicTypeDashboard) && len(data) > 0 {
				keep = data
			}
			var at *time.Time
			if renderedAt.Valid {
				at = &renderedAt.Time
			}
			if err := q.ImportDynamic(ctx, db.ImportDynamicParams{ContentID: id, Type: dynamicType.String, Config: cfg, Data: keep, RenderedAt: at}); err != nil {
				return imported, skipped, fmt.Errorf("content %s dynamic: %w", id, err)
			}
		}
		if err := importAudio(ctx, q, id, groupID, blobDir, blobs, audioEtag, audioStatus, audioSource, voice, text, audioErr); err != nil {
			return imported, skipped, fmt.Errorf("content %s audio: %w", id, err)
		}
		imported++
	}
	return imported, skipped, rows.Err()
}

func importAudio(ctx context.Context, q *db.Queries, id, groupID, blobDir string, blobs *blob.Store, etag sql.NullString, status string, source, voice, text, lastErr sql.NullString) error {
	if status == "none" || !source.Valid {
		return nil
	}
	p := db.SetAudioParams{ContentID: id, Source: db.AudioSource(source.String), Status: db.AudioStatus(status), Voice: nullable(voice), Text: nullable(text)}
	if status == "ready" {
		pcm, err := os.ReadFile(filepath.Join(blobDir, groupID, id+"."+etag.String+".pcm"))
		if err == nil {
			hash, err := blobs.Put(pcm)
			if err != nil {
				return err
			}
			size := int32(len(pcm))
			p.Hash, p.Size = &hash, &size
		} else {
			p.Status = db.AudioStatusPending
		}
	}
	if p.Source == db.AudioSourceUpload && p.Status != db.AudioStatusReady {
		return nil
	}
	if p.Source == db.AudioSourceTts && (p.Voice == nil || p.Text == nil) {
		return nil
	}
	if p.Status == db.AudioStatusGenerating {
		p.Status = db.AudioStatusPending
	}
	if err := q.SetAudio(ctx, p); err != nil {
		return err
	}
	if p.Status == db.AudioStatusFailed && lastErr.Valid {
		return q.SetAudioError(ctx, db.SetAudioErrorParams{ContentID: id, LastError: &lastErr.String})
	}
	return nil
}

var snakeKey = regexp.MustCompile(`_([a-z0-9])`)

// convertConfig renames snake_case keys to camelCase, maps renamed hot-list sources and
// normalizes the result. On failure the renamed config is stored unnormalized.
func convertConfig(raw []byte) (json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage(`{}`), err
	}
	var camel func(any) any
	camel = func(v any) any {
		switch t := v.(type) {
		case map[string]any:
			out := make(map[string]any, len(t))
			for k, val := range t {
				out[snakeKey.ReplaceAllStringFunc(k, func(m string) string { return strings.ToUpper(m[1:]) })] = camel(val)
			}
			return out
		case []any:
			for i := range t {
				t[i] = camel(t[i])
			}
		}
		return v
	}
	m, _ := camel(v).(map[string]any)
	delete(m, "provider")
	if m["type"] == string(api.DynamicTypeHotList) {
		if s, ok := m["source"].(string); ok && sourceAliases[s] != "" {
			m["source"] = sourceAliases[s]
		}
	}
	renamed, _ := json.Marshal(m)
	cfg, err := dynamic.NormalizeConfig(renamed)
	if err != nil {
		return renamed, err
	}
	out, _ := json.Marshal(cfg)
	return out, nil
}

func blankFrame() []byte {
	b := make([]byte, api.FrameBytes)
	for i := range b {
		b[i] = 0xff
	}
	return b
}

func nullable(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
