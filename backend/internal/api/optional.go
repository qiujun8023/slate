package api

import "encoding/json"

// Optional distinguishes an omitted JSON field from an explicit null.
type Optional[T any] struct {
	Set   bool
	Value T
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.Value)
}
