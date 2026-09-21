package config

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeProjectFile(t *testing.T, dir, contents string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, ProjectFileName)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestFindProjectFileResolution(t *testing.T) {
	t.Setenv(ProjectDirEnv, "")

	tests := []struct {
		name    string
		setup   func(t *testing.T) (want string, wantErr bool, errSubstr string)
		check   func(t *testing.T, path string)
		wantErr bool
	}{
		{
			name: "NIMBU_PROJECT_DIR finds file in that directory",
			setup: func(t *testing.T) (string, bool, string) {
				cwd := t.TempDir()
				envDir := t.TempDir()
				writeProjectFile(t, cwd, "site: cwd\n")
				want := writeProjectFile(t, envDir, "site: env\n")
				t.Chdir(cwd)
				t.Setenv(ProjectDirEnv, envDir)
				return want, false, ""
			},
		},
		{
			name: "NIMBU_PROJECT_DIR walks up from nested directory",
			setup: func(t *testing.T) (string, bool, string) {
				root := t.TempDir()
				nested := filepath.Join(root, "theme", "src")
				if err := os.MkdirAll(nested, 0o755); err != nil {
					t.Fatal(err)
				}
				want := writeProjectFile(t, root, "site: walked\n")
				t.Chdir(t.TempDir())
				t.Setenv(ProjectDirEnv, nested)
				return want, false, ""
			},
		},
		{
			name: "NIMBU_PROJECT_DIR does not fall through to CWD",
			setup: func(t *testing.T) (string, bool, string) {
				cwd := t.TempDir()
				empty := t.TempDir()
				writeProjectFile(t, cwd, "site: cwd\n")
				t.Chdir(cwd)
				t.Setenv(ProjectDirEnv, empty)
				return "", true, ProjectDirEnv
			},
		},
		{
			name: "walk up from CWD",
			setup: func(t *testing.T) (string, bool, string) {
				root := t.TempDir()
				nested := filepath.Join(root, "a", "b")
				if err := os.MkdirAll(nested, 0o755); err != nil {
					t.Fatal(err)
				}
				want := writeProjectFile(t, root, "site: cwd-walk\n")
				t.Chdir(nested)
				return want, false, ""
			},
		},
		{
			name: "missing project file",
			setup: func(t *testing.T) (string, bool, string) {
				t.Chdir(t.TempDir())
				return "", true, ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ProjectDirEnv, "")
			want, wantErr, errSubstr := tt.setup(t)
			got, err := FindProjectFile()
			if wantErr {
				if err == nil {
					t.Fatalf("FindProjectFile() = %q, want error", got)
				}
				if errSubstr != "" && !strings.Contains(err.Error(), errSubstr) {
					t.Fatalf("error %q, want substring %q", err, errSubstr)
				}
				if errSubstr == "" && !errors.Is(err, ErrNotFound) {
					t.Fatalf("error %v, want ErrNotFound", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindProjectFile(): %v", err)
			}
			if got != want {
				t.Fatalf("FindProjectFile() = %q, want %q", got, want)
			}
		})
	}
}

func TestReadProjectConfigUsesProjectDirEnv(t *testing.T) {
	cwd := t.TempDir()
	envDir := t.TempDir()
	writeProjectFile(t, cwd, "site: cwd\n")
	writeProjectFile(t, envDir, "site: from-env\n")
	t.Chdir(cwd)
	t.Setenv(ProjectDirEnv, envDir)

	cfg, err := ReadProjectConfig()
	if err != nil {
		t.Fatalf("ReadProjectConfig: %v", err)
	}
	if cfg.Site != "from-env" {
		t.Fatalf("site = %q, want from-env", cfg.Site)
	}

	root, err := ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}
	if root != envDir {
		t.Fatalf("ProjectRoot() = %q, want %q", root, envDir)
	}
}

func TestGitShowToplevel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	nested := filepath.Join(repo, "theme", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")

	top, ok := gitShowToplevel(nested)
	if !ok {
		t.Fatal("expected git top-level")
	}
	got, err := filepath.EvalSymlinks(top)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("gitShowToplevel() = %q, want %q", got, want)
	}

	if _, ok := gitShowToplevel(t.TempDir()); ok {
		t.Fatal("non-repo should not report a top-level")
	}
}

