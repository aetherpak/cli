package cmd

import "testing"

// Guards that every command consumed by an action exposes --output-file.
func TestOutputFileFlagsRegistered(t *testing.T) {
	for _, name := range []string{"build", "import", "push-oci", "plan"} {
		c, _, err := RootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if c.Flags().Lookup("output-file") == nil {
			t.Errorf("%s: missing --output-file flag", name)
		}
	}
}

// Verifies that all commands with environment variable support have their
// flags automatically and correctly bound via bindFlags.
func TestEnvBindings(t *testing.T) {
	tests := []struct {
		command  string
		flagName string
		envName  string
		expected string
	}{
		{"build", "app-id", "AETHERPAK_APP_ID", "custom-build-appid"},
		{"build", "app", "AETHERPAK_APP", "custom-build-app"},
		{"build", "repo-path", "AETHERPAK_REPO_PATH", "custom-build-repo"},
		{"build", "branch", "AETHERPAK_BRANCH", "custom-build-branch"},
		{"import", "app-id", "AETHERPAK_APP_ID", "custom-import-appid"},
		{"import", "app", "AETHERPAK_APP", "custom-import-app"},
		{"import", "repo-path", "AETHERPAK_REPO_PATH", "custom-import-repo"},
		{"import", "branch", "AETHERPAK_BRANCH", "custom-import-branch"},
		{"push-oci", "app-id", "AETHERPAK_APP_ID", "custom-push-appid"},
		{"push-oci", "app", "AETHERPAK_APP", "custom-push-app"},
		{"push-oci", "registry", "AETHERPAK_REGISTRY", "custom-registry"},
		{"push-oci", "oci-repository", "AETHERPAK_OCI_REPOSITORY", "custom-oci-repo"},
		{"push-oci", "repo-path", "AETHERPAK_REPO_PATH", "custom-push-repo"},
		{"push-oci", "branch", "AETHERPAK_BRANCH", "custom-push-branch"},
		{"push-oci", "records-dir", "AETHERPAK_RECORDS_DIR", "custom-push-records"},
		{"build-site", "records-dir", "AETHERPAK_RECORDS_DIR", "custom-site-records"},
		{"build-site", "site-dir", "AETHERPAK_SITE_DIR", "custom-site-dir"},
		{"build-site", "pages-url", "AETHERPAK_PAGES_URL", "http://custom-pages"},
		{"build-site", "remote-name", "AETHERPAK_REMOTE_NAME", "custom-remote"},
		{"publish", "app-id", "AETHERPAK_APP_ID", "custom-pub-appid"},
		{"publish", "app", "AETHERPAK_APP", "custom-pub-app"},
		{"publish", "registry", "AETHERPAK_REGISTRY", "custom-pub-registry"},
		{"publish", "oci-repository", "AETHERPAK_OCI_REPOSITORY", "custom-pub-oci-repo"},
		{"publish", "branch", "AETHERPAK_BRANCH", "custom-pub-branch"},
		{"publish", "repo-path", "AETHERPAK_REPO_PATH", "custom-pub-repo"},
		{"publish", "records-dir", "AETHERPAK_RECORDS_DIR", "custom-pub-records"},
		{"publish", "bundle", "AETHERPAK_BUNDLE", "custom-pub-bundle"},
		{"publish", "bundle-url", "AETHERPAK_BUNDLE_URL", "custom-pub-bundle-url"},
		{"publish", "bundle-path", "AETHERPAK_BUNDLE_PATH", "custom-pub-bundle-path"},
		{"release", "repo-path", "AETHERPAK_REPO_PATH", "custom-rel-repo"},
		{"release", "records-dir", "AETHERPAK_RECORDS_DIR", "custom-rel-records"},
		{"release", "site-dir", "AETHERPAK_SITE_DIR", "custom-rel-site"},
		{"inspect-repo", "repo-path", "AETHERPAK_REPO_PATH", "custom-inspect-repo"},
		{"plan", "branch", "AETHERPAK_BRANCH", "custom-plan-branch"},
		{"add", "app-id", "AETHERPAK_APP_ID", "custom-add-appid"},
		{"add", "id", "AETHERPAK_ID", "custom-add-id"},
	}

	for _, tt := range tests {
		t.Run(tt.command+"/"+tt.flagName, func(t *testing.T) {
			cmd, _, err := RootCmd.Find([]string{tt.command})
			if err != nil {
				t.Fatalf("find %s: %v", tt.command, err)
			}

			flag := cmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("flag %s not found on command %s", tt.flagName, tt.command)
			}

			// Save the current state of changed and value to restore it later
			oldChanged := flag.Changed
			oldVal := flag.Value.String()
			defer func() {
				flag.Changed = oldChanged
				_ = flag.Value.Set(oldVal)
			}()

			flag.Changed = false
			_ = flag.Value.Set(flag.DefValue)

			t.Setenv(tt.envName, tt.expected)

			initConfig()
			bindFlags(cmd)

			if flag.Value.String() != tt.expected {
				t.Errorf("expected flag %s on command %s to be %q, got %q", tt.flagName, tt.command, tt.expected, flag.Value.String())
			}
		})
	}
}
