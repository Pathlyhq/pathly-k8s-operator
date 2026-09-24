package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PathlyMaintenanceWindowSpec describes a Pathly maintenance window.
type PathlyMaintenanceWindowSpec struct {
	// ConnectionRef selects the PathlyConnection that supplies the API token.
	ConnectionRef corev1.LocalObjectReference `json:"connectionRef"`

	// MonitorID is the scenario this window applies to. Omit for org-wide.
	// +optional
	MonitorID *string `json:"monitorId,omitempty"`

	// StartsAt is an absolute start (RFC3339).
	// +optional
	StartsAt *string `json:"startsAt,omitempty"`

	// EndsAt is an absolute end (RFC3339).
	// +optional
	EndsAt *string `json:"endsAt,omitempty"`

	// Reason is shown in the console and alerts.
	// +optional
	Reason *string `json:"reason,omitempty"`

	// Weekday is 0=Sunday … 6=Saturday for weekly recurrence.
	// +optional
	Weekday *int64 `json:"weekday,omitempty"`

	// StartMinute is minutes from midnight for weekly recurrence.
	// +optional
	StartMinute *int64 `json:"startMinute,omitempty"`

	// DurationMin is the length of a weekly window in minutes.
	// +optional
	DurationMin *int64 `json:"durationMin,omitempty"`
}

// PathlyMaintenanceWindowStatus is the observed state.
type PathlyMaintenanceWindowStatus struct {
	ID                 string  `json:"id,omitempty"`
	MonitorID          *string `json:"monitorId,omitempty"`
	StartsAt           *string `json:"startsAt,omitempty"`
	EndsAt             *string `json:"endsAt,omitempty"`
	Reason             *string `json:"reason,omitempty"`
	Weekday            *int64  `json:"weekday,omitempty"`
	StartMinute        *int64  `json:"startMinute,omitempty"`
	DurationMin        *int64  `json:"durationMin,omitempty"`
	Ready              bool    `json:"ready,omitempty"`
	Message            string  `json:"message,omitempty"`
	ObservedGeneration int64   `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pmw
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PathlyMaintenanceWindow is a Pathly maintenance window managed from Kubernetes.
type PathlyMaintenanceWindow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PathlyMaintenanceWindowSpec   `json:"spec"`
	Status PathlyMaintenanceWindowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PathlyMaintenanceWindowList contains a list of PathlyMaintenanceWindow.
type PathlyMaintenanceWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PathlyMaintenanceWindow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PathlyMaintenanceWindow{}, &PathlyMaintenanceWindowList{})
}
