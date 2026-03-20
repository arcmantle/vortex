package main

import "testing"

func TestIsSupportedConfigPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "dev.vortex", want: true},
		{path: "dev.vortex.yaml", want: false},
		{path: "dev.yaml", want: false},
		{path: "dev.yml", want: false},
		{path: "dev.txt", want: false},
	}

	for _, tc := range tests {
		if got := isSupportedConfigPath(tc.path); got != tc.want {
			t.Fatalf("isSupportedConfigPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
