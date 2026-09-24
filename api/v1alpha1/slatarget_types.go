package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PathlySlaTargetSpec describes a Pathly SLA target.
type PathlySlaTargetSpec struct {
	// ConnectionRef selects the PathlyConnection that supplies the API token.
	ConnectionRef corev1.LocalObjectReference `json:"connectionRef"`

	// MonitorID is the scenario this target measures.
	// +optional
	MonitorID *string `json:"monitorId,omitempty"`

	// Name of the target.
	// +optional
	Name *string `json:"name,omitempty"`

	// ObjectivePct is the uptime objective (e.g. 99.9).
	ObjectivePct float64 `json:"objectivePct"`

	// WindowDays is the rolling measurement window.
	WindowDays int64 `json:"windowDays"`

	// ExcludeMaintenance excludes runs that fall in a maintenance window.
	// +optional
	ExcludeMaintenance *bool `json:"excludeMaintenance,omitempty"`

	// WarnAtBudgetRatio raises a warning when this fraction of the error budget is consumed.
	// +optional
	WarnAtBudgetRatio *float64 `json:"warnAtBudgetRatio,omitempty"`

	// Enabled toggles the target.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// PathlySlaTargetStatus is the observed state.
type PathlySlaTargetStatus struct {
	ID                 string   `json:"id,omitempty"`
	MonitorID          *string  `json:"monitorId,omitempty"`
	Name               *string  `json:"name,omitempty"`
	ObjectivePct       *float64 `json:"objectivePct,omitempty"`
	WindowDays         *int64   `json:"windowDays,omitempty"`
	ExcludeMaintenance *bool    `json:"excludeMaintenance,omitempty"`
	WarnAtBudgetRatio  *float64 `json:"warnAtBudgetRatio,omitempty"`
	Enabled            *bool    `json:"enabled,omitempty"`
	Ready              bool     `json:"ready,omitempty"`
	Message            string   `json:"message,omitempty"`
	ObservedGeneration int64    `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=psla
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PathlySlaTarget is a Pathly SLA target managed from Kubernetes.
type PathlySlaTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PathlySlaTargetSpec   `json:"spec"`
	Status PathlySlaTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PathlySlaTargetList contains a list of PathlySlaTarget.
type PathlySlaTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PathlySlaTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PathlySlaTarget{}, &PathlySlaTargetList{})
}
