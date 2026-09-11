// SPDX-License-Identifier: MPL-2.0

package winrm

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// PowerShellCommand replaces github.com/masterzen/winrm.Powershell,
// which hardcodes powershell.exe. The executable is quoted when it
// contains a space, so it names a program and cannot carry flags.
func PowerShellCommand(exe, script string) string {
	switch {
	case exe == "":
		exe = "powershell.exe"
	case strings.Contains(exe, " ") && !strings.HasPrefix(exe, `"`):
		exe = `"` + exe + `"`
	}

	script = "$ProgressPreference = 'SilentlyContinue';" + script
	units := utf16.Encode([]rune(script))
	wide := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(wide[2*i:], u)
	}

	return exe + " -EncodedCommand " + base64.StdEncoding.EncodeToString(wide)
}
