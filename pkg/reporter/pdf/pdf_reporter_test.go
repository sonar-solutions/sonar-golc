package pdf

import "testing"

func TestParseResultFileName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantOrg    string
		wantRepo   string
		wantBranch string
		wantOK     bool
	}{
		{
			name:       "simple json with no underscores",
			input:      "Result_org__repo__main.json",
			wantOrg:    "org",
			wantRepo:   "repo",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:       "underscore in org (GitLab group)",
			input:      "Result_my_group__repo__main.json",
			wantOrg:    "my_group",
			wantRepo:   "repo",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:       "underscore in repo slug",
			input:      "Result_org__my_app__main.json",
			wantOrg:    "org",
			wantRepo:   "my_app",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:       "underscore in branch name",
			input:      "Result_org__repo__feature_xyz.json",
			wantOrg:    "org",
			wantRepo:   "repo",
			wantBranch: "feature_xyz",
			wantOK:     true,
		},
		{
			name:       "underscores in every component",
			input:      "Result_my_group__my_app__feat_xyz.json",
			wantOrg:    "my_group",
			wantRepo:   "my_app",
			wantBranch: "feat_xyz",
			wantOK:     true,
		},
		{
			// SplitN with N=3 keeps any trailing "__" intact in the last segment,
			// so a literal double-underscore inside the branch name survives.
			name:       "literal double underscore inside branch",
			input:      "Result_org__repo__feat__xyz.json",
			wantOrg:    "org",
			wantRepo:   "repo",
			wantBranch: "feat__xyz",
			wantOK:     true,
		},
		{
			name:       "byfile pdf variant",
			input:      "Result_org__repo__main_byfile.pdf",
			wantOrg:    "org",
			wantRepo:   "repo",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:       "pdf without byfile suffix",
			input:      "Result_org__repo__main.pdf",
			wantOrg:    "org",
			wantRepo:   "repo",
			wantBranch: "main",
			wantOK:     true,
		},
		{
			name:   "missing branch returns not ok",
			input:  "Result_org__repo.json",
			wantOK: false,
		},
		{
			name:   "missing prefix returns not ok",
			input:  "other_org__repo__main.json",
			wantOK: false,
		},
		{
			// Hard cutover: pre-migration single-'_' names are intentionally
			// rejected so legacy stale files in Results/ are not parsed with
			// the old ambiguous heuristic. They will be regenerated on the
			// next analysis run.
			name:   "legacy single-underscore format rejected",
			input:  "Result_org_repo_main.json",
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
