/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const kubebuilderRootMarker = "+kubebuilder:object:root=true"

// TestAllRootTypesAreRegistered ensures every Kubernetes root type declared in
// this API package is registered with the package SchemeBuilder.
//
// This test intentionally discovers root types from the Kubebuilder markers,
// so adding a new CRD or List type without registering it makes the test fail.
// It supports both SchemeBuilder.Register and scheme.AddKnownTypes patterns.
func TestAllRootTypesAreRegistered(t *testing.T) {
	t.Helper()

	apiDirectory := currentSourceDirectory(t)

	expected := findKubebuilderRootTypes(t, apiDirectory)
	registered := findSchemeRegistrations(t, apiDirectory)

	missing := difference(expected, registered)
	unexpected := difference(registered, expected)

	if len(missing) == 0 && len(unexpected) == 0 {
		return
	}

	t.Fatalf(
		"scheme registrations are out of sync with the API root types\n"+
			"missing registrations: %v\n"+
			"unexpected registrations: %v",
		missing,
		unexpected,
	)
}

func currentSourceDirectory(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test source directory")
	}

	return filepath.Dir(filename)
}

func findKubebuilderRootTypes(
	t *testing.T,
	apiDirectory string,
) map[string]struct{} {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(apiDirectory, "*_types.go"))
	if err != nil {
		t.Fatalf("find API type files: %v", err)
	}

	if len(matches) == 0 {
		t.Fatal("no *_types.go files found")
	}

	rootTypes := make(map[string]struct{})
	fileSet := token.NewFileSet()

	for _, filename := range matches {
		parsedFile, parseErr := parser.ParseFile(
			fileSet,
			filename,
			nil,
			parser.ParseComments,
		)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", filename, parseErr)
		}

		typeDeclarations := make([]*ast.TypeSpec, 0)
		for _, declaration := range parsedFile.Decls {
			genericDeclaration, ok := declaration.(*ast.GenDecl)
			if !ok || genericDeclaration.Tok != token.TYPE {
				continue
			}

			for _, specification := range genericDeclaration.Specs {
				typeSpecification, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}

				typeDeclarations = append(typeDeclarations, typeSpecification)
			}
		}

		for _, commentGroup := range parsedFile.Comments {
			if !containsRootMarker(commentGroup) {
				continue
			}

			for _, typeDeclaration := range typeDeclarations {
				if typeDeclaration.Pos() <= commentGroup.End() {
					continue
				}

				rootTypes[typeDeclaration.Name.Name] = struct{}{}
				break
			}
		}
	}

	if len(rootTypes) == 0 {
		t.Fatalf(
			"no types marked with %q were found",
			kubebuilderRootMarker,
		)
	}

	return rootTypes
}

func findSchemeRegistrations(
	t *testing.T,
	apiDirectory string,
) map[string]struct{} {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(apiDirectory, "*.go"))
	if err != nil {
		t.Fatalf("find API Go files: %v", err)
	}

	registeredTypes := make(map[string]struct{})
	fileSet := token.NewFileSet()

	for _, filename := range matches {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		parsedFile, parseErr := parser.ParseFile(fileSet, filename, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", filename, parseErr)
		}

		ast.Inspect(parsedFile, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			arguments := call.Args
			switch selector.Sel.Name {
			case "Register":
				identifier, ok := selector.X.(*ast.Ident)
				if !ok || identifier.Name != "SchemeBuilder" {
					return true
				}
			case "AddKnownTypes":
				// The first argument is GroupVersion.
				if len(arguments) == 0 {
					return true
				}
				arguments = arguments[1:]
			default:
				return true
			}

			for _, argument := range arguments {
				typeName, typeErr := registeredTypeName(argument)
				if typeErr != nil {
					t.Fatalf(
						"inspect scheme registration in %s: %v",
						filename,
						typeErr,
					)
				}

				registeredTypes[typeName] = struct{}{}
			}

			return true
		})
	}

	if len(registeredTypes) == 0 {
		t.Fatalf("no API scheme registrations found in %s", apiDirectory)
	}

	return registeredTypes
}

func registeredTypeName(expression ast.Expr) (string, error) {
	addressExpression, ok := expression.(*ast.UnaryExpr)
	if !ok || addressExpression.Op != token.AND {
		return "", &invalidRegistrationError{
			expressionType: expression,
		}
	}

	compositeLiteral, ok := addressExpression.X.(*ast.CompositeLit)
	if !ok {
		return "", &invalidRegistrationError{
			expressionType: expression,
		}
	}

	identifier, ok := compositeLiteral.Type.(*ast.Ident)
	if !ok {
		return "", &invalidRegistrationError{
			expressionType: expression,
		}
	}

	return identifier.Name, nil
}

type invalidRegistrationError struct {
	expressionType ast.Expr
}

func (e *invalidRegistrationError) Error() string {
	return "expected registration in the form &TypeName{}"
}

func containsRootMarker(groups ...*ast.CommentGroup) bool {
	for _, group := range groups {
		if group == nil {
			continue
		}

		for _, comment := range group.List {
			if strings.Contains(comment.Text, kubebuilderRootMarker) {
				return true
			}
		}
	}

	return false
}

func difference(
	left map[string]struct{},
	right map[string]struct{},
) []string {
	result := make([]string, 0)

	for value := range left {
		if _, exists := right[value]; !exists {
			result = append(result, value)
		}
	}

	sort.Strings(result)

	return result
}
