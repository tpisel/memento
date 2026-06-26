package markdown

import (
	"errors"
	"testing"
)

func TestParseWriteMode(t *testing.T) {
	for _, mode := range []WriteMode{ModeAppendOnly, ModeLiving, ModeReadOnly} {
		got, err := ParseWriteMode(string(mode))
		if err != nil {
			t.Fatalf("ParseWriteMode(%q) err = %v, want nil", mode, err)
		}
		if got != mode {
			t.Fatalf("ParseWriteMode(%q) = %q, want %q", mode, got, mode)
		}
	}

	got, err := ParseWriteMode("  living  ")
	if err != nil {
		t.Fatalf("ParseWriteMode(padded) err = %v, want nil", err)
	}
	if got != ModeLiving {
		t.Fatalf("ParseWriteMode(padded) = %q, want %q", got, ModeLiving)
	}

	for _, bad := range []string{"", "frozen", "readonly", "append", "Living"} {
		if _, err := ParseWriteMode(bad); !errors.Is(err, ErrInvalidMode) {
			t.Fatalf("ParseWriteMode(%q) err = %v, want ErrInvalidMode", bad, err)
		}
	}
}

func TestSetMode(t *testing.T) {
	tests := []struct {
		name   string
		source string
		mode   WriteMode
		want   string
	}{
		{
			name:   "rewrites existing mode line",
			source: "---\ntitle: X\nmode: append-only\n---\n# H\n\nBody.\n",
			mode:   ModeReadOnly,
			want:   "---\ntitle: X\nmode: read-only\n---\n# H\n\nBody.\n",
		},
		{
			name:   "inserts when mode absent",
			source: "---\ntitle: X\n---\n# H\n\nBody.\n",
			mode:   ModeLiving,
			want:   "---\ntitle: X\nmode: living\n---\n# H\n\nBody.\n",
		},
		{
			name:   "creates frontmatter when absent",
			source: "# H\n\nBody.\n",
			mode:   ModeReadOnly,
			want:   "---\nmode: read-only\n---\n# H\n\nBody.\n",
		},
		{
			name:   "preserves other fields and block tags",
			source: "---\ntitle: X\ntags:\n  - a\n  - b\nmode: read-only\n---\n# H\n\nBody.\n",
			mode:   ModeLiving,
			want:   "---\ntitle: X\ntags:\n  - a\n  - b\nmode: living\n---\n# H\n\nBody.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(SetMode([]byte(tt.source), tt.mode))
			if got != tt.want {
				t.Fatalf("SetMode() =\n%q\nwant\n%q", got, tt.want)
			}

			meta, err := ExtractMetadata("note.md", []byte(got))
			if err != nil {
				t.Fatalf("ExtractMetadata(result) err = %v, want nil", err)
			}
			if meta.Mode != tt.mode {
				t.Fatalf("round-trip mode = %q, want %q", meta.Mode, tt.mode)
			}
		})
	}
}
