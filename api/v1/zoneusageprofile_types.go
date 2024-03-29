package v1

import (
	controlv1 "github.com/appuio/control-api/apis/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZoneUsageProfileSpec defines the desired state of ZoneUsageProfile
type ZoneUsageProfileSpec struct {
	// UpstreamSpec is the spec of the upstream UsageProfile
	UpstreamSpec controlv1.UsageProfileSpec `json:"upstreamSpec"`
}

// ZoneUsageProfileStatus defines the observed state of ZoneUsageProfile
type ZoneUsageProfileStatus struct {
	// TODO(bastjan): track applied resources to be able to clean up.
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ZoneUsageProfile is the Schema for the ZoneUsageProfiles API
type ZoneUsageProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZoneUsageProfileSpec   `json:"spec,omitempty"`
	Status ZoneUsageProfileStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ZoneUsageProfileList contains a list of ZoneUsageProfile
type ZoneUsageProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZoneUsageProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZoneUsageProfile{}, &ZoneUsageProfileList{})
}
