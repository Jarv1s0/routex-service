//go:build windows
// +build windows

package main

import (
	"reflect"
	"testing"
)

func TestParseParamArgs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
		wantErr bool
	}{
		{name: "empty file", content: "", want: nil},
		{name: "json empty", content: "[]", want: []string{}},
		{name: "json startup", content: `["--routex-startup"]`, want: []string{"--routex-startup"}},
		{name: "json preserves spaces", content: `["--profile","C:\\RouteX Data\\profile.yaml"]`, want: []string{"--profile", `C:\RouteX Data\profile.yaml`}},
		{name: "plain text is invalid", content: "--routex-startup", wantErr: true},
		{name: "invalid json array", content: `["--routex-startup"`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseParamArgs(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseParamArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveTargetArgsUsesDirectArgs(t *testing.T) {
	target, args, err := resolveTargetArgs([]string{"routex-run.exe", "RouteX.exe", "--routex-startup", "value with space"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != "RouteX.exe" {
		t.Fatalf("target = %q, want RouteX.exe", target)
	}
	want := []string{"--routex-startup", "value with space"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}
