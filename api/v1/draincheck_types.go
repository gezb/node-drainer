package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DrainCheckSpec defines the desired state of DrainCheck.
type DrainCheckSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// PodRegex is a regex that matches pods that should block the draining of nodes
	PodRegex string `json:"podregex,omitempty"`
}

// DrainCheckStatus defines the observed state of DrainCheck.
type DrainCheckStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DrainCheck is the Schema for the drainchecks API.
type DrainCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DrainCheckSpec   `json:"spec,omitempty"`
	Status DrainCheckStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DrainCheckList contains a list of DrainCheck.
type DrainCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DrainCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DrainCheck{}, &DrainCheckList{})
}
