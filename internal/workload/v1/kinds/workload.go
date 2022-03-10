// Copyright 2021 VMware, Inc.
// SPDX-License-Identifier: MIT

package kinds

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/vmware-tanzu-labs/object-code-generator-for-k8s/pkg/generate"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/kubebuilder/v3/pkg/model/resource"

	"github.com/vmware-tanzu-labs/operator-builder/internal/markers/inspect"
	"github.com/vmware-tanzu-labs/operator-builder/internal/workload/v1/commands/companion"
	"github.com/vmware-tanzu-labs/operator-builder/internal/workload/v1/markers"
	"github.com/vmware-tanzu-labs/operator-builder/internal/workload/v1/rbac"
	"github.com/vmware-tanzu-labs/operator-builder/internal/workload/v1/resources"
)

// WorkloadAPISpec sample fields which may be used in things like testing or
// generation of sample files.
const (
	SampleWorkloadAPIDomain  = "acme.com"
	SampleWorkloadAPIGroup   = "apps"
	SampleWorkloadAPIKind    = "MyApp"
	SampleWorkloadAPIVersion = "v1alpha1"
)

// Workload defines an interface for identifying any workload.
type Workload interface {
	IsClusterScoped() bool
	IsStandalone() bool
	IsCollection() bool
	IsComponent() bool

	HasRootCmdName() bool
	HasSubCmdName() bool
	HasChildResources() bool

	GetWorkloadKind() WorkloadKind
	GetName() string
	GetPackageName() string
	GetDomain() string
	GetAPIGroup() string
	GetAPIVersion() string
	GetAPIKind() string
	GetDependencies() []*ComponentWorkload
	GetCollection() *WorkloadCollection
	GetComponents() []*ComponentWorkload
	GetSourceFiles() *[]resources.SourceFile
	GetAPISpecFields() *APIFields
	GetRBACRules() *[]rbac.Rule
	GetComponentResource(domain, repo string, clusterScoped bool) *resource.Resource
	GetFuncNames() (createFuncNames, initFuncNames []string)
	GetRootCommand() *companion.CLI
	GetSubCommand() *companion.CLI

	SetNames()
	SetRBAC()
	SetResources(workloadPath string) error
	SetComponents(components []*ComponentWorkload) error

	LoadManifests(workloadPath string) error
	Validate() error
}

// WorkloadAPISpec contains fields shared by all workload specs.
type WorkloadAPISpec struct {
	Domain        string `json:"domain" yaml:"domain"`
	Group         string `json:"group" yaml:"group"`
	Version       string `json:"version" yaml:"version"`
	Kind          string `json:"kind" yaml:"kind"`
	ClusterScoped bool   `json:"clusterScoped" yaml:"clusterScoped"`
}

// WorkloadShared contains fields shared by all workloads.
type WorkloadShared struct {
	Name        string       `json:"name"  yaml:"name" validate:"required"`
	Kind        WorkloadKind `json:"kind"  yaml:"kind" validate:"required"`
	PackageName string       `json:",omitempty" yaml:",omitempty" validate:"omitempty"`
}

// WorkloadSpec contains information required to generate source code.
type WorkloadSpec struct {
	Manifests              []*resources.Manifest            `json:"resources" yaml:"resources"`
	FieldMarkers           []*markers.FieldMarker           `json:",omitempty" yaml:",omitempty" validate:"omitempty"`
	CollectionFieldMarkers []*markers.CollectionFieldMarker `json:",omitempty" yaml:",omitempty" validate:"omitempty"`
	ForCollection          bool                             `json:",omitempty" yaml:",omitempty" validate:"omitempty"`
	Collection             *WorkloadCollection              `json:",omitempty" yaml:",omitempty" validate:"omitempty"`
	APISpecFields          *APIFields                       `json:",omitempty" yaml:",omitempty" validate:"omitempty"`
	SourceFiles            *[]resources.SourceFile          `json:",omitempty" yaml:",omitempty" validate:"omitempty"`
	RBACRules              *rbac.Rules                      `json:",omitempty" yaml:",omitempty" validate:"omitempty"`
}

