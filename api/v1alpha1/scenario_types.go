package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PathlyScenarioSpec describes a desired Pathly HTTP scenario.
//
// HTTP-only policy: only type "http" is accepted. Browser journeys carry
// credentials that must not land in etcd; manage those at https://pathlyhq.com
// (console or API).
type PathlyScenarioSpec struct {
	// ConnectionRef selects the PathlyConnection (same namespace) that supplies
	// the API token (Secret holding PATHLY_API_TOKEN from https://pathlyhq.com).
	ConnectionRef corev1.LocalObjectReference `json:"connectionRef"`

	// Name is the human-readable scenario name.
	Name string `json:"name"`

	// Type of the scenario. Only "http" is accepted (HTTP-only policy —
	// browser step ASTs are rejected so credentials never land in etcd).
	// See https://pathlyhq.com/en/developers
	// +kubebuilder:validation:Enum=http
	// +kubebuilder:default=http
	// +optional
	Type string `json:"type,omitempty"`

	// URL to probe.
	// +optional
	URL *string `json:"url,omitempty"`

	// IntervalSec between runs.
	// +optional
	IntervalSec *int64 `json:"intervalSec,omitempty"`

	// Method is the HTTP method (GET, POST, …).
	// +optional
	Method *string `json:"method,omitempty"`

	// ExpectedStatus is the HTTP status code that means success.
	// +optional
	ExpectedStatus *int64 `json:"expectedStatus,omitempty"`

	// MaxLatencyMs fails the run when the response is slower.
	// +optional
	MaxLatencyMs *int64 `json:"maxLatencyMs,omitempty"`

	// ExpectText fails the run when the body does not contain this string.
	// +optional
	ExpectText *string `json:"expectText,omitempty"`

	// Regions where the probe runs.
	// +optional
	Regions []string `json:"regions,omitempty"`

	// Tags for organization.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Folder groups scenarios in the console.
	// +optional
	Folder *string `json:"folder,omitempty"`

	// Severity of alerts raised by this scenario.
	// +optional
	Severity *string `json:"severity,omitempty"`

	// Runbook URL shown in alerts.
	// +optional
	Runbook *string `json:"runbook,omitempty"`

	// Cron expression for schedule-based runs.
	// +optional
	Cron *string `json:"cron,omitempty"`

	// Enabled toggles the scenario.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// PathlyScenarioStatus is the observed state.
type PathlyScenarioStatus struct {
	// ID is the Pathly scenario identifier.
	ID string `json:"id,omitempty"`
	// LastStatus is the outcome of the latest run.
	LastStatus *string `json:"lastStatus,omitempty"`
	// ScenarioFingerprint is a hash of the scenario definition.
	ScenarioFingerprint *string `json:"scenarioFingerprint,omitempty"`
	// CreatedAt is the creation timestamp from Pathly.
	CreatedAt *string `json:"createdAt,omitempty"`
	// MutedUntil is set when the scenario is muted.
	MutedUntil *string `json:"mutedUntil,omitempty"`
	// Ready is true when the remote resource matches the desired spec.
	Ready bool `json:"ready,omitempty"`
	// Message explains the last reconciliation outcome.
	Message string `json:"message,omitempty"`
	// ObservedGeneration is the last generation processed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=psc
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PathlyScenario is a Pathly HTTP monitoring scenario managed from Kubernetes
// (HTTP-only; no browser journeys). Create tokens at https://pathlyhq.com —
// store PATHLY_API_TOKEN in a Secret referenced by PathlyConnection.
type PathlyScenario struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PathlyScenarioSpec   `json:"spec"`
	Status PathlyScenarioStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PathlyScenarioList contains a list of PathlyScenario.
type PathlyScenarioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PathlyScenario `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PathlyScenario{}, &PathlyScenarioList{})
}
