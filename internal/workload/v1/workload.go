// Copyright 2021 VMware, Inc.
// SPDX-License-Identifier: MIT

package v1

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/vmware-tanzu-labs/object-code-generator-for-k8s/pkg/generate"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/vmware-tanzu-labs/operator-builder/internal/markers/inspect"
)

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
	PackageName string
}

// WorkloadSpec contains information required to generate source code.
type WorkloadSpec struct {
	Resources           []string `json:"resources" yaml:"resources"`
	APISpecFields       *APIFields
	SourceFiles         *[]SourceFile
	RBACRules           *RBACRules
	OwnershipRules      *OwnershipRules
	collection          bool
	collectionResources bool
}

func (ws *WorkloadSpec) init() {
	ws.APISpecFields = &APIFields{
		Name:   "Spec",
		Type:   FieldStruct,
		Tags:   fmt.Sprintf("`json: %q`", "spec"),
		Sample: "spec:",
	}

	ws.OwnershipRules = &OwnershipRules{}
	ws.RBACRules = &RBACRules{}
	ws.SourceFiles = &[]SourceFile{}
}

func (ws *WorkloadSpec) processManifests(workloadPath string, collection, collectionResources bool) error {
	ws.init()

	ws.collection = collection
	ws.collectionResources = collectionResources

	for _, manifestFile := range ws.Resources {
		// capture entire resource manifest file content
		manifests, err := ws.processMarkers(filepath.Join(filepath.Dir(workloadPath), manifestFile))
		if err != nil {
			return err
		}

		// determine sourceFile filename
		sourceFile := determineSourceFileName(manifestFile)

		var childResources []ChildResource

		for _, manifest := range manifests {
			// decode manifest into unstructured data type
			var manifestObject unstructured.Unstructured

			decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()

			err := runtime.DecodeInto(decoder, []byte(manifest), &manifestObject)
			if err != nil {
				return formatProcessError(manifestFile, err)
			}

			// generate a unique name for the resource using the kind and name
			resourceUniqueName := generateUniqueResourceName(manifestObject)
			// determine resource group and version
			resourceVersion, resourceGroup := versionGroupFromAPIVersion(manifestObject.GetAPIVersion())

			// determine group and resource for RBAC rule generation
			ws.RBACRules.addRulesForManifest(manifestObject.GetKind(), resourceGroup, manifestObject.Object)

			ws.OwnershipRules.addOrUpdateOwnership(
				manifestObject.GetAPIVersion(),
				manifestObject.GetKind(),
				resourceGroup,
			)

			resource := ChildResource{
				Name:       manifestObject.GetName(),
				UniqueName: resourceUniqueName,
				Group:      resourceGroup,
				Version:    resourceVersion,
				Kind:       manifestObject.GetKind(),
			}

			// generate the object source code
			resourceDefinition, err := generate.Generate([]byte(manifest), "resourceObj")
			if err != nil {
				return formatProcessError(manifestFile, err)
			}

			// add the source code to the resource
			resource.SourceCode = resourceDefinition
			resource.StaticContent = manifest

			childResources = append(childResources, resource)
		}

		sourceFile.Children = childResources

		if ws.SourceFiles == nil {
			ws.SourceFiles = &[]SourceFile{}
		}

		*ws.SourceFiles = append(*ws.SourceFiles, sourceFile)
	}

	// ensure no duplicate file names exist within the source files
	ws.deduplicateFileNames()

	return nil
}

func (ws *WorkloadSpec) processMarkers(manifestFile string) ([]string, error) {
	// capture entire resource manifest file content
	manifestContent, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		return nil, formatProcessError(manifestFile, err)
	}

	insp, err := InitializeMarkerInspector()
	if err != nil {
		return nil, formatProcessError(manifestFile, err)
	}

	nodes, markerResults, err := insp.InspectYAML(manifestContent, TransformYAML)
	if err != nil {
		return nil, formatProcessError(manifestFile, err)
	}

	buf := bytes.Buffer{}

	for _, node := range nodes {
		m, err := yaml.Marshal(node)
		if err != nil {
			return nil, formatProcessError(manifestFile, err)
		}

		mustWrite(buf.WriteString("---\n"))
		mustWrite(buf.Write(m))
	}

	manifestContent = buf.Bytes()

	err = ws.processMarkerResults(markerResults)
	if err != nil {
		return nil, formatProcessError(manifestFile, err)
	}

	// If processing manifests for collection resources there is no case
	// where there should be collection markers - they will result in
	// code that won't compile.  We will convert collection markers to
	// field markers for the sake of UX.
	if ws.collection && ws.collectionResources {
		// find & replace collection markers with field markers
		manifestContent = []byte(strings.ReplaceAll(string(manifestContent), "!!var collection", "!!var parent"))
	}

	manifests := extractManifests(manifestContent)

	return manifests, nil
}

func (ws *WorkloadSpec) processMarkerResults(markerResults []*inspect.YAMLResult) error {
	for _, markerResult := range markerResults {
		var defaultFound bool

		var sampleVal interface{}

		switch r := markerResult.Object.(type) {
		case FieldMarker:
			if ws.collection && !ws.collectionResources {
				continue
			}

			comments := []string{}

			if r.Description != nil {
				comments = append(comments, strings.Split(*r.Description, "\n")...)
			}

			if r.Default != nil {
				defaultFound = true
				sampleVal = r.Default
			} else {
				sampleVal = r.originalValue
			}

			if err := ws.APISpecFields.AddField(
				r.Name,
				r.Type,
				comments,
				sampleVal,
				defaultFound,
			); err != nil {
				return err
			}

		case CollectionFieldMarker:
			if !ws.collection {
				continue
			}

			comments := []string{}

			if r.Description != nil {
				comments = append(comments, strings.Split(*r.Description, "\n")...)
			}

			if r.Default != nil {
				defaultFound = true
				sampleVal = r.Default
			} else {
				sampleVal = r.originalValue
			}

			if err := ws.APISpecFields.AddField(
				r.Name,
				r.Type,
				comments,
				sampleVal,
				defaultFound,
			); err != nil {
				return err
			}

		default:
			continue
		}
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

func formatProcessError(manifestFile string, err error) error {
	return fmt.Errorf("error processing file %s; %w", manifestFile, err)
}

func generateUniqueResourceName(object unstructured.Unstructured) string {
	resourceName := strings.Replace(strings.Title(object.GetName()), "-", "", -1)
	resourceName = strings.Replace(resourceName, ".", "", -1)
	resourceName = strings.Replace(resourceName, ":", "", -1)
	resourceName = fmt.Sprintf("%s%s", object.GetKind(), resourceName)

	return resourceName
}

func versionGroupFromAPIVersion(apiVersion string) (version, group string) {
	apiVersionElements := strings.Split(apiVersion, "/")

	if len(apiVersionElements) == 1 {
		version = apiVersionElements[0]
		group = coreRBACGroup
	} else {
		version = apiVersionElements[1]
		group = rbacGroupFromGroup(apiVersionElements[0])
	}

	return version, group
}
