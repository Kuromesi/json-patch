package jsonpatch

import (
	"testing"
)

func TestArrayIndexPatchMerger(t *testing.T) {
	cases := []struct {
		doc    string
		patch  string
		expect string
	}{
		{
			doc:    `[]`,
			patch:  `["test"]`,
			expect: `["test"]`,
		},
	}
	for _, c := range cases {
		MergePatch([]byte(c.doc), []byte(c.patch))
	}
}
