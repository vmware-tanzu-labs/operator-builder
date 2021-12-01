// Copyright 2021 VMware, Inc.
// SPDX-License-Identifier: MIT

package scaffolds

import (
	"fmt"
	"log"

	"github.com/spf13/afero"
	"sigs.k8s.io/kubebuilder/v3/pkg/config"
	"sigs.k8s.io/kubebuilder/v3/pkg/machinery"
	"sigs.k8s.io/kubebuilder/v3/pkg/model/resource"
	"sigs.k8s.io/kubebuilder/v3/pkg/plugins"

	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/api"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/api/common"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/api/resources"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/cli"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/config/crd"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/config/samples"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/controller"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/int/controllers/phases"
	controllersutils "github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/int/controllers/utils"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/int/dependencies"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/int/helpers"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/int/mutate"
	resourcespkg "github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/int/resources"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/int/wait"
	"github.com/vmware-tanzu-labs/operator-builder/internal/plugins/workload/v1/scaffolds/templates/test/e2e"
	workloadv1 "github.com/vmware-tanzu-labs/operator-builder/internal/workload/v1"
)

var _ plugins.Scaffolder = &apiScaffolder{}

type apiScaffolder struct {
	config             config.Config
	resource           *resource.Resource
	boilerplatePath    string
	workload           workloadv1.WorkloadAPIBuilder
	cliRootCommandName string

	fs machinery.Filesystem
}

// NewAPIScaffolder returns a new Scaffolder for project initialization operations.
func NewAPIScaffolder(
	cfg config.Config,
	res *resource.Resource,
	workload workloadv1.WorkloadAPIBuilder,
	cliRootCommandName string,
) plugins.Scaffolder {
	return &apiScaffolder{
		config:             cfg,
		resource:           res,
		boilerplatePath:    "hack/boilerplate.go.txt",
		workload:           workload,
		cliRootCommandName: cliRootCommandName,
	}
}

// InjectFS implements cmdutil.Scaffolder.
func (s *apiScaffolder) InjectFS(fs machinery.Filesystem) {
	s.fs = fs
}

