package tools

import "testing"

func TestObsidianDeepLink(t *testing.T) {
	tests := []struct {
		name      string
		vaultName string
		relPath   string
		want      string
	}{
		{
			name:      "simple note",
			vaultName: "MyVault",
			relPath:   "Notes/simple.md",
			want:      "obsidian://open?file=Notes%2Fsimple&vault=MyVault",
		},
		{
			name:      "empty vault name yields no link",
			vaultName: "",
			relPath:   "Notes/simple.md",
			want:      "",
		},
		{
			name:      "vault name with spaces is URL-encoded",
			vaultName: "My Vault",
			relPath:   "Note.md",
			want:      "obsidian://open?file=Note&vault=My+Vault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := obsidianDeepLink(tt.vaultName, tt.relPath)
			if got != tt.want {
				t.Errorf("obsidianDeepLink(%q, %q) = %q, want %q", tt.vaultName, tt.relPath, got, tt.want)
			}
		})
	}
}
