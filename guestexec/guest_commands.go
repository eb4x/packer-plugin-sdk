// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package guestexec

import (
	"cmp"
	"fmt"
	"log"
	"strings"
)

const UnixOSType = "unix"
const WindowsOSType = "windows"
const DefaultOSType = UnixOSType

type GuestCommands struct {
	GuestOSType string
	Sudo        bool
	// PowerShellExecutable names the executable for the Windows commands.
	// Empty means powershell.exe.
	PowerShellExecutable string
}

func NewGuestCommands(osType string, sudo bool) (*GuestCommands, error) {
	if osType != UnixOSType && osType != WindowsOSType {
		return nil, fmt.Errorf("Invalid osType: \"%s\"", osType)
	}
	return &GuestCommands{GuestOSType: osType, Sudo: sudo}, nil
}

func (g *GuestCommands) Chmod(path string, mode string) string {
	return g.sudo(fmt.Sprintf(g.commands().chmod, mode, g.escapePath(path)))
}

func (g *GuestCommands) CreateDir(path string) string {
	return g.sudo(fmt.Sprintf(g.commands().mkdir, g.escapePath(path)))
}

func (g *GuestCommands) RemoveDir(path string) string {
	return g.sudo(fmt.Sprintf(g.commands().removeDir, g.escapePath(path)))
}

type guestOSTypeCommand struct {
	chmod     string
	mkdir     string
	removeDir string
	statPath  string
	mv        string
}

var unixCommands = guestOSTypeCommand{
	chmod:     "chmod %s '%s'",
	mkdir:     "mkdir -p '%s'",
	removeDir: "rm -rf '%s'",
	statPath:  "stat '%s'",
	mv:        "mv '%s' '%s'",
}

func windowsCommands(exe string) guestOSTypeCommand {
	return guestOSTypeCommand{
		chmod:     "echo 'skipping chmod %s %s'", // no-op
		mkdir:     exe + " -Command \"New-Item -ItemType directory -Force -ErrorAction SilentlyContinue -Path %s\"",
		removeDir: exe + " -Command \"rm %s -recurse -force\"",
		statPath:  exe + " -Command { if (test-path %s) { exit 0 } else { exit 1 } }",
		mv:        exe + " -Command \"mv %s %s -force\"",
	}
}

func (g *GuestCommands) commands() guestOSTypeCommand {
	switch g.GuestOSType {
	case WindowsOSType:
		return windowsCommands(quoteExe(cmp.Or(g.PowerShellExecutable, "powershell.exe")))
	case UnixOSType:
		return unixCommands
	default:
		log.Printf("[WARN] unknown GuestOSType %q, using unix commands", g.GuestOSType)
		return unixCommands
	}
}

// quoteExe keeps cmd.exe from splitting an unquoted spaced path into a
// program name and arguments.
func quoteExe(exe string) string {
	if strings.Contains(exe, " ") && !strings.HasPrefix(exe, `"`) {
		return `"` + exe + `"`
	}
	return exe
}

func (g *GuestCommands) escapePath(path string) string {
	if g.GuestOSType == WindowsOSType {
		return strings.Replace(path, " ", "` ", -1)
	}
	return path
}

func (g *GuestCommands) StatPath(path string) string {
	return g.sudo(fmt.Sprintf(g.commands().statPath, g.escapePath(path)))
}

func (g *GuestCommands) MovePath(srcPath string, dstPath string) string {
	return g.sudo(fmt.Sprintf(g.commands().mv, g.escapePath(srcPath), g.escapePath(dstPath)))
}

func (g *GuestCommands) sudo(cmd string) string {
	if g.GuestOSType == UnixOSType && g.Sudo {
		return "sudo " + cmd
	}
	return cmd
}
