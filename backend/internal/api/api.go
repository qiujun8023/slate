// Package api holds the HTTP request and response types. `go generate ./internal/api`
// writes frontend/src/api/generated.ts (tygo) and frontend/src/api/catalog.json.
package api

//go:generate go run github.com/gzuidhof/tygo@v0.2.21 generate --config ../../tygo.yaml
//go:generate go run ../../cmd/gencatalog ../../../frontend/src/api/catalog.json

import (
	"encoding/json"
	"time"
)

const (
	FrameWidth  = 400
	FrameHeight = 300
	FrameBytes  = FrameWidth * FrameHeight / 8
	// StatusBarHeight is reserved at the top of every frame for the firmware status bar.
	StatusBarHeight = 24
	AudioSampleRate = 16000
	// DeviceProtocol is the firmware sync protocol version; requests with another version get 426.
	DeviceProtocol = 2
)

// Envelope wraps every JSON response; failures carry only success and message.
type Envelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty" tstype:"unknown"`
}

type User struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	Username *string `json:"username" tstype:"string | null,required"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type Session struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type OrderRequest struct {
	Order []string `json:"order"`
}

type Device struct {
	ID         string     `json:"id"`
	MAC        string     `json:"mac"`
	Name       *string    `json:"name" tstype:"string | null,required"`
	GroupID    *string    `json:"groupId" tstype:"string | null,required"`
	SortOrder  int32      `json:"sortOrder"`
	BatteryPct *int16     `json:"batteryPct" tstype:"number | null,required"`
	RSSIDbm    *int16     `json:"rssiDbm" tstype:"number | null,required"`
	FWVersion  *string    `json:"fwVersion" tstype:"string | null,required"`
	LastSeenAt *time.Time `json:"lastSeenAt" tstype:"string | null,required"`
}

type ClaimDeviceRequest struct {
	PairCode string `json:"pairCode"`
}

// PatchDeviceRequest leaves omitted fields unchanged; an explicit null groupId clears the group.
type PatchDeviceRequest struct {
	Name    *string           `json:"name,omitempty"`
	GroupID Optional[*string] `json:"groupId,omitempty" tstype:"string | null"`
}

type Group struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SortOrder    int32  `json:"sortOrder"`
	ContentCount int    `json:"contentCount"`
	TotalBytes   int64  `json:"totalBytes"`
}

type GroupRequest struct {
	Name string `json:"name"`
}

type ContentKind string

const (
	ContentKindImage   ContentKind = "image"
	ContentKindDynamic ContentKind = "dynamic"
)

type AudioSource string

const (
	AudioSourceUpload AudioSource = "upload"
	AudioSourceTTS    AudioSource = "tts"
)

type AudioStatus string

const (
	AudioStatusPending    AudioStatus = "pending"
	AudioStatusGenerating AudioStatus = "generating"
	AudioStatusReady      AudioStatus = "ready"
	AudioStatusFailed     AudioStatus = "failed"
)

type Content struct {
	ID            string          `json:"id"`
	GroupID       string          `json:"groupId"`
	Seq           int32           `json:"seq"`
	Kind          ContentKind     `json:"kind"`
	FrameName     *string         `json:"frameName" tstype:"string | null,required"`
	StatusBarText string          `json:"statusBarText"`
	ImageHash     string          `json:"imageHash"`
	Audio         *ContentAudio   `json:"audio" tstype:"ContentAudio | null,required"`
	Dynamic       *ContentDynamic `json:"dynamic" tstype:"ContentDynamic | null,required"`
}

type ContentAudio struct {
	Source    AudioSource `json:"source"`
	Status    AudioStatus `json:"status"`
	Voice     *string     `json:"voice" tstype:"string | null,required"`
	Text      *string     `json:"text" tstype:"string | null,required"`
	Hash      *string     `json:"hash" tstype:"string | null,required"`
	Size      *int32      `json:"size" tstype:"number | null,required"`
	Error     *string     `json:"error" tstype:"string | null,required"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type ContentDynamic struct {
	Type       DynamicType     `json:"type"`
	Config     json.RawMessage `json:"config" tstype:"DynamicConfig"`
	Data       json.RawMessage `json:"data" tstype:"unknown"`
	RenderedAt *time.Time      `json:"renderedAt" tstype:"string | null,required"`
	NextRunAt  *time.Time      `json:"nextRunAt" tstype:"string | null,required"`
	Error      *string         `json:"error" tstype:"string | null,required"`
}

// CreateDynamicContentRequest creates dynamic content; image content is created with
// multipart/form-data fields frame (FrameBytes of packed 1bpp, rendered by the browser),
// audio and frameName.
type CreateDynamicContentRequest struct {
	Config      json.RawMessage `json:"config" tstype:"DynamicConfig"`
	FrameName   *string         `json:"frameName,omitempty"`
	InitialData json.RawMessage `json:"initialData,omitempty" tstype:"JsonObject"`
}

// PatchContentRequest updates frame name or dynamic config; image and audio files are
// replaced with multipart/form-data.
type PatchContentRequest struct {
	FrameName Optional[*string] `json:"frameName,omitempty" tstype:"string | null"`
	Config    json.RawMessage   `json:"config,omitempty" tstype:"DynamicConfig"`
}

type PreviewDynamicRequest struct {
	Config json.RawMessage `json:"config" tstype:"DynamicConfig"`
	Data   json.RawMessage `json:"data,omitempty" tstype:"JsonObject"`
}

type GenerateTTSRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

// IngestRequest is the dashboard push payload; the content ID in the URL is the credential.
type IngestRequest struct {
	Version int                        `json:"version"`
	Data    map[string]json.RawMessage `json:"data" tstype:"JsonObject"`
}

type IngestResponse struct {
	ID         string    `json:"id"`
	ImageHash  string    `json:"imageHash"`
	RenderedAt time.Time `json:"renderedAt"`
}

type WeatherCity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Adm1 string `json:"adm1"`
	Adm2 string `json:"adm2"`
}

const DefaultTTSVoice = "冰糖"

var TTSVoices = []string{"冰糖", "茉莉", "苏打", "白桦", "Mia", "Chloe", "Milo", "Dean"}

func IsTTSVoice(v string) bool {
	for _, voice := range TTSVoices {
		if voice == v {
			return true
		}
	}
	return false
}
