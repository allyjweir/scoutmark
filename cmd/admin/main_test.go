package main

import "testing"

func TestFormatQueryValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "null", value: nil, want: "NULL"},
		{name: "text", value: "scoutmark", want: "scoutmark"},
		{name: "bytes", value: []byte("scoutmark"), want: "scoutmark"},
		{name: "number", value: 14, want: "14"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatQueryValue(test.value); got != test.want {
				t.Errorf("formatQueryValue(%v) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
