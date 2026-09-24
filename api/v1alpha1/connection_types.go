package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PathlyConnectionSpec holds how the operator authenticates to Pathly.
type PathlyConnectionSpec struct {
	// SecretRef points at a Secret that holds the Pathly API token under key
	// "token" (or "apiToken"). Never put the token in this CR.
	SecretRef corev1.SecretKeySelector `json:"secretRef"`

	// BaseURL overrides the Pathly API base URL. Defaults to https://api.pathlyhq.com.
	// +optional
	BaseURL *string `json:"baseURL,omitempty"`
}

// PathlyConnectionStatus reports readiness of the connection.
type PathlyConnectionStatus struct {
	// Ready is true when the token Secret is readable.
	Ready bool `json:"ready,omitempty"`
	// Message explains the last reconciliation outcome.
	Message string `json:"message,omitempty"`
	// ObservedGeneration is the last generation processed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pcxn
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PathlyConnection references a Secret that holds a Pathly API token.
type PathlyConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PathlyConnectionSpec   `json:"spec"`
	Status PathlyConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PathlyConnectionList contains a list of PathlyConnection.
type PathlyConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PathlyConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PathlyConnection{}, &PathlyConnectionList{})
}
