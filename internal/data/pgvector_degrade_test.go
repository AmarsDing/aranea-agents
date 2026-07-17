package data

import (
	"errors"
	"testing"
)

func TestIsPgvectorExtensionError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("knowledge schema: other failure"), false},
		{errors.New(`ERROR:  extension "vector" is not available`), true},
		{errors.New(`Could not open extension control file ".../vector.control"`), true},
		{errors.New("create extension vector: permission denied"), true},
	}
	for _, tc := range cases {
		if got := isPgvectorExtensionError(tc.err); got != tc.want {
			t.Fatalf("err=%v want=%v got=%v", tc.err, tc.want, got)
		}
	}
}
