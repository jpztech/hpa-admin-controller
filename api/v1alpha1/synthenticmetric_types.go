/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScaleTargetRef points to the target resource to scale.
type ScaleTargetRef struct {
	// APIVersion is the API version of the target resource.
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`
	// Kind is the kind of the target resource.
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`
	// Name is the name of the target resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// MetricSpec defines the desired behavior of a metric.
type MetricSpec struct {
	// Weight is the weight of this metric.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Weight int32 `json:"weight"`
	// Type is the type of metric. Only "Pods" is supported currently.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Pods
	Type autoscalingv2.MetricSourceType `json:"type"`
	// Pods specifies the pods metric source. Required if Type is "Pods".
	// +kubebuilder:validation:Required
	Pods *autoscalingv2.PodsMetricSource `json:"pods"`
}

// SynthenticMetricSpec defines the desired state of SynthenticMetric
type SynthenticMetricSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ScaleTargetRef points to the target resource to scale.
	// +kubebuilder:validation:Required
	ScaleTargetRef ScaleTargetRef `json:"scaleTargetRef"`
	// Metrics is the list of metrics used to calculate the desired replicas.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Metrics []MetricSpec `json:"metrics"`
}

// SynthenticMetricStatus defines the observed state of SynthenticMetric
type SynthenticMetricStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=synthenticmetrics,singular=synthenticmetric,scope=Namespaced,shortName=sm
//+kubebuilder:printcolumn:name="Target Kind",type="string",JSONPath=".spec.scaleTargetRef.kind"
//+kubebuilder:printcolumn:name="Target Name",type="string",JSONPath=".spec.scaleTargetRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SynthenticMetric is the Schema for the synthenticmetrics API
type SynthenticMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SynthenticMetricSpec   `json:"spec,omitempty"`
	Status SynthenticMetricStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SynthenticMetricList contains a list of SynthenticMetric
type SynthenticMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SynthenticMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SynthenticMetric{}, &SynthenticMetricList{})
}
