package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DrainCheckSpec defines the desired state of DrainCheck.
type DrainCheckSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of DrainCheck. Edit draincheck_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// DrainCheckStatus defines the observed state of DrainCheck.
type DrainCheckStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

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
