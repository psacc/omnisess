package model

import "testing"

func TestQualifiedID(t *testing.T) {
	tests := []struct {
		name string
		sess Session
		want string
	}{
		{
			name: "claude session",
			sess: Session{ID: "abc12345", Tool: ToolClaude},
			want: "claude:abc12345",
		},
		{
			name: "cursor session",
			sess: Session{ID: "xyz99999", Tool: ToolCursor},
			want: "cursor:xyz99999",
		},
		{
			name: "empty ID",
			sess: Session{ID: "", Tool: ToolClaude},
			want: "claude:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sess.QualifiedID()
			if got != tt.want {
				t.Errorf("QualifiedID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "long UUID",
			id:   "abc12345-1234-5678-9abc-def012345678",
			want: "abc12345",
		},
		{
			name: "exactly 8 chars",
			id:   "abcdefgh",
			want: "abcdefgh",
		},
		{
			name: "shorter than 8",
			id:   "abc",
			want: "abc",
		},
		{
			name: "empty",
			id:   "",
			want: "",
		},
		{
			name: "9 chars",
			id:   "123456789",
			want: "12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{ID: tt.id}
			got := s.ShortID()
			if got != tt.want {
				t.Errorf("ShortID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortProject(t *testing.T) {
	tests := []struct {
		name    string
		project string
		want    string
	}{
		{
			name:    "full path",
			project: "/Users/paolo/prj/sessions",
			want:    "prj/sessions",
		},
		{
			name:    "two components",
			project: "/foo/bar",
			want:    "foo/bar",
		},
		{
			name:    "single component",
			project: "/onlyone",
			want:    "onlyone",
		},
		{
			name:    "empty path",
			project: "",
			want:    "",
		},
		{
			name:    "deep path",
			project: "/a/b/c/d/e",
			want:    "d/e",
		},
		{
			name:    "trailing slash stripped by splitPath",
			project: "/Users/foo/bar/",
			want:    "foo/bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{Project: tt.project}
			got := s.ShortProject()
			if got != tt.want {
				t.Errorf("ShortProject() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "absolute path",
			path: "/Users/foo/bar",
			want: []string{"Users", "foo", "bar"},
		},
		{
			name: "relative path",
			path: "foo/bar/baz",
			want: []string{"foo", "bar", "baz"},
		},
		{
			name: "empty",
			path: "",
			want: nil,
		},
		{
			name: "root slash only",
			path: "/",
			want: nil,
		},
		{
			name: "double slashes",
			path: "/foo//bar",
			want: []string{"foo", "bar"},
		},
		{
			name: "single component",
			path: "foo",
			want: []string{"foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPath(tt.path)
			if len(got) != len(tt.want) {
				t.Fatalf("splitPath(%q) = %v (len %d), want %v (len %d)", tt.path, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitPath(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}
