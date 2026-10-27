package legacyimport

import (
	"encoding/json"
	"testing"
)

func TestConvertConfig(t *testing.T) {
	cases := map[string]string{
		`{"type":"weather","tz":"Asia/Shanghai","provider":"qweather","location_id":"101210101","location_label":"杭州","audio_enabled":true,"audio_voice":"茉莉"}`:                                                                                    `{"type":"weather","tz":"Asia/Shanghai","audioEnabled":true,"audioVoice":"茉莉","locationId":"101210101","locationLabel":"杭州"}`,
		`{"type":"hot_list","source":"wallstreetcn","refresh_interval_sec":900}`:                                                                                                                                                                   `{"type":"hot_list","refreshIntervalSec":900,"source":"wallstreetcn-quick"}`,
		`{"type":"dashboard","refresh_interval_sec":600,"template":{"kind":"custom","template":{"version":1,"blocks":[{"type":"progress","x":20,"y":120,"w":360,"h":34,"label":"{a}","value_text":"{b}","label_font_size":16,"bar_height":14}]}}}`: `{"type":"dashboard","refreshIntervalSec":600,"template":{"kind":"custom","template":{"version":1,"blocks":[{"type":"progress","x":20,"y":120,"w":360,"h":34,"x1":0,"y1":0,"x2":0,"y2":0,"label":"{a}","valueText":"{b}","labelFontSize":16,"valueFontSize":12,"barHeight":14}]}}}`,
	}
	for in, want := range cases {
		got, err := convertConfig([]byte(in))
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		var a, b any
		_ = json.Unmarshal(got, &a)
		_ = json.Unmarshal([]byte(want), &b)
		ga, _ := json.Marshal(a)
		gb, _ := json.Marshal(b)
		if string(ga) != string(gb) {
			t.Errorf("%s:\n got %s\nwant %s", in, ga, gb)
		}
	}
}
