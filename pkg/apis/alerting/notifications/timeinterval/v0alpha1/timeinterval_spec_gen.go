package v0alpha1

// Interval defines model for Interval.
// +k8s:openapi-gen=true
type Interval struct {
	// +listType=atomic
	DaysOfMonth []string `json:"daysOfMonth,omitempty"`

	// +listType=atomic
	Location *string `json:"location,omitempty"`

	// +listType=atomic
	Months []string `json:"months,omitempty"`

	// +listType=atomic
	Times []TimeRange `json:"times,omitempty"`

	// +listType=atomic
	Weekdays []string `json:"weekdays,omitempty"`

	// +listType=atomic
	Years []string `json:"years,omitempty"`
}

// Spec defines model for Spec.
// +k8s:openapi-gen=true
type Spec struct {
	// +listType=atomic
	Intervals []Interval `json:"intervals"`
}

// TimeRange defines model for TimeRange.
// +k8s:openapi-gen=true
type TimeRange struct {
	EndMinute   string `json:"endMinute"`
	StartMinute string `json:"startMinute"`
}
