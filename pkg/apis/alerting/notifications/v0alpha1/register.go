package v0alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	common "github.com/grafana/grafana/pkg/apimachinery/apis/common/v0alpha1"
	timeInterval "github.com/grafana/grafana/pkg/apis/alerting/notifications/v0alpha1/timeinterval"
)

const (
	GROUP      = "notifications.alerting.grafana.app"
	VERSION    = "v0alpha1"
	APIVERSION = GROUP + "/" + VERSION
)

var TimeIntervalResourceInfo = common.NewResourceInfo(GROUP, VERSION,
	"time-intervals", "time-interval", "TimeIntervals",
	func() runtime.Object { return &timeInterval.TimeInterval{} },
	func() runtime.Object { return &timeInterval.TimeIntervalList{} },
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: GROUP, Version: VERSION}
	// SchemaBuilder is used by standard codegen
	SchemeBuilder      runtime.SchemeBuilder
	localSchemeBuilder = &SchemeBuilder
	AddToScheme        = localSchemeBuilder.AddToScheme
)

func init() {
	localSchemeBuilder.Register(AddKnownTypes)
}

// Adds the list of known types to the given scheme.
func AddKnownTypes(scheme *runtime.Scheme) error {
	return AddKnownTypesGroup(scheme, SchemeGroupVersion)
}

func AddKnownTypesGroup(scheme *runtime.Scheme, g schema.GroupVersion) error{
	scheme.AddKnownTypes(g,
		&timeInterval.TimeInterval{},
		&timeInterval.TimeIntervalList{},
	)
	metav1.AddToGroupVersion(scheme, g)
	return nil
}
