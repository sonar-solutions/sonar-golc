package pdf

import "testing"

func TestParseResultFileName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantOrg     string
		wantRepo    string
		wantBranch  string
		wantOK      bool
	}{
		{
			name:       "simple json with no underscores",
			input:      "Result_org_repo_main.json",
			wantOrg:    "org",
			wantRepo:   "repo",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:       "underscore in org (GitLab group)",
			input:      "Result_my_group_repo_main.json",
			wantOrg:    "my",
			wantRepo:   "group_repo",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:       "underscore in repo slug",
			input:      "Result_org_my_app_main.json",
			wantOrg:    "org",
			wantRepo:   "my_app",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			// Known ambiguity: '_' is also valid inside branch names, so
			// "feature_xyz" is mislabeled — "feature" is absorbed into Repo
			// and only "xyz" is returned as Branch. See the comment on
			// parseResultFileName for the long-term fix.
			name:       "underscore in branch name (mislabeled by last-segment heuristic)",
			input:      "Result_org_repo_feature_xyz.json",
			wantOrg:    "org",
			wantRepo:   "repo_feature",
			wantBranch: "xyz",
			wantOK:     true,
		},
		{
			name:       "byfile pdf variant",
			input:      "Result_org_repo_main_byfile.pdf",
			wantOrg:    "org",
			wantRepo:   "repo",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:       "pdf without byfile suffix",
			input:      "Result_org_repo_main.pdf",
			wantOrg:    "org",
			wantRepo:   "repo",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:   "missing branch returns not ok",
			input:  "Result_org_repo.json",
			wantOK: false,
		},
		{
			name:   "missing prefix returns not ok",
			input:  "other_org_repo_main.json",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, repo, branch, ok := parseResultFileName(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if org != tt.wantOrg {
				t.Errorf("org = %q, want %q", org, tt.wantOrg)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", branch, tt.wantBranch)
			}
		})
	}
}
