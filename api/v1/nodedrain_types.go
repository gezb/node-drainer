package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeDrainPhase string

const (
	// NodeDrainFinalizer is a finalizer for a NodeMaintenance CR deletion
	NodeDrainFinalizer string = "co.uk.gezb,NodeDrain"

	NodeDrainPhasePending               NodeDrainPhase = "Pending"
	NodeDrainPhaseCordoned              NodeDrainPhase = "Cordoned"
	NodeDrainPhaseDraining              NodeDrainPhase = "Draining"
	NodeDrainPhaseCompleted             NodeDrainPhase = "Completed"
	NodeDrainPhasePodsBlocking          NodeDrainPhase = "PodsBlockingDrain"
	NodeDrainPhaseOtherNodesNotCordoned NodeDrainPhase = "OtherNodesNotCordoned"
	NodeDrainPhaseWaitForPodsToRestart  NodeDrainPhase = "WaitForPodsToRestart"
	NodeDrainPhaseFailed                NodeDrainPhase = "Failed"
)

type NamespaceAndName struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// NodeDrainSpec defines the desired state of NodeDrain.
type NodeDrainSpec struct {
	// NodeName is the name of the node to drain
	NodeName string `json:"nodeName"`
	// VersionToDrainRegex is a regex to match the expected kubernetes version that we want to Drain
	VersionToDrainRegex string `json:"versionToDrainRegex"`
	// NodeRole is the nodes expected "role" label
	NodeRole string `json:"nodeRole"`
	// DisableCordon stop the controller cordoning the node
	// +kubebuilder:validation:Optional
	DisableCordon bool `json:"disableCordon"`
	// WaitForPods waits for the evicted pods to be running again before completing
	// +kubebuilder:validation:Optional
	WaitForPodsToRestart bool `json:"waitForPodsToRestart"`
}

// NodeDrainStatus defines the observed state of NodeDrain.
type NodeDrainStatus struct {
	// Phase represents the progress of this nodeDrain
	Phase NodeDrainPhase `json:"phase"`
	// The last time the status has been updated
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`
	// LastError represents the latest error if any in the latest reconciliation
	LastError string `json:"lastError,omitempty"`
	// PodsToBeEvicted is the list of pods for the controller needs to evict
	PodsToBeEvicted []NamespaceAndName `json:"podsToBeEvicted,omitempty"`
	// PendingPods is a list of pending pods for eviction
	PendingPods []string `json:"pendingPods,omitempty"`
	// TotalPods is the total number of all pods on the node from the start
	// +operator-sdk:csv:customresourcedefinitions:type=status
	TotalPods int `json:"totalPods,omitempty"`
	// EvictionPods is the total number of pods up for eviction from the start
	EvictionPodCount int `json:"evictionPods,omitempty"`
	// Percentage completion of draining the node
	DrainProgress int `json:"drainProgress,omitempty"`
	// PodsBlockingDrain is a list of pods that are blocking the draining of this node
	PodsBlockingDrain string `json:"podsBlockingDrain,omitempty"`
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
