package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeDrainPhase string

const (
	// NodeDrainFinalizer is a finalizer for a NodeMaintenance CR deletion
	NodeDrainFinalizer string = "co.uk.gezb,NodeDrain"

	NodeDrainPhasePending   NodeDrainPhase = "Pending"
	NodeDrainPhaseCordoned  NodeDrainPhase = "Cordoned"
	NodeDrainPhaseDraining  NodeDrainPhase = "Draining"
	NodeDrainPhaseCompleted NodeDrainPhase = "Completed"
	NodeDrainPhaseFailed    NodeDrainPhase = "Failed"
)

// NodeDrainSpec defines the desired state of NodeDrain.
type NodeDrainSpec struct {
	NodeName string `json:"nodeName"`
}

// NodeDrainStatus defines the observed state of NodeDrain.
type NodeDrainStatus struct {
	// Phase represents the progress of this nodeDrain
	Phase NodeDrainPhase `json:"phase"`
	// The last time the status has been updated
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`
	// LastError represents the latest error if any in the latest reconciliation
	LastError string `json:"lastError,omitempty"`
	// PendingPods is a list of pending pods for eviction
	PendingPods []string `json:"pendingpods,omitempty"`
	// TotalPods is the total number of all pods on the node from the start
	// +operator-sdk:csv:customresourcedefinitions:type=status
	TotalPods int `json:"totalpods,omitempty"`
	// EvictionPods is the total number of pods up for eviction from the start
	EvictionPodCount int `json:"evictionPods,omitempty"`
	// Percentage completion of draining the node
	DrainProgress int `json:"drainProgress,omitempty"`
	// PodsBlockingDrain is a list of pods that are blocking the draining of this node
	PodsBlockingDrain string `json:"podsblockingdrain,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Phase of the NodeDrain"
// +kubebuilder:printcolumn:name="Pods BlockingDrain",type="string",JSONPath=".status.podsblockingdrain",description="Pods that are blocking drain"

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
