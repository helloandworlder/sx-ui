package service

import "testing"

func TestDynamicRateLimitInt64AcceptsJSONNumbers(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
	}{
		{name: "int64", in: int64(2_000_000), want: 2_000_000},
		{name: "int", in: int(2_000_000), want: 2_000_000},
		{name: "float64 from json map", in: float64(2_000_000), want: 2_000_000},
		{name: "missing", in: nil, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dynamicRateLimitInt64(tt.in)
			if got != tt.want {
				t.Fatalf("dynamicRateLimitInt64(%T(%v)) = %d, want %d", tt.in, tt.in, got, tt.want)
			}
		})
	}
}
