package main

import (
	"reflect"
	"testing"
)

func TestNormalizeLegacyFlags(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "legacy space-separated config",
			in:   []string{"dns-proxy", "-config", "./dns-proxy.yaml"},
			want: []string{"dns-proxy", "--config", "./dns-proxy.yaml"},
		},
		{
			name: "legacy equals-form config",
			in:   []string{"dns-proxy", "-config=./dns-proxy.yaml"},
			want: []string{"dns-proxy", "--config=./dns-proxy.yaml"},
		},
		{
			name: "already double-dash config unchanged",
			in:   []string{"dns-proxy", "--config", "./dns-proxy.yaml"},
			want: []string{"dns-proxy", "--config", "./dns-proxy.yaml"},
		},
		{
			name: "legacy version flag",
			in:   []string{"dns-proxy", "-version"},
			want: []string{"dns-proxy", "--version"},
		},
		{
			name: "positional run subcommand unchanged",
			in:   []string{"dns-proxy", "run", "-config", "./dns-proxy.yaml"},
			want: []string{"dns-proxy", "run", "--config", "./dns-proxy.yaml"},
		},
		{
			name: "unrelated single-dash flag unchanged",
			in:   []string{"dns-proxy", "-x"},
			want: []string{"dns-proxy", "-x"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeLegacyFlags(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("normalizeLegacyFlags(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
