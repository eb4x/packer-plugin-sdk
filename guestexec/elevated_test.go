// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package guestexec

import (
	"regexp"
	"strings"
	"testing"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func testConfig() map[string]interface{} {
	return map[string]interface{}{
		"inline": []interface{}{"foo", "bar"},
	}
}

func TestProvisioner_GenerateElevatedRunner(t *testing.T) {

	// Non-elevated
	config := testConfig()
	p := new(packersdk.MockProvisioner)
	p.Prepare(config)
	comm := new(packersdk.MockCommunicator)
	p.ProvCommunicator = comm
	path, err := GenerateElevatedRunner("whoami", p)

	if err != nil {
		t.Fatalf("Did not expect error: %s", err.Error())
	}

	if comm.UploadCalled != true {
		t.Fatalf("Should have uploaded file")
	}

	matched, _ := regexp.MatchString("C:/Windows/Temp/packer-elevated-shell.*", path)
	if !matched {
		t.Fatalf("Got unexpected file: %s", path)
	}

	// A provisioner without PowerShellExecutableProvisioner keeps the
	// default executable.
	if !strings.HasPrefix(path, "powershell.exe ") {
		t.Fatalf("Expected powershell prefix, got: %s", path)
	}
}

type mockExeProvisioner struct {
	*packersdk.MockProvisioner
	exe string
}

func (p *mockExeProvisioner) PowerShellExecutable() string {
	return p.exe
}

func TestProvisioner_GenerateElevatedRunner_executable(t *testing.T) {
	mock := new(packersdk.MockProvisioner)
	mock.Prepare(testConfig())
	mock.ProvCommunicator = new(packersdk.MockCommunicator)

	for exe, wantPrefix := range map[string]string{
		"pwsh":                                   `pwsh -executionpolicy bypass -file "C:/Windows/Temp/packer-elevated-shell-`,
		"":                                       `powershell.exe -executionpolicy bypass -file "C:/Windows/Temp/packer-elevated-shell-`,
		`C:\Program Files\PowerShell\7\pwsh.exe`: `"C:\Program Files\PowerShell\7\pwsh.exe" -executionpolicy bypass -file "C:/Windows/Temp/packer-elevated-shell-`,
	} {
		p := &mockExeProvisioner{MockProvisioner: mock, exe: exe}
		cmd, err := GenerateElevatedRunner("whoami", p)
		if err != nil {
			t.Fatalf("Did not expect error: %s", err.Error())
		}
		if !strings.HasPrefix(cmd, wantPrefix) {
			t.Fatalf("PowerShellExecutable %q: expected prefix %q, got: %s", exe, wantPrefix, cmd)
		}
	}
}
