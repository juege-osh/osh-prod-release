package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/ssh"
)

func TestExecuteRawGreenRestoresSnapshotWhenSQLFails(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(tmp, "sshpass.log")
	fakeSSHPass := `#!/usr/bin/env bash
set -euo pipefail
remote="${@: -1}"
printf "%s\n" "$remote" > "$SSHPASS_LOG"
if [[ "$remote" == *"mysqldump"* && "$remote" == *"__OSH_SQL_RESTORE_OK__"* ]]; then
  echo "__OSH_SQL_RESTORE_OK__"
  exit 17
fi
echo "missing restore path" >&2
exit 17
`
	if err := os.WriteFile(filepath.Join(binDir, "sshpass"), []byte(fakeSSHPass), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SSHPASS_LOG", logPath)

	cfg := &config.Config{
		MockMode:               false,
		ProdExecMode:           "ssh",
		ProdSSHHost:            "203.0.113.10",
		ProdSSHPort:            22,
		ProdSSHUser:            "root",
		ProdSSHPassword:        "pw",
		GreenMySQLContainer:    "osh-g-mysql",
		GreenMySQLDatabase:     "backstage",
		GreenMySQLRootPassword: "pw",
	}
	runner := NewRunner(cfg, ssh.New(cfg))

	res, err := runner.ExecuteRaw(context.Background(), "case fail restore", "alter table demo add column c int;", "tester")
	if err == nil {
		t.Fatal("ExecuteRaw succeeded, want failure after apply error")
	}
	if res == nil {
		t.Fatal("ExecuteRaw returned nil result")
	}
	if res.Success {
		t.Fatal("ExecuteRaw result Success = true, want false")
	}
	if !strings.Contains(res.Output, "__OSH_SQL_RESTORE_OK__") {
		t.Fatalf("output does not show restore success:\n%s", res.Output)
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	remoteScript := string(logRaw)
	if !strings.Contains(remoteScript, "mysqldump") {
		t.Fatalf("remote script does not include mysqldump backup:\n%s", remoteScript)
	}
	if strings.Count(remoteScript, " mysql ") < 2 {
		t.Fatalf("remote script should include apply mysql and restore mysql calls:\n%s", remoteScript)
	}
}
