package main

import (
	"context"
	"testing"
	"time"
)

func TestBoolValue(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		v    any
		d    bool
		want bool
	}{
		{"Normal usage", true, false, true},
		{"nil", nil, false, false},
		{"false", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := ContextKeyType[bool]{"boolKey"}
			ctx := context.Background()
			if tt.v != nil {
				ctx = context.WithValue(ctx, k, tt.v)
			}
			got := ContextValue(ctx, k, tt.d)
			if got != tt.want {
				t.Errorf("BoolValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringValue(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		v    any
		d    string
		want string
	}{
		{"normal usage", "yes", "", "yes"},
		{"nil", nil, "", ""},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := ContextKeyType[string]{"stringKey"}
			ctx := context.Background()
			if tt.v != nil {
				ctx = context.WithValue(ctx, k, tt.v)
			}
			got := ContextValue(ctx, k, tt.d)
			if got != tt.want {
				t.Errorf("StringValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeValue(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		v any
		d time.Time
	}{
		{"normal usage", time.Now(), time.Time{}},
		{"nil", nil, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k := ContextKeyType[time.Time]{"timeKey"}
			if tt.v != nil {
				ctx = context.WithValue(ctx, k, tt.v)
			}
			got := ContextValue(ctx, k, tt.d)
			if tt.v != nil && got.IsZero() {
				t.Errorf("TimeValue() = %v", got)
			}
			if tt.v == nil && !got.IsZero() {
				t.Errorf("TimeValue() = %v", got)
			}
		})
	}
}
