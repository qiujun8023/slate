package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	type body struct {
		Name string `json:"name"`
	}
	cases := []struct {
		in string
		ok bool
	}{
		{`{"name":"a"}`, true},
		{` {"name":"a"} ` + "\n", true},
		{`{} }`, false},
		{`{} ]`, false},
		{`{}{}`, false},
		{`null`, false},
		{`[]`, false},
		{`"x"`, false},
		{``, false},
		{`{"name":`, false},
		{`{"other":1}`, false},
		{`{"name":1}`, false},
	}
	for _, c := range cases {
		var v body
		err := decodeJSON(httptest.NewRequest("POST", "/", strings.NewReader(c.in)), 1<<10, &v)
		if (err == nil) != c.ok {
			t.Errorf("%q: err=%v, want ok=%v", c.in, err, c.ok)
		}
	}
	var v body
	if err := decodeJSON(httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"`+strings.Repeat("x", 2048)+`"}`)), 1<<10, &v); err == nil || !strings.Contains(err.Error(), "KB") {
		t.Errorf("oversized body: %v", err)
	}
}