// ProcessResourceMarkers processes a collection of field markers, associates them with
// their respective resource markers, and generates the source code needed for that particular
// resource marker.
func (ws *WorkloadSpec) ProcessResourceMarkers(markerCollection *markers.MarkerCollection) error {
	for _, sourceFile := range *ws.SourceFiles {
		for i := range sourceFile.Children {
			if err := sourceFile.Children[i].ProcessResourceMarkers(markerCollection); err != nil {
				return err
			}
		}
	}

	return nil
}

func (ws *WorkloadSpec) init() {
	ws.APISpecFields = &APIFields{
		Name:   "Spec",
		Type:   markers.FieldStruct,
		Tags:   fmt.Sprintf("`json: %q`", "spec"),
		Sample: "spec:",
	}

	// append the collection ref if we need to
	if ws.needsCollectionRef() {
		ws.appendCollectionRef()
	}

	ws.RBACRules = &rbac.Rules{}
	ws.SourceFiles = &[]resources.SourceFile{}
}

func (ws *WorkloadSpec) appendCollectionRef() {
	// ensure api spec and collection is already set
	if ws.APISpecFields == nil || ws.Collection == nil {
		return
	}

	// ensure we are adding to the spec field
	if ws.APISpecFields.Name != "Spec" {
		return
	}

	var sampleNamespace string

	if ws.Collection.IsClusterScoped() {
		sampleNamespace = ""
	} else {
		sampleNamespace = "default"
	}

	// append to children
	collectionField := &APIFields{
		Name:       "Collection",
		Type:       markers.FieldStruct,
		Tags:       fmt.Sprintf("`json:%q`", "collection"),
		Sample:     "#collection:",
		StructName: "CollectionSpec",
		Markers: []string{
			"+kubebuilder:validation:Optional",
			"Specifies a reference to the collection to use for this workload.",
			"Requires the name and namespace input to find the collection.",
			"If no collection field is set, default to selecting the only",
			"workload collection in the cluster, which will result in an error",
			"if not exactly one collection is found.",
		},
		Comments: nil,
		Children: []*APIFields{
			{
				Name:   "Name",
				Type:   markers.FieldString,
				Tags:   fmt.Sprintf("`json:%q`", "name"),
				Sample: fmt.Sprintf("#name: %q", strings.ToLower(ws.Collection.GetAPIKind())+"-sample"),
				Markers: []string{
					"+kubebuilder:validation:Required",
					"Required if specifying collection.  The name of the collection",
					"within a specific collection.namespace to reference.",
				},
			},
			{
				Name:   "Namespace",
				Type:   markers.FieldString,
				Tags:   fmt.Sprintf("`json:%q`", "namespace"),
				Sample: fmt.Sprintf("#namespace: %q", sampleNamespace),
				Markers: []string{
					"+kubebuilder:validation:Optional",
					"(Default: \"\") The namespace where the collection exists.  Required only if",
					"the collection is namespace scoped and not cluster scoped.",
				},
			},
		},
	}

	ws.APISpecFields.Children = append(ws.APISpecFields.Children, collectionField)
}

func NewSampleAPISpec() *WorkloadAPISpec {
	return &WorkloadAPISpec{
		Domain:        SampleWorkloadAPIDomain,
		Group:         SampleWorkloadAPIGroup,
		Kind:          SampleWorkloadAPIKind,
		Version:       SampleWorkloadAPIVersion,
		ClusterScoped: false,
	}
}

