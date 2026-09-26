package ui

import (
	"reflect"
	"testing"
)

func TestParseEditorCommand(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  []string
	}{
		{"code --wait", []string{"code", "--wait"}},
		{`"/Applications/Visual Studio Code.app/bin/code" --wait`, []string{"/Applications/Visual Studio Code.app/bin/code", "--wait"}},
		{`vim -c 'set number'`, []string{"vim", "-c", "set number"}},
		{`vim\ editor --wait`, []string{"vim editor", "--wait"}},
	} {
		got, err := parseEditorCommand(tt.input)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseEditorCommand(%q) = %q, %v; want %q", tt.input, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", `"unterminated`, `vim\`} {
		if _, err := parseEditorCommand(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
