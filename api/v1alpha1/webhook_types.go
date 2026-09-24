package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PathlyWebhookSpec describes a desired Pathly outbound webhook.
type PathlyWebhookSpec struct {
	// ConnectionRef selects the PathlyConnection that supplies the API token.
	ConnectionRef corev1.LocalObjectReference `json:"connectionRef"`

	// URL is the destination. It is write-only: the API never returns it on read.
	URL string `json:"url"`

	// Events to subscribe to (e.g. run.failed, incident.opened).
	// +optional
	Events []string `json:"events,omitempty"`
}

// PathlyWebhookStatus is the observed state.
type PathlyWebhookStatus struct {
	ID             string   `json:"id,omitempty"`
	Events         []string `json:"events,omitempty"`
	Enabled        *bool    `json:"enabled,omitempty"`
	HasSecret      *bool    `json:"hasSecret,omitempty"`
	URLFingerprint *string  `json:"urlFingerprint,omitempty"`
	CreatedAt      *string  `json:"createdAt,omitempty"`
	Ready          bool     `json:"ready,omitempty"`
	Message        string   `json:"message,omitempty"`
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pwh
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PathlyWebhook is a Pathly outbound webhook managed from Kubernetes.
type PathlyWebhook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PathlyWebhookSpec   `json:"spec"`
	Status PathlyWebhookStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PathlyWebhookList contains a list of PathlyWebhook.
type PathlyWebhookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PathlyWebhook `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PathlyWebhook{}, &PathlyWebhookList{})
}
