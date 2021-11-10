// Copyright 2021 VMware, Inc.
// SPDX-License-Identifier: MIT

package common

import (
	"path/filepath"

	"sigs.k8s.io/kubebuilder/v3/pkg/machinery"
)

var _ machinery.Template = &Components{}

// Components scaffolds the interfaces between workloads.
type Components struct {
	machinery.TemplateMixin
	machinery.BoilerplateMixin

	IsStandalone bool
}

func (f *Components) SetTemplateDefaults() error {
	f.Path = filepath.Join("apis", "common", "components.go")

	f.TemplateBody = commonTemplate

	return nil
}

const commonTemplate = `
// +build !ignore_autogenerated

{{ .Boilerplate }}
package common

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

type Component interface {
	GetComponentGVK() schema.GroupVersionKind
	GetDependencies() []Component
	GetDependencyStatus() bool
	GetReadyStatus() bool
	GetPhaseConditions() []*PhaseCondition
	GetResources() []*Resource

	SetReadyStatus(bool)
	SetDependencyStatus(bool)
	SetPhaseCondition(*PhaseCondition)
	SetResource(*Resource)
}

type ComponentReconciler interface {
	// attribute exporters and setters
	GetClient() client.Client
	GetComponent() Component
	GetContext() context.Context
	GetController() controller.Controller
	GetLogger() logr.Logger
	GetScheme() *runtime.Scheme
	GetResources() ([]metav1.Object, error)
	GetWatches() []client.Object
	SetWatch(client.Object)

	// component and child resource methods
	CreateOrUpdate(client.Object) error
	UpdateStatus() error

	// methods from the underlying client package
	Get(context.Context, types.NamespacedName, client.Object) error
	List(context.Context, client.ObjectList, ...client.ListOption) error
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error

	// custom methods which are managed by consumers
	CheckReady() (bool, error)
	Mutate(metav1.Object) ([]metav1.Object, bool, error)
	Wait(metav1.Object) (bool, error)
}
`
