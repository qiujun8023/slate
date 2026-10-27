// Package hotlist fetches trending lists from Chinese and international sites.
package hotlist

import (
	_ "embed"
	"encoding/json"

	"slate/internal/api"
)

//go:embed sources.json
var sourcesJSON []byte

var catalog = func() []api.HotListSourceInfo {
	var out []api.HotListSourceInfo
	if err := json.Unmarshal(sourcesJSON, &out); err != nil {
		panic(err)
	}
	return out
}()

// Catalog lists every source in display order.
func Catalog() []api.HotListSourceInfo { return catalog }

func Info(id string) *api.HotListSourceInfo {
	for i := range catalog {
		if catalog[i].ID == id {
			return &catalog[i]
		}
	}
	return nil
}
