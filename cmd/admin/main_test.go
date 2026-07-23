package main

import (
	"errors"
	"testing"

	"github.com/lib/pq"
)

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

func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(&pq.Error{Code: "23505"}) {
		t.Error("unique violation was not detected")
	}
	if isUniqueViolation(&pq.Error{Code: "23503"}) {
		t.Error("foreign key violation was detected as unique")
	}
	if isUniqueViolation(errors.New("not a PostgreSQL error")) {
		t.Error("non-PostgreSQL error was detected as unique")
	}
}
