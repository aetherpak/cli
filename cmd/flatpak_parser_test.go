package cmd

import (
	"testing"
)

func TestParseFlatpakRemotes(t *testing.T) {
	tests := []struct {
		name    string
		remotes []string
		want    map[string]string
		wantErr bool
	}{
		{
			name:    "valid remotes",
			remotes: []string{"flathub=https://dl.flathub.org/repo/flathub.flatpakrepo", "repoA=https://example.com/repoA.flatpakrepo"},
			want: map[string]string{
				"flathub": "https://dl.flathub.org/repo/flathub.flatpakrepo",
				"repoA":   "https://example.com/repoA.flatpakrepo",
			},
			wantErr: false,
		},
		{
			name:    "invalid format missing equals",
			remotes: []string{"flathubhttps://dl.flathub.org/repo/flathub.flatpakrepo"},
			wantErr: true,
		},
		{
			name:    "empty name",
			remotes: []string{"=https://dl.flathub.org/repo/flathub.flatpakrepo"},
			wantErr: true,
		},
		{
			name:    "empty url",
			remotes: []string{"flathub="},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFlatpakRemotes(tt.remotes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFlatpakRemotes() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("expected len %d, got %d", len(tt.want), len(got))
				}
				for k, v := range tt.want {
					if got[k] != v {
						t.Errorf("expected key %q to be %q, got %q", k, v, got[k])
					}
				}
			}
		})
	}
}

func TestParseFlatpakDeps(t *testing.T) {
	tests := []struct {
		name       string
		deps       []string
		wantRemote []string
		wantRef    []string
		wantErr    bool
	}{
		{
			name:       "valid deps with colon",
			deps:       []string{"flathub:org.gnome.Sdk//45", "repoA:org.gnome.Sdk.ExtensionA//45"},
			wantRemote: []string{"flathub", "repoA"},
			wantRef:    []string{"org.gnome.Sdk//45", "org.gnome.Sdk.ExtensionA//45"},
			wantErr:    false,
		},
		{
			name:       "valid deps with equals",
			deps:       []string{"flathub=org.gnome.Sdk//45"},
			wantRemote: []string{"flathub"},
			wantRef:    []string{"org.gnome.Sdk//45"},
			wantErr:    false,
		},
		{
			name:    "invalid missing separator",
			deps:    []string{"flathuborg.gnome.Sdk//45"},
			wantErr: true,
		},
		{
			name:    "empty remote",
			deps:    []string{":org.gnome.Sdk//45"},
			wantErr: true,
		},
		{
			name:    "empty ref",
			deps:    []string{"flathub:"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFlatpakDeps(tt.deps)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFlatpakDeps() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(got) != len(tt.wantRemote) {
					t.Fatalf("expected len %d, got %d", len(tt.wantRemote), len(got))
				}
				for i := range got {
					if got[i].Remote != tt.wantRemote[i] {
						t.Errorf("expected remote %q, got %q", tt.wantRemote[i], got[i].Remote)
					}
					if got[i].Ref != tt.wantRef[i] {
						t.Errorf("expected ref %q, got %q", tt.wantRef[i], got[i].Ref)
					}
				}
			}
		})
	}
}
