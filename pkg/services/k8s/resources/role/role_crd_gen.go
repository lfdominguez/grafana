// Code generated - EDITING IS FUTILE. DO NOT EDIT.
//
// Generated by:
//     kinds/gen.go
// Using jennies:
//     CRDTypesJenny
//
// Run 'make gen-cue' from repository root to regenerate.

package role

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana/pkg/kinds/role"
	"github.com/grafana/grafana/pkg/registry/corekind"
	"github.com/grafana/grafana/pkg/services/k8s/crd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var coreReg = corekind.NewBase(nil)
var Kind = coreReg.Role()

var CRD = crd.Kind{
	GrafanaKind: Kind,
	Object:      &Role{},
	ObjectList:  &RoleList{},
}

// The CRD YAML representation of the Role kind.
//
//go:embed role.crd.yml
var CRDYaml []byte

// Role is the Go CRD representation of a single Role object.
// It implements [runtime.Object], and is used in k8s scheme construction.
type Role struct {
	crd.Base[role.Role]
}

func (roleCRD *Role) UnmarshalJSON(data []byte) error {
	m := make(map[string]interface{})
	json.Unmarshal(data, &m)

	u := &unstructured.Unstructured{}
	u.SetUnstructuredContent(m)

	obj, err := fromUnstructured(u)
	if err != nil {
		return err
	}

	*roleCRD = *obj
	return nil
}

// RoleList is the Go CRD representation of a list Role objects.
// It implements [runtime.Object], and is used in k8s scheme construction.
type RoleList struct {
	crd.ListBase[role.Role]
}

// fromUnstructured converts an *unstructured.Unstructured object to a *Role.
func fromUnstructured(obj any) (*Role, error) {
	uObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("failed to convert to *unstructured.Unstructured")
	}

	var role crd.Base[role.Role]
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(uObj.UnstructuredContent(), &role)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to Role: %w", err)
	}

	return &Role{role}, nil
}

// toUnstructured converts a Role to an *unstructured.Unstructured.
func toUnstructured(obj *role.Role, metadata metav1.ObjectMeta) (*unstructured.Unstructured, error) {
	roleObj := crd.Base[role.Role]{
		TypeMeta: metav1.TypeMeta{
			Kind:       CRD.GVK().Kind,
			APIVersion: CRD.GVK().Group + "/" + CRD.GVK().Version,
		},
		ObjectMeta: metadata,
		Spec:       *obj,
	}

	out, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&roleObj)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{
		Object: out,
	}, nil
}
