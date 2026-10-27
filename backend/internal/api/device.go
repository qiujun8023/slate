package api

// Device protocol: the firmware registers once, then authenticates every request with
// Authorization: Bearer <secret>. Blobs are addressed by sha256 and never change.

type DeviceRegisterRequest struct {
	Protocol int    `json:"protocol"`
	MAC      string `json:"mac"`
}

type DeviceRegisterResponse struct {
	DeviceID   string `json:"deviceId"`
	Secret     string `json:"secret"`
	PairCode   string `json:"pairCode"`
	ServerTime int64  `json:"serverTime"`
}

// DeviceSyncRequest reports telemetry. The manifest is returned only when
// ManifestVersion differs from the selected group's current version.
type DeviceSyncRequest struct {
	Protocol        int             `json:"protocol"`
	ManifestVersion string          `json:"manifestVersion"`
	Telemetry       DeviceTelemetry `json:"telemetry"`
}

type DeviceTelemetry struct {
	BatteryPct *int    `json:"batteryPct,omitempty"`
	RSSIDbm    *int    `json:"rssiDbm,omitempty"`
	FWVersion  *string `json:"fwVersion,omitempty"`
	// WakeReason is timer, button, power_on, charge or other. On timer wakes the server
	// renders CurrentContentID first when its dynamic refresh is due.
	WakeReason       string `json:"wakeReason,omitempty"`
	CurrentContentID string `json:"currentContentId,omitempty"`
}

type DeviceGroupRequest struct {
	Protocol        int    `json:"protocol"`
	Action          string `json:"action" tstype:"'next' | 'prev' | 'select'"`
	GroupID         string `json:"groupId,omitempty"`
	ManifestVersion string `json:"manifestVersion"`
}

type DeviceState struct {
	ServerTime int64        `json:"serverTime"`
	Device     DeviceInfo   `json:"device"`
	Group      *DeviceGroup `json:"group" tstype:"DeviceGroup | null,required"`
	Manifest   *Manifest    `json:"manifest" tstype:"Manifest | null,required"`
}

type DeviceInfo struct {
	ID       string  `json:"id"`
	Name     *string `json:"name" tstype:"string | null,required"`
	Bound    bool    `json:"bound"`
	PairCode *string `json:"pairCode" tstype:"string | null,required"`
}

type DeviceGroup struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Position        GroupPosition `json:"position"`
	ContentCount    int           `json:"contentCount"`
	ManifestVersion string        `json:"manifestVersion"`
}

type GroupPosition struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}

type Manifest struct {
	Version string          `json:"version"`
	Frames  []ManifestFrame `json:"frames"`
}

type ManifestFrame struct {
	ID            string `json:"id"`
	Seq           int32  `json:"seq"`
	StatusBarText string `json:"statusBarText"`
	Image         Blob   `json:"image"`
	Audio         *Blob  `json:"audio" tstype:"Blob | null,required"`
	// RefreshAt is the Unix time in seconds when a dynamic frame is re-rendered.
	RefreshAt *int64 `json:"refreshAt" tstype:"number | null,required"`
}

type Blob struct {
	Hash string `json:"hash"`
	Size int32  `json:"size"`
}