func (ws *WorkloadSpec) processManifests(markerTypes ...markers.MarkerType) error {
	ws.init()

	for _, manifestFile := range ws.Manifests {
		err := ws.processMarkers(manifestFile, markerTypes...)
		if err != nil {
			return err
		}

		// determine sourceFile filename
		sourceFile := resources.NewSourceFile(manifestFile)

		var childResources []resources.Child

		for _, manifest := range manifestFile.ExtractManifests() {
			// decode manifest into unstructured data type
			var manifestObject unstructured.Unstructured

			decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()

			if err := runtime.DecodeInto(decoder, []byte(manifest), &manifestObject); err != nil {
				return resources.ProcessManifestError(manifestFile, err)
			}

			// add the rules for this manifest
			rules, err := rbac.ForManifest(&manifestObject)
			if err != nil {
				return fmt.Errorf(
					"%w; error generating rbac for kind [%s] with name [%s]",
					resources.ProcessManifestError(manifestFile, err),
					manifestObject.GetKind(),
					manifestObject.GetName(),
				)
			}

			ws.RBACRules.Add(rules)

			resource := resources.Child{
				Name:       manifestObject.GetName(),
				UniqueName: generateUniqueResourceName(manifestObject),
				Group:      manifestObject.GetObjectKind().GroupVersionKind().Group,
				Version:    manifestObject.GetObjectKind().GroupVersionKind().Version,
				Kind:       manifestObject.GetKind(),
			}

			// generate the object source code
			resourceDefinition, err := generate.Generate([]byte(manifest), "resourceObj")
			if err != nil {
				return fmt.Errorf(
					"%w; error generating resource definition for kind [%s] with name [%s]",
					resources.ProcessManifestError(manifestFile, err),
					manifestObject.GetKind(),
					manifestObject.GetName(),
				)
			}

			// add the source code to the resource
			resource.SourceCode = resourceDefinition
			resource.StaticContent = manifest

			childResources = append(childResources, resource)
		}

		sourceFile.Children = childResources

		if ws.SourceFiles == nil {
			ws.SourceFiles = &[]resources.SourceFile{}
		}

		*ws.SourceFiles = append(*ws.SourceFiles, *sourceFile)
	}

	// ensure no duplicate file names exist within the source files
	ws.deduplicateFileNames()

	return nil
}

func (ws *WorkloadSpec) processMarkers(manifestFile *resources.Manifest, markerTypes ...markers.MarkerType) error {
	nodes, markerResults, err := markers.InspectForYAML(manifestFile.Content, markerTypes...)
	if err != nil {
		return resources.ProcessManifestError(manifestFile, err)
	}

	buf := bytes.Buffer{}

	for _, node := range nodes {
		m, err := yaml.Marshal(node)
		if err != nil {
			return resources.ProcessManifestError(manifestFile, err)
		}

		mustWrite(buf.WriteString("---\n"))
		mustWrite(buf.Write(m))
	}

	manifestFile.Content = buf.Bytes()

	if err = ws.processMarkerResults(markerResults); err != nil {
		return resources.ProcessManifestError(manifestFile, err)
	}

	// If processing manifests for collection resources there is no case
	// where there should be collection markers - they will result in
	// code that won't compile.  We will convert collection markers to
	// field markers for the sake of UX.
	if markers.ContainsMarkerType(markerTypes, markers.FieldMarkerType) &&
		markers.ContainsMarkerType(markerTypes, markers.CollectionMarkerType) {
		// find & replace collection markers with field markers
		manifestFile.Content = []byte(strings.ReplaceAll(string(manifestFile.Content), "!!var collection", "!!var parent"))
		manifestFile.Content = []byte(strings.ReplaceAll(string(manifestFile.Content), "!!start collection", "!!start parent"))
	}

	return nil
}

func (ws *WorkloadSpec) processResourceMarkers(markerCollection *markers.MarkerCollection) error {
	for _, sourceFile := range *ws.SourceFiles {
		for i := range sourceFile.Children {
			if err := sourceFile.Children[i].ProcessResourceMarkers(markerCollection); err != nil {
				return err
			}
		}
	}

	return nil
}

