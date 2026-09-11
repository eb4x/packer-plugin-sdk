// SPDX-License-Identifier: MPL-2.0

package winrm

import (
	"strings"
	"testing"

	"github.com/masterzen/winrm"
)

func TestPowerShellCommand_matchesUpstream(t *testing.T) {
	for _, script := range []string{
		"dir",
		`Remove-Item "C:\Users\Bjørn\tmp.txt"`,
		"Write-Output '日本語 😀'",
	} {
		if got, want := PowerShellCommand("", script), winrm.Powershell(script); got != want {
			t.Errorf("PowerShellCommand(\"\", %q):\n got %s\nwant %s", script, got, want)
		}
	}
}

func TestPowerShellCommand_binary(t *testing.T) {
	const script = "dir"
	encoded := strings.TrimPrefix(winrm.Powershell(script), "powershell.exe")
	cases := map[string]string{
		"pwsh":                                   "pwsh",
		"pwsh.exe":                               "pwsh.exe",
		`C:\Program Files\PowerShell\7\pwsh.exe`: `"C:\Program Files\PowerShell\7\pwsh.exe"`,
		`"C:\Program Files\PowerShell\7\pwsh.exe"`: `"C:\Program Files\PowerShell\7\pwsh.exe"`,
	}
	for exe, wantExe := range cases {
		if got, want := PowerShellCommand(exe, script), wantExe+encoded; got != want {
			t.Errorf("PowerShellCommand(%q, %q):\n got %s\nwant %s", exe, script, got, want)
		}
	}
}
