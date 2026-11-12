package utils

import (
	"reflect"
	"testing"
)

func TestSortedArgKeys(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want []string
	}{
		{name: "nil", args: nil, want: nil},
		{name: "empty", args: map[string]any{}, want: nil},
		{
			name: "sorted names only",
			args: map[string]any{"z": 1, "a": "secret-value", "m": []string{"x"}},
			want: []string{"a", "m", "z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SortedArgKeys(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SortedArgKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}