func (ws *WorkloadSpec) processMarkerResults(markerResults []*inspect.YAMLResult) error {
	for _, markerResult := range markerResults {
		var defaultFound bool

		var sampleVal interface{}

		// convert to interface
		var marker markers.FieldMarkerProcessor

		switch t := markerResult.Object.(type) {
		case *markers.FieldMarker:
			marker = t
			ws.FieldMarkers = append(ws.FieldMarkers, t)
		case *markers.CollectionFieldMarker:
			marker = t
			ws.CollectionFieldMarkers = append(ws.CollectionFieldMarkers, t)
		default:
			continue
		}

		// set the comments based on the description field of a field marker
		comments := []string{}

		if marker.GetDescription() != "" {
			comments = append(comments, strings.Split(marker.GetDescription(), "\n")...)
		}

		// set the sample value based on if a default was specified in the marker or not
		if marker.GetDefault() != nil {
			defaultFound = true
			sampleVal = marker.GetDefault()
		} else {
			sampleVal = marker.GetOriginalValue()
		}

		// add the field to the api specification
		if err := ws.APISpecFields.AddField(
			marker.GetName(),
			marker.GetFieldType(),
			comments,
			sampleVal,
			defaultFound,
		); err != nil {
			return err
		}

		marker.SetForCollection(ws.ForCollection)
	}

	return nil
}

// deduplicateFileNames dedeplicates the names of the files.  This is because
// we cannot guarantee that files exist in different directories and may have
// naming collisions.
func (ws *WorkloadSpec) deduplicateFileNames() {
	// create a slice to track existing fileNames and preallocate an existing
	// known conflict
	fileNames := make([]string, len(*ws.SourceFiles)+1)
	fileNames[len(fileNames)-1] = "resources.go"

	// dereference the sourcefiles
	sourceFiles := *ws.SourceFiles

	for i, sourceFile := range sourceFiles {
		var count int

		// deduplicate the file names
		for _, fileName := range fileNames {
			if fileName == "" {
				continue
			}

			if sourceFile.Filename == fileName {
				// increase the count which serves as an index to append
				count++

				// adjust the filename
				fields := strings.Split(sourceFile.Filename, ".go")
				sourceFiles[i].Filename = fmt.Sprintf("%s_%v.go", fields[0], count)
			}
		}

		fileNames[i] = sourceFile.Filename
	}
}

// needsCollectionRef determines if the workload spec needs a collection ref as
// part of its spec for determining which collection to use.  In this case, we
// want to check and see if a collection is set, but also ensure that this is not
// a workload spec that belongs to a collection, as nested collections are
// unsupported.
func (ws *WorkloadSpec) needsCollectionRef() bool {
	return ws.Collection != nil && !ws.ForCollection
}

func generateUniqueResourceName(object unstructured.Unstructured) string {
	resourceName := strings.ReplaceAll(strings.Title(object.GetName()), "-", "")
	resourceName = strings.ReplaceAll(resourceName, ".", "")
	resourceName = strings.ReplaceAll(resourceName, ":", "")
	resourceName = strings.ReplaceAll(resourceName, "!!Start", "")
	resourceName = strings.ReplaceAll(resourceName, "!!End", "")
	resourceName = strings.ReplaceAll(resourceName, "ParentSpec", "")
	resourceName = strings.ReplaceAll(resourceName, "CollectionSpec", "")
	resourceName = strings.ReplaceAll(resourceName, " ", "")

	namespaceName := strings.ReplaceAll(strings.Title(object.GetNamespace()), "-", "")
	namespaceName = strings.ReplaceAll(namespaceName, ".", "")
	namespaceName = strings.ReplaceAll(namespaceName, ":", "")
	namespaceName = strings.ReplaceAll(namespaceName, "!!Start", "")
	namespaceName = strings.ReplaceAll(namespaceName, "!!End", "")
	namespaceName = strings.ReplaceAll(namespaceName, "ParentSpec", "")
	namespaceName = strings.ReplaceAll(namespaceName, "CollectionSpec", "")
	namespaceName = strings.ReplaceAll(namespaceName, " ", "")

	resourceName = fmt.Sprintf("%s%s%s", object.GetKind(), namespaceName, resourceName)

	return resourceName
}