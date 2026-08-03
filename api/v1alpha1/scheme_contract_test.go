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
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// TestSchemeRuntimeContract exercises the public runtime contract used by
// controller-runtime clients. The source-level registration test catches an
// omitted type; this test additionally catches registrations that compile but
// produce the wrong GVK or cannot be encoded and decoded by Kubernetes.
func TestSchemeRuntimeContract(t *testing.T) {
	t.Parallel()

	scheme := newAPIScheme(t)
	typeNames := sortedSet(findKubebuilderRootTypes(t, currentSourceDirectory(t)))
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	for _, typeName := range typeNames {
		t.Run(typeName, func(t *testing.T) {
			t.Parallel()

			gvk := GroupVersion.WithKind(typeName)
			object, err := scheme.New(gvk)
			if err != nil {
				t.Fatalf("create %s from scheme: %v", gvk, err)
			}

			assertObjectHasOnlyGVK(t, scheme, object, gvk)
			assertDeepCopyContract(t, object)
			assertJSONRoundTrip(t, scheme, decoder, object, gvk)
		})
	}
}

func TestSchemeContainsResourceAndListPairs(t *testing.T) {
	t.Parallel()

	scheme := newAPIScheme(t)
	typeNames := findKubebuilderRootTypes(t, currentSourceDirectory(t))

	for typeName := range typeNames {
		if strings.HasSuffix(typeName, "List") {
			continue
		}

		listTypeName := typeName + "List"
		if _, exists := typeNames[listTypeName]; !exists {
			t.Errorf("API resource %s has no root list type %s", typeName, listTypeName)
			continue
		}

		if _, err := scheme.New(GroupVersion.WithKind(listTypeName)); err != nil {
			t.Errorf("list type %s is not usable through the scheme: %v", listTypeName, err)
		}
	}
}

func TestAddToSchemeIsIdempotent(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	for call := 1; call <= 2; call++ {
		if err := AddToScheme(scheme); err != nil {
			t.Fatalf("AddToScheme call %d: %v", call, err)
		}
	}

	for typeName := range findKubebuilderRootTypes(t, currentSourceDirectory(t)) {
		if _, err := scheme.New(GroupVersion.WithKind(typeName)); err != nil {
			t.Errorf("type %s missing after repeated registration: %v", typeName, err)
		}
	}
}

func TestGroupVersionContract(t *testing.T) {
	t.Parallel()

	if GroupVersion.Group != "github.k8sready.com" {
		t.Errorf("unexpected API group %q", GroupVersion.Group)
	}
	if GroupVersion.Version != "v1alpha1" {
		t.Errorf("unexpected API version %q", GroupVersion.Version)
	}
	if got := GroupVersion.String(); got != "github.k8sready.com/v1alpha1" {
		t.Errorf("unexpected group/version string %q", got)
	}
}

func newAPIScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("register API scheme: %v", err)
	}
	return scheme
}

func assertObjectHasOnlyGVK(
	t *testing.T,
	scheme *runtime.Scheme,
	object runtime.Object,
	want schema.GroupVersionKind,
) {
	t.Helper()

	gvks, unversioned, err := scheme.ObjectKinds(object)
	if err != nil {
		t.Fatalf("resolve object kinds: %v", err)
	}
	if unversioned {
		t.Fatal("custom resource was registered as unversioned")
	}
	if len(gvks) != 1 || gvks[0] != want {
		t.Fatalf("unexpected GVKs: got %v, want [%s]", gvks, want)
	}
}

func assertDeepCopyContract(t *testing.T, object runtime.Object) {
	t.Helper()

	copy := object.DeepCopyObject()
	if copy == nil {
		t.Fatal("DeepCopyObject returned nil")
	}
	if reflect.TypeOf(copy) != reflect.TypeOf(object) {
		t.Fatalf("DeepCopyObject changed type from %T to %T", object, copy)
	}
	if reflect.ValueOf(copy).Pointer() == reflect.ValueOf(object).Pointer() {
		t.Fatal("DeepCopyObject returned the original pointer")
	}
	if !reflect.DeepEqual(copy, object) {
		t.Fatalf("DeepCopyObject changed the object: got %#v, want %#v", copy, object)
	}
}

func assertJSONRoundTrip(
	t *testing.T,
	scheme *runtime.Scheme,
	decoder runtime.Decoder,
	object runtime.Object,
	wantGVK schema.GroupVersionKind,
) {
	t.Helper()

	encoder := serializer.NewCodecFactory(scheme).LegacyCodec(GroupVersion)
	var encoded bytes.Buffer
	if err := encoder.Encode(object, &encoded); err != nil {
		t.Fatalf("encode object: %v", err)
	}

	var typeMeta struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(encoded.Bytes(), &typeMeta); err != nil {
		t.Fatalf("decode JSON type metadata: %v", err)
	}
	if typeMeta.APIVersion != GroupVersion.String() || typeMeta.Kind != wantGVK.Kind {
		t.Fatalf(
			"unexpected JSON type metadata: got %s/%s, want %s/%s",
			typeMeta.APIVersion,
			typeMeta.Kind,
			GroupVersion,
			wantGVK.Kind,
		)
	}

	decoded, actualGVK, err := decoder.Decode(encoded.Bytes(), nil, nil)
	if err != nil {
		t.Fatalf("decode object: %v", err)
	}
	if *actualGVK != wantGVK {
		t.Fatalf("decoded GVK is %s, want %s", actualGVK, wantGVK)
	}
	if reflect.TypeOf(decoded) != reflect.TypeOf(object) {
		t.Fatalf("decoded type is %T, want %T", decoded, object)
	}
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
