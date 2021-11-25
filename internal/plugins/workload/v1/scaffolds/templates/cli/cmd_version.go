// Copyright 2021 VMware, Inc.
// SPDX-License-Identifier: MIT

package cli

import (
	"path/filepath"

	"sigs.k8s.io/kubebuilder/v3/pkg/machinery"
)

const (
	versionCommandName  = "version"
	versionCommandDescr = "Display the version information"
)

var _ machinery.Template = &CmdVersion{}

// CmdVersion scaffolds the companion CLI's version subcommand for
// component workloads.  The version logic will live in the workload's
// subcommand to this command; see cmd_version_sub.go.
type CmdVersion struct {
	machinery.TemplateMixin
	machinery.BoilerplateMixin

	RootCmdName string

	IsCollection bool

	VersionCommandName  string
	VersionCommandDescr string
}

func (f *CmdVersion) SetTemplateDefaults() error {
	f.Path = filepath.Join("cmd", f.RootCmdName, "commands", "version", "version.go")
	f.TemplateBody = cliCmdVersionTemplate

	f.VersionCommandName = versionCommandName
	f.VersionCommandDescr = versionCommandDescr

	return nil
}

const cliCmdVersionTemplate = `{{ .Boilerplate }}

package version

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var CliVersion = "dev"

type VersionInfo struct {
	CLIVersion  string   ` + "`" + `json:"cliVersion"` + "`" + `
	APIVersions []string ` + "`" + `json:"apiVersions"` + "`" + `
}

type VersionFunc func(*VersionSubCommand) error

type VersionSubCommand struct {
	*cobra.Command

	// options
	Name         string
	Description  string
	SubCommandOf *cobra.Command

	VersionFunc VersionFunc
}

{{ if .IsCollection }}
// NewBaseVersionSubCommand returns a subcommand that is meant to belong to a parent
// subcommand but have subcommands itself.
func NewBaseVersionSubCommand(parentCommand *cobra.Command) *VersionSubCommand {
	versionCmd := &VersionSubCommand{
		Name:         "{{ .VersionCommandName }}",
		Description:  "{{ .VersionCommandDescr }}",
		SubCommandOf: parentCommand,
	}

	versionCmd.Setup()

	return versionCmd
}
{{ end }}

// Setup sets up this command to be used as a command.
func (v *VersionSubCommand) Setup() {
	v.Command = &cobra.Command{
		Use:   v.Name,
		Short: v.Description,
		Long:  v.Description,
	}

	// run the version function if the function signature is set
	if v.VersionFunc != nil {
		v.RunE = v.version
	}

	// add this as a subcommand of another command if set
	if v.SubCommandOf != nil {
		v.SubCommandOf.AddCommand(v.Command)
	}
}

// version run the function to display version information about a workload.
func (v *VersionSubCommand) version(cmd *cobra.Command, args []string) error {
	return v.VersionFunc(v)
}

// Display will parse and print the information stored on the VersionInfo object.
func (v *VersionInfo) Display() error {
	output, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to determine versionInfo, %s", err)
	}

	outputStream := os.Stdout

	if _, err := outputStream.WriteString(fmt.Sprintln(string(output))); err != nil {
		return fmt.Errorf("failed to write to stdout, %s", err)
	}

	return nil
}
`
