// SPDX-License-Identifier: MPL-2.0

package winrm

import "testing"

func TestWinPath(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"C:/Windows/Temp/a.txt":      `C:\Windows\Temp\a.txt`,
		"C:/Program Files/a.txt":     `C:\Program Files\a.txt`,
		`C:\Windows\Temp\packer.ps1`: `C:\Windows\Temp\packer.ps1`,
	}
	for in, want := range cases {
		if got := winPath(in); got != want {
			t.Errorf("winPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPsQuote(t *testing.T) {
	cases := map[string]string{
		"":                           `''`,
		`C:\Program Files\a.txt`:     `'C:\Program Files\a.txt'`,
		`C:\Users\O'Brien\$x[1].ps1`: `'C:\Users\O''Brien\$x[1].ps1'`,
	}
	for in, want := range cases {
		if got := psQuote(in); got != want {
			t.Errorf("psQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
