// SPDX-License-Identifier: MPL-2.0

// The upload code in this file is derived from
// github.com/packer-community/winrmcp, used under the MIT license:
//
// Copyright (c) 2015 Dylan Meissner
//
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to
// permit persons to whom the Software is furnished to do so, subject to
// the following conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
// LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
// OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
// WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package winrm

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masterzen/winrm"
)

// Everything run on the guest sticks to cmd.exe and PowerShell 2.0 /
// .NET 2.0 features for maximum compatibility.

// maxCommandLen keeps an append command under cmd.exe's 8191-character
// command-line limit.
const maxCommandLen = 8000

// maxOperationsPerShell is a conservative default for how many commands a
// WinRM server accepts through one shell.
const maxOperationsPerShell = 15

// uploadFile writes src to toPath on the guest. WinRM offers no file
// transfer, so the content travels base64-encoded on command lines.
func (c *Communicator) uploadFile(toPath string, src io.Reader) error {
	toPath = winPath(toPath)

	// The scripts get the bare file name because a single-quoted literal
	// cannot expand $env:TEMP; cmd.exe gets the %TEMP% form.
	tempFile := fmt.Sprintf("packer-upload-%s.tmp", uuid.New())
	cmdTempPath := `%TEMP%\` + tempFile

	log.Printf("[DEBUG] Appending base64 content to %s", cmdTempPath)
	if err := c.appendBase64(cmdTempPath, src); err != nil {
		return fmt.Errorf("Error uploading file to %s: %v", cmdTempPath, err)
	}

	log.Printf("[DEBUG] Decoding %s into %s", cmdTempPath, toPath)
	if err := c.decodeTempFile(tempFile, toPath); err != nil {
		return fmt.Errorf("Error decoding file %s into %s: %v", cmdTempPath, toPath, err)
	}

	log.Printf("[DEBUG] Removing temporary file %s", cmdTempPath)
	if err := c.removeTempFile(tempFile); err != nil {
		return fmt.Errorf("Error removing temporary file %s: %v", cmdTempPath, err)
	}

	return nil
}

func (c *Communicator) uploadDir(toPath, fromPath string) error {
	return filepath.Walk(fromPath, func(hostPath string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() || fi.Name() == ".DS_Store" {
			return nil
		}

		absPath, _ := filepath.Abs(hostPath)
		absFrom, _ := filepath.Abs(fromPath)
		relPath, _ := filepath.Rel(absFrom, absPath)

		f, err := os.Open(absPath)
		if err != nil {
			return fmt.Errorf("Couldn't read file %s: %v", hostPath, err)
		}
		defer f.Close()

		return c.uploadFile(filepath.Join(toPath, relPath), f)
	})
}

func (c *Communicator) appendBase64(remotePath string, src io.Reader) error {
	for {
		done, err := c.appendBatch(remotePath, src)
		if err != nil {
			return err
		}
		if done {
			break
		}
	}

	return nil
}

func (c *Communicator) appendBatch(remotePath string, src io.Reader) (done bool, err error) {
	const appendCommand = `echo %s >> "%s"`
	shell, err := c.client.CreateShell()
	if err != nil {
		return false, fmt.Errorf("Couldn't create shell: %v", err)
	}
	defer shell.Close()

	// Size the chunk so its base64 encoding plus the command around it fits
	// in one command line.
	overhead := len(fmt.Sprintf(appendCommand, "", remotePath))
	encodedLen := maxCommandLen - overhead
	chunk := make([]byte, base64.StdEncoding.DecodedLen(encodedLen))

	for i := 0; i < maxOperationsPerShell; i++ {
		n, err := src.Read(chunk)
		if n > 0 {
			content := base64.StdEncoding.EncodeToString(chunk[:n])
			if err := runInShell(shell, fmt.Sprintf(appendCommand, content, remotePath)); err != nil {
				return false, err
			}
		}
		if err == io.EOF {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}

	return false, nil
}

const decodeTempFileScript = `
	$tmp = Join-Path $env:TEMP %[1]s
	$dest = [System.IO.Path]::GetFullPath(%[2]s)
	if (Test-Path -LiteralPath $dest -PathType Container) {
		[System.Console]::Error.WriteLine("$dest is a directory")
		Exit 1
	}
	$destDir = [System.IO.Path]::GetDirectoryName($dest)
	New-Item -ItemType Directory -Force -ErrorAction SilentlyContinue -Path $destDir | Out-Null

	if (-not (Test-Path -LiteralPath $tmp)) {
		[System.IO.File]::Create($dest).Close()
		Exit 0
	}

	$reader = [System.IO.File]::OpenText($tmp)
	$writer = [System.IO.File]::Create($dest)
	try {
		while (($line = $reader.ReadLine()) -ne $null) {
			$bytes = [System.Convert]::FromBase64String($line)
			$writer.Write($bytes, 0, $bytes.Length)
		}
	}
	finally {
		$reader.Close()
		$writer.Close()
	}
`

func (c *Communicator) decodeTempFile(tempFile, destPath string) error {
	return c.runPowerShell(fmt.Sprintf(decodeTempFileScript, psQuote(tempFile), psQuote(destPath)))
}

const removeTempFileScript = `
	$tmp = Join-Path $env:TEMP %s
	if (Test-Path -LiteralPath $tmp) {
		Remove-Item -LiteralPath $tmp
	}
`

func (c *Communicator) removeTempFile(tempFile string) error {
	return c.runPowerShell(fmt.Sprintf(removeTempFileScript, psQuote(tempFile)))
}

func (c *Communicator) runPowerShell(script string) error {
	shell, err := c.client.CreateShell()
	if err != nil {
		return fmt.Errorf("Couldn't create shell: %v", err)
	}
	defer shell.Close()

	return runInShell(shell, PowerShellCommand(c.config.PowerShellExecutable, script))
}

// runInShell drains both output pipes, or the command would block.
func runInShell(shell *winrm.Shell, command string) error {
	cmd, err := shell.Execute(command)
	if err != nil {
		return err
	}
	defer cmd.Close()

	var stderr bytes.Buffer
	var wg sync.WaitGroup
	copyFunc := func(w io.Writer, r io.Reader) {
		defer wg.Done()
		io.Copy(w, r)
	}
	wg.Add(2)
	go copyFunc(io.Discard, cmd.Stdout)
	go copyFunc(&stderr, cmd.Stderr)

	cmd.Wait()
	wg.Wait()

	if cmd.ExitCode() != 0 {
		return fmt.Errorf("exit code %d: %s", cmd.ExitCode(), strings.TrimSpace(stderr.String()))
	}
	return nil
}

// psQuote quotes s as a PowerShell single-quoted literal, inside which
// nothing is expanded.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func winPath(path string) string {
	return strings.ReplaceAll(path, "/", `\`)
}