func TestFindProjectFileGitToplevelCandidate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	nested := filepath.Join(repo, "subdir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	want := writeProjectFile(t, repo, "site: git-root\n")
	runGit(t, repo, "init")
	t.Chdir(nested)
	t.Setenv(ProjectDirEnv, "")

	got, err := FindProjectFile()
	if err != nil {
		t.Fatalf("FindProjectFile: %v", err)
	}
	if got != want {
		t.Fatalf("FindProjectFile() = %q, want %q", got, want)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = withoutGitDirEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestFindProjectFileWalksUpToParent(t *testing.T) {
	t.Setenv(ProjectDirEnv, "")

	root := t.TempDir()
	nested := filepath.Join(root, "theme", "templates", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	want := writeProjectFile(t, root, "site: parent\n")
	t.Chdir(nested)

	got, err := FindProjectFile()
	if err != nil {
		t.Fatalf("FindProjectFile: %v", err)
	}
	if got != want {
		t.Fatalf("FindProjectFile() = %q, want %q", got, want)
	}
}

func TestFindProjectFileChecksGitRootButNotAbove(t *testing.T) {
	t.Setenv(ProjectDirEnv, "")

	outer := t.TempDir()
	repo := filepath.Join(outer, "repo")
	nested := filepath.Join(repo, "src", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// nimbu.yml only above the repo: the search must stop at the git root.
	writeProjectFile(t, outer, "site: above-git-root\n")
	t.Chdir(nested)

	got, err := FindProjectFile()
	if err == nil {
		t.Fatalf("FindProjectFile() = %q, want error", got)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}
	var lookupErr *ProjectLookupError
	if !errors.As(err, &lookupErr) {
		t.Fatalf("error %v, want *ProjectLookupError", err)
	}
	if lookupErr.StoppedAt != repo {
		t.Fatalf("StoppedAt = %q, want %q", lookupErr.StoppedAt, repo)
	}
	if want := "searched up to " + repo; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q, want substring %q", err, want)
	}

	// The git root itself is still checked.
	want := writeProjectFile(t, repo, "site: at-git-root\n")
	found, err := FindProjectFile()
	if err != nil {
		t.Fatalf("FindProjectFile: %v", err)
	}
	if found != want {
		t.Fatalf("FindProjectFile() = %q, want %q", found, want)
	}
}

func TestFindProjectFileProjectDirEnvBeatsCWDWalk(t *testing.T) {
	cwdRoot := t.TempDir()
	nested := filepath.Join(cwdRoot, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, cwdRoot, "site: from-cwd-walk\n")

	envDir := t.TempDir()
	want := writeProjectFile(t, envDir, "site: from-env\n")

	t.Chdir(nested)
	t.Setenv(ProjectDirEnv, envDir)

	got, err := FindProjectFile()
	if err != nil {
		t.Fatalf("FindProjectFile: %v", err)
	}
	if got != want {
		t.Fatalf("FindProjectFile() = %q, want %q", got, want)
	}
}

func TestDescribeProjectLookupReportsSearchBoundary(t *testing.T) {
	t.Setenv(ProjectDirEnv, "")

	repo := t.TempDir()
	nested := filepath.Join(repo, "scratch")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	_, err := FindProjectFile()
	if err == nil {
		t.Fatal("expected lookup error")
	}
	want := ProjectFileName + " (searched up to " + repo + ")"
	if got := DescribeProjectLookup(err); got != want {
		t.Fatalf("DescribeProjectLookup() = %q, want %q", got, want)
	}
	if got := DescribeProjectLookup(nil); got != want {
		t.Fatalf("DescribeProjectLookup(nil) = %q, want %q", got, want)
	}
	if got := ProjectSearchLimit(); got != repo {
		t.Fatalf("ProjectSearchLimit() = %q, want %q", got, repo)
	}
}