//nolint:funlen //this will be refactored later
// scaffold implements cmdutil.Scaffolder.
func (s *apiScaffolder) Scaffold() error {
	log.Println("Building API...")

	boilerplate, err := afero.ReadFile(s.fs.FS, s.boilerplatePath)
	if err != nil {
		return fmt.Errorf("unable to read boilerplate file %s, %w", s.boilerplatePath, err)
	}

	scaffold := machinery.NewScaffold(s.fs,
		machinery.WithConfig(s.config),
		machinery.WithBoilerplate(string(boilerplate)),
		machinery.WithResource(s.resource),
	)

	createFuncNames, initFuncNames := s.workload.GetFuncNames()

	//nolint:nestif //this will be refactored later
	// API types
	if s.workload.IsStandalone() {
		err = scaffold.Execute(
			&templates.MainUpdater{
				WireResource:   true,
				WireController: true,
			},
			&api.Types{
				SpecFields:    s.workload.GetAPISpecFields(),
				ClusterScoped: s.workload.IsClusterScoped(),
				Dependencies:  s.workload.GetDependencies(),
				IsStandalone:  s.workload.IsStandalone(),
			},
			&common.Components{
				IsStandalone: s.workload.IsStandalone(),
			},
			&common.Conditions{},
			&common.Resources{},
			&resources.Resources{
				PackageName:     s.workload.GetPackageName(),
				CreateFuncNames: createFuncNames,
				InitFuncNames:   initFuncNames,
				IsComponent:     s.workload.IsComponent(),
			},
			&resourcespkg.Resources{},
			&controller.Controller{
				PackageName:       s.workload.GetPackageName(),
				RBACRules:         s.workload.GetRBACRules(),
				OwnershipRules:    s.workload.GetOwnershipRules(),
				HasChildResources: s.workload.HasChildResources(),
				IsStandalone:      s.workload.IsStandalone(),
				IsComponent:       s.workload.IsComponent(),
			},
			&controller.SuiteTest{},
			&controllersutils.Utils{
				IsStandalone: s.workload.IsStandalone(),
			},
			&controllersutils.RateLimiter{},
			&phases.Types{},
			&phases.Common{},
			&phases.CreateResource{
				IsStandalone: s.workload.IsStandalone(),
			},
			&phases.ResourcePersist{},
			&phases.Dependencies{},
			&phases.PreFlight{},
			&phases.ResourceWait{},
			&phases.CheckReady{},
			&phases.Complete{},
			&helpers.Common{},
			&helpers.Component{},
			&dependencies.Component{},
			&mutate.Component{},
			&wait.Component{},
			&samples.CRDSample{
				SpecFields: s.workload.GetAPISpecFields(),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to scaffold standalone workload, %w", err)
		}

		if err := s.scaffoldE2ETests(scaffold, s.workload); err != nil {
			return fmt.Errorf("unable to scaffold standalone workload e2e tests, %w", err)
		}
	} else {
		// collection API
		err = scaffold.Execute(
			&templates.MainUpdater{
				WireResource:   true,
				WireController: true,
			},
			&api.Types{
				SpecFields:    s.workload.GetAPISpecFields(),
				ClusterScoped: s.workload.IsClusterScoped(),
				Dependencies:  s.workload.GetDependencies(),
				IsStandalone:  s.workload.IsStandalone(),
			},
			&common.Components{
				IsStandalone: s.workload.IsStandalone(),
			},
			&common.Conditions{},
			&common.Resources{},
			&resources.Resources{
				PackageName:     s.workload.GetPackageName(),
				CreateFuncNames: createFuncNames,
				InitFuncNames:   initFuncNames,
				IsComponent:     s.workload.IsComponent(),
			},
			&resourcespkg.Resources{},
			&controller.Controller{
				PackageName:       s.workload.GetPackageName(),
				RBACRules:         s.workload.GetRBACRules(),
				OwnershipRules:    s.workload.GetOwnershipRules(),
				HasChildResources: s.workload.HasChildResources(),
				IsStandalone:      s.workload.IsStandalone(),
				IsComponent:       s.workload.IsComponent(),
			},
			&controller.SuiteTest{},
			&controllersutils.Utils{
				IsStandalone: s.workload.IsStandalone(),
			},
			&controllersutils.RateLimiter{},
			&phases.Types{},
			&phases.Common{},
			&phases.CreateResource{
				IsStandalone: s.workload.IsStandalone(),
			},
			&phases.ResourcePersist{},
			&phases.Dependencies{},
			&phases.PreFlight{},
			&phases.ResourceWait{},
			&phases.CheckReady{},
			&phases.Complete{},
			&helpers.Common{},
			&helpers.Component{},
			&dependencies.Component{},
			&mutate.Component{},
			&wait.Component{},
			&samples.CRDSample{
				SpecFields: s.workload.GetAPISpecFields(),
			},
			&crd.Kustomization{},
		)
		if err != nil {
			return fmt.Errorf("unable to scaffold collection workload, %w", err)
		}

		if err := s.scaffoldE2ETests(scaffold, s.workload); err != nil {
			return fmt.Errorf("unable to scaffold collection workload e2e tests, %w", err)
		}

		for _, component := range s.workload.GetComponents() {
			componentScaffold := machinery.NewScaffold(s.fs,
				machinery.WithConfig(s.config),
				machinery.WithBoilerplate(string(boilerplate)),
				machinery.WithResource(component.GetComponentResource(
					s.config.GetDomain(),
					s.config.GetRepository(),
					component.IsClusterScoped(),
				)),
			)

			createFuncNames, initFuncNames := component.GetFuncNames()

			err = componentScaffold.Execute(
				&templates.MainUpdater{
					WireResource:   true,
					WireController: true,
				},
				&api.Types{
					SpecFields:    component.Spec.APISpecFields,
					ClusterScoped: component.IsClusterScoped(),
					Dependencies:  component.GetDependencies(),
					IsStandalone:  component.IsStandalone(),
				},
				&api.Group{},
				&resources.Resources{
					PackageName:     component.GetPackageName(),
					CreateFuncNames: createFuncNames,
					InitFuncNames:   initFuncNames,
					IsComponent:     component.IsComponent(),
					Collection:      s.workload.(*workloadv1.WorkloadCollection),
				},
				&controller.Controller{
					PackageName:       component.GetPackageName(),
					RBACRules:         component.GetRBACRules(),
					OwnershipRules:    component.GetOwnershipRules(),
					HasChildResources: component.HasChildResources(),
					IsStandalone:      component.IsStandalone(),
					IsComponent:       component.IsComponent(),
					Collection:        s.workload.(*workloadv1.WorkloadCollection),
				},
				&controller.SuiteTest{},
				&dependencies.Component{},
				&mutate.Component{},
				&helpers.Component{},
				&wait.Component{},
				&samples.CRDSample{
					SpecFields: component.Spec.APISpecFields,
				},
				&crd.Kustomization{},
			)
			if err != nil {
				return fmt.Errorf("unable to scaffold component workload %s, %w", component.Name, err)
			}

			if err := s.scaffoldE2ETests(componentScaffold, component); err != nil {
				return fmt.Errorf("unable to scaffold component workload e2e tests, %w", err)
			}

			// component child resource definition files
			// these are the resources defined in the static yaml manifests
			for _, sourceFile := range *component.GetSourceFiles() {
				scaffold := machinery.NewScaffold(s.fs,
					machinery.WithConfig(s.config),
					machinery.WithBoilerplate(string(boilerplate)),
					machinery.WithResource(component.GetComponentResource(
						s.config.GetDomain(),
						s.config.GetRepository(),
						component.IsClusterScoped(),
					)),
				)

				err = scaffold.Execute(
					&resources.Definition{
						ClusterScoped: component.IsClusterScoped(),
						SourceFile:    sourceFile,
						PackageName:   component.GetPackageName(),
						IsComponent:   component.IsComponent(),
						Collection:    s.workload.(*workloadv1.WorkloadCollection),
					},
				)
				if err != nil {
					return fmt.Errorf("unable to scaffold component workload resource files for %s, %w", component.Name, err)
				}
			}
		}
	}

	// child resource definition files
	// these are the resources defined in the static yaml manifests
	for _, sourceFile := range *s.workload.GetSourceFiles() {
		scaffold := machinery.NewScaffold(s.fs,
			machinery.WithConfig(s.config),
			machinery.WithBoilerplate(string(boilerplate)),
			machinery.WithResource(s.resource),
		)

		err = scaffold.Execute(
			&resources.Definition{
				ClusterScoped: s.workload.IsClusterScoped(),
				SourceFile:    sourceFile,
				PackageName:   s.workload.GetPackageName(),
				IsComponent:   s.workload.IsComponent(),
			},
		)
		if err != nil {
			return fmt.Errorf("unable to scaffold resource files, %w", err)
		}
	}

	// scaffold the companion CLI last only if we have a root command name
	if s.cliRootCommandName != "" {
		if err = s.scaffoldCLI(scaffold); err != nil {
			return fmt.Errorf("error scaffolding CLI; %w", err)
		}
	}

	return nil
}

// scaffoldCLI runs the specific logic to scaffold the companion CLI
//nolint:funlen,gocyclo //this will be refactored later
func (s *apiScaffolder) scaffoldCLI(scaffold *machinery.Scaffold) error {
	// create a root command object in memory
	rootCommand := workloadv1.CliCommand{
		Name:    s.workload.GetRootcommandName(),
		VarName: s.workload.GetRootcommandVarName(),
	}

	// obtain a list of workload commands to generate, to include the parent collection
	// and its children
	workloadCommands := make([]workloadv1.WorkloadAPIBuilder, len(s.workload.GetComponents())+1)
	workloadCommands[0] = s.workload

	if len(s.workload.GetComponents()) > 0 {
		for i, component := range s.workload.GetComponents() {
			workloadCommands[i+1] = component
		}
	}

	// scaffold the common code
	if err := scaffold.Execute(&cli.CmdUtils{Builder: s.workload}); err != nil {
		return fmt.Errorf("unable to scaffold companion cli utility code; %w", err)
	}

	for _, workloadCommand := range workloadCommands {
		// create subcommand object in memory
		subCommand := workloadv1.CliCommand{
			Name:        workloadCommand.GetSubcommandName(),
			VarName:     workloadCommand.GetSubcommandVarName(),
			Description: workloadCommand.GetSubcommandDescr(),
			FileName:    workloadCommand.GetSubcommandFileName(),
		}

		// scaffold init subcommand
		if err := scaffold.Execute(
			&cli.CmdInitSub{
				RootCmd:      rootCommand,
				SubCmd:       subCommand,
				SpecFields:   workloadCommand.GetAPISpecFields(),
				IsComponent:  workloadCommand.IsComponent() || workloadCommand.IsCollection(),
				IsStandalone: workloadCommand.IsStandalone(),
				ComponentResource: workloadCommand.GetComponentResource(
					s.config.GetDomain(),
					s.config.GetRepository(),
					workloadCommand.IsClusterScoped(),
				),
			},
			&cli.CmdInitSubLatest{
				RootCmd:     rootCommand,
				SubCmd:      subCommand,
				IsComponent: workloadCommand.IsComponent() || workloadCommand.IsCollection(),
				ComponentResource: workloadCommand.GetComponentResource(
					s.config.GetDomain(),
					s.config.GetRepository(),
					workloadCommand.IsClusterScoped(),
				),
			},
			&cli.CmdInitSubUpdater{
				RootCmd:     rootCommand,
				SubCmd:      subCommand,
				SpecFields:  workloadCommand.GetAPISpecFields(),
				IsComponent: workloadCommand.IsComponent() || workloadCommand.IsCollection(),
				ComponentResource: workloadCommand.GetComponentResource(
					s.config.GetDomain(),
					s.config.GetRepository(),
					workloadCommand.IsClusterScoped(),
				),
			},
		); err != nil {
			return fmt.Errorf("unable to scaffold init subcommand, %w", err)
		}

		// build generate subcommand
		generateSubCommand := &cli.CmdGenerateSub{
			PackageName:  workloadCommand.GetPackageName(),
			RootCmd:      rootCommand,
			SubCmd:       subCommand,
			IsComponent:  workloadCommand.IsComponent() || workloadCommand.IsCollection(),
			IsCollection: workloadCommand.IsCollection(),
			IsStandalone: workloadCommand.IsStandalone(),
		}
		generateSubUpdaterCommand := &cli.CmdGenerateSubUpdater{
			PackageName:  workloadCommand.GetPackageName(),
			RootCmd:      rootCommand,
			SubCmd:       subCommand,
			IsComponent:  workloadCommand.IsComponent() || workloadCommand.IsCollection(),
			IsCollection: workloadCommand.IsCollection(),
			IsStandalone: workloadCommand.IsStandalone(),
		}

		if workloadCommand.IsCollection() || workloadCommand.IsComponent() {
			// scaffold the initial base subcommand first
			//nolint:forcetypeassert // this will be refactored later
			generateSubCommand.Collection = s.workload.(*workloadv1.WorkloadCollection)
			generateSubCommand.ComponentResource = workloadCommand.GetComponentResource(
				s.config.GetDomain(),
				s.config.GetRepository(),
				workloadCommand.IsClusterScoped(),
			)

			// update the scaffold with specific version information
			//nolint:forcetypeassert // this will be refactored later
			generateSubUpdaterCommand.Collection = s.workload.(*workloadv1.WorkloadCollection)
			generateSubUpdaterCommand.ComponentResource = workloadCommand.GetComponentResource(
				s.config.GetDomain(),
				s.config.GetRepository(),
				workloadCommand.IsClusterScoped(),
			)
		}

		// scaffold the generate command unless we have a collection without resources
		if (workloadCommand.HasChildResources() && workloadCommand.IsCollection()) || !workloadCommand.IsCollection() {
			if err := scaffold.Execute(generateSubCommand, generateSubUpdaterCommand); err != nil {
				return fmt.Errorf("unable to scaffold generate subcommand, %w", err)
			}
		}

		// scaffold version subcommand
		if err := scaffold.Execute(
			&cli.CmdVersionSub{
				RootCmd:      rootCommand,
				SubCmd:       subCommand,
				IsStandalone: workloadCommand.IsStandalone(),
				IsComponent:  workloadCommand.IsComponent() || workloadCommand.IsCollection(),
				ComponentResource: workloadCommand.GetComponentResource(
					s.config.GetDomain(),
					s.config.GetRepository(),
					workloadCommand.IsClusterScoped(),
				),
			},
			&cli.CmdVersionSubUpdater{
				RootCmd:     rootCommand,
				SubCmd:      subCommand,
				IsComponent: workloadCommand.IsComponent() || workloadCommand.IsCollection(),
			},
		); err != nil {
			return fmt.Errorf("unable to scaffold version subcommand, %w", err)
		}

		// scaffold the root command
		if err := scaffold.Execute(
			&cli.CmdRootUpdater{
				RootCmdName:     rootCommand.Name,
				InitCommand:     true,
				GenerateCommand: true,
				VersionCommand:  true,
				Builder:         workloadCommand,
			},
		); err != nil {
			return fmt.Errorf("error updating root.go, %w", err)
		}
	}

	return nil
}

// scaffoldE2ETests run the specific logic to scaffold the end to end tests.
func (s *apiScaffolder) scaffoldE2ETests(
	scaffold *machinery.Scaffold,
	workload workloadv1.WorkloadAPIBuilder,
) error {
	e2eWorkloadBuilder := &e2e.WorkloadTestUpdater{
		HasChildResources: workload.HasChildResources(),
		IsStandalone:      workload.IsStandalone(),
		IsComponent:       workload.IsComponent(),
		IsCollection:      workload.IsCollection(),
		PackageName:       workload.GetPackageName(),
		IsClusterScoped:   workload.IsClusterScoped(),
	}

	if !s.workload.IsStandalone() {
		collection, ok := s.workload.(*workloadv1.WorkloadCollection)
		if !ok {
			//nolint: goerr113
			return fmt.Errorf("unable to convert workload to collection")
		}

		e2eWorkloadBuilder.Collection = collection
	}

	//nolint: wrapcheck
	return scaffold.Execute(
		&e2e.Test{},
		&e2e.WorkloadTest{},
		e2eWorkloadBuilder,
	)
}
