package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeDrainSpec defines the desired state of NodeDrain.
type NodeDrainSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of NodeDrain. Edit nodedrain_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// NodeDrainStatus defines the observed state of NodeDrain.
type NodeDrainStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// NodeDrain is the Schema for the nodedrains API.
type NodeDrain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeDrainSpec   `json:"spec,omitempty"`
	Status NodeDrainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeDrainList contains a list of NodeDrain.
type NodeDrainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeDrain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeDrain{}, &NodeDrainList{})
}
