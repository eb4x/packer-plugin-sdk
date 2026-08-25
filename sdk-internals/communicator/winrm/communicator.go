// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

// Package winrm implements the WinRM communicator. Plugin maintainers should not
// import this package directly, instead using the tooling in the
// "packer-plugin-sdk/communicator" module.
package winrm

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/masterzen/winrm"
)

// Communicator represents the WinRM communicator
type Communicator struct {
	config   *Config
	client   *winrm.Client
	endpoint *winrm.Endpoint
}

// New creates a new communicator implementation over WinRM.
func New(config *Config) (*Communicator, error) {
	endpoint := &winrm.Endpoint{
		Host:     config.Host,
		Port:     config.Port,
		HTTPS:    config.Https,
		Insecure: config.Insecure,
		Timeout:  config.ConnectTimeout,

		/*
			TODO
			HTTPS:    connInfo.HTTPS,
			Insecure: connInfo.Insecure,
			CACert:   connInfo.CACert,
		*/
	}

	// Create the client
	params := *winrm.DefaultParameters

	if config.TransportDecorator != nil {
		params.TransportDecorator = config.TransportDecorator
	}

	params.Timeout = formatDuration(config.Timeout)
	client, err := winrm.NewClientWithParameters(
		endpoint, config.Username, config.Password, &params)
	if err != nil {
		return nil, err
	}

	// Create the shell to verify the connection
	log.Printf("[DEBUG] connecting to remote shell using WinRM")
	shell, err := client.CreateShell()
	if err != nil {
		log.Printf("[ERROR] connection error: %s", err)
		return nil, err
	}

	if err := shell.Close(); err != nil {
		log.Printf("[ERROR] error closing connection: %s", err)
		return nil, err
	}

	return &Communicator{
		config:   config,
		client:   client,
		endpoint: endpoint,
	}, nil
}

// Start implementation of communicator.Communicator interface
func (c *Communicator) Start(ctx context.Context, rc *packersdk.RemoteCmd) error {
	log.Printf("[INFO] starting remote command: %s", rc.Command)
	exitCode, err := c.client.RunWithContext(ctx, rc.Command, rc.Stdout, rc.Stderr)

	rc.SetExited(exitCode)
	log.Printf("[INFO] command '%s' exited with code: %d", rc.Command, exitCode)
	return err
}

// Upload implementation of communicator.Communicator interface
func (c *Communicator) Upload(path string, input io.Reader, fi *os.FileInfo) error {
	if strings.HasSuffix(path, `\`) {
		// path is a directory
		if fi != nil {
			path += filepath.Base((*fi).Name())
		} else {
			return fmt.Errorf("Was unable to infer file basename for upload.")
		}
	}
	log.Printf("Uploading file to '%s'", path)
	return c.uploadFile(path, input)
}

// UploadDir implementation of communicator.Communicator interface
func (c *Communicator) UploadDir(dst string, src string, exclude []string) error {
	if !strings.HasSuffix(src, "/") {
		dst = fmt.Sprintf("%s\\%s", dst, filepath.Base(src))
	}
	log.Printf("Uploading dir '%s' to '%s'", src, dst)
	return c.uploadDir(dst, src)
}

func (c *Communicator) Download(src string, dst io.Writer) error {
	client, err := c.newWinRMClient()
	if err != nil {
		return err
	}

	encodeScript := `$file=[System.IO.File]::ReadAllBytes("%s"); Write-Output $([System.Convert]::ToBase64String($file))`

	base64DecodePipe := &Base64Pipe{w: dst}

	cmd := winrm.Powershell(fmt.Sprintf(encodeScript, src))
	_, err = client.Run(cmd, base64DecodePipe, io.Discard)

	return err
}

func (c *Communicator) DownloadDir(src string, dst string, exclude []string) error {
	return fmt.Errorf("WinRM doesn't support download dir.")
}

// newWinRMClient creates a client for Download with a fixed 3 minute
// operation timeout, independent of the configured Timeout used by c.client.
func (c *Communicator) newWinRMClient() (*winrm.Client, error) {
	var endpoint = &winrm.Endpoint{
		Host:     c.endpoint.Host,
		Port:     c.endpoint.Port,
		HTTPS:    c.config.Https,
		Insecure: c.config.Insecure,
	}
	params := winrm.NewParameters(
		winrm.DefaultParameters.Timeout,
		winrm.DefaultParameters.Locale,
		winrm.DefaultParameters.EnvelopeSize,
	)

	params.TransportDecorator = c.config.TransportDecorator
	params.Timeout = "PT3M"

	client, err := winrm.NewClientWithParameters(
		endpoint, c.config.Username, c.config.Password, params)
	return client, err
}

type Base64Pipe struct {
	w io.Writer // underlying writer (file, buffer)
}

func (d *Base64Pipe) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}

	var i int
	i, err = d.Write(b)

	if err != nil {
		return 0, err
	}

	return int64(i), err
}

func (d *Base64Pipe) Write(p []byte) (int, error) {
	dst := make([]byte, base64.StdEncoding.DecodedLen(len(p)))

	decodedBytes, err := base64.StdEncoding.Decode(dst, p)
	if err != nil {
		return 0, err
	}

	return d.w.Write(dst[0:decodedBytes])
}
