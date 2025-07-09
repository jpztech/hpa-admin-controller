/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/client/custom_metrics"
	"k8s.io/metrics/pkg/client/external_metrics"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	scalingv1alpha1 "github.com/jpztech/hpa-admin-controller/api/v1alpha1"
)

// SynthenticMetricReconciler reconciles a SynthenticMetric object
type SynthenticMetricReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *rest.Config
}

//+kubebuilder:rbac:groups=scaling.scaling.com,resources=synthenticmetrics,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=scaling.scaling.com,resources=synthenticmetrics/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=scaling.scaling.com,resources=synthenticmetrics/finalizers,verbs=update
//+kubebuilder:rbac:groups=custom.metrics.k8s.io,resources=*,verbs=*
//+kubebuilder:rbac:groups=external.metrics.k8s.io,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SynthenticMetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	syntheticMetric := &scalingv1alpha1.SynthenticMetric{}
	if err := r.Get(ctx, req.NamespacedName, syntheticMetric); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("SyntheticMetric resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get SyntheticMetric")
		return ctrl.Result{}, err
	}

	clientset, err := kubernetes.NewForConfig(r.Config)
	if err != nil {
		logger.Error(err, "Failed to create kubernetes clientset")
		return ctrl.Result{}, err
	}

	customMetricsClient := custom_metrics.NewForConfig(r.Config)
	externalMetricsClient := external_metrics.NewForConfig(r.Config)

	var syntheticUsageRatio float64

	for _, metricSpec := range syntheticMetric.Spec.Metrics {
		var currentValue int64
		var targetValue int64

		switch metricSpec.Type {
		case "Pods":
			// Example: Get CPU usage for pods
			// This is a simplified example. You'll need to adjust based on actual metric names and labels.
			// Ensure the metric name and selectors in metricSpec.Pods.Metric are correctly configured.
			metricName := metricSpec.Pods.Metric.Name
			namespace := syntheticMetric.Namespace // Assuming target is in the same namespace

			// Determine the target resource kind
			targetGR := metav1.GroupResource{Group: "", Resource: syntheticMetric.Spec.ScaleTargetRef.Kind} // Adjust group if needed, e.g., "apps" for Deployments
			if syntheticMetric.Spec.ScaleTargetRef.APIVersion != "v1" { // core v1 types have no group
				targetGR.Group = syntheticMetric.Spec.ScaleTargetRef.APIVersion
			}


			// Fetch metric from custom.metrics.k8s.io
			// Note: This assumes the metric is a pod metric.
			// The GroupResource should refer to "pods".
			// The 'namedMetric' would be the scaleTargetRef's name if metrics are aggregated by the target object.
			// Or, if it's a direct pod metric, you might need to list pods and then get metrics.
			// This part is highly dependent on how your metrics are exposed by the metrics server.

			// Let's assume for "Pods" type, we are looking for a custom metric associated with the target resource itself.
			// This is a common pattern but might need adjustment.
			value, err := customMetricsClient.NamespacedMetrics(namespace).GetForObject(
				targetGR,                        // This might need to be pods if it's a direct pod metric
				syntheticMetric.Spec.ScaleTargetRef.Name, // Name of the object (e.g., Deployment name)
				metricName,
				metav1.LabelSelector{}, // metricSpec.Pods.Metric.Selector - needs conversion to metav1.LabelSelector
			)

			if err != nil {
				// Fallback to external metrics if custom metric not found or error
				logger.Info("Trying to fetch from external.metrics.k8s.io", "metricName", metricName)
				extValue, errExt := externalMetricsClient.NamespacedMetrics(namespace).Get(metricName, metav1.LabelSelector{})
				if errExt != nil {
					logger.Error(errExt, "Failed to get external metric for", "metricName", metricName)
					// Continue to next metric or handle error appropriately
					// We might want to skip this metric in the calculation or return an error for the reconcile
					continue
				}
				if len(extValue.Items) > 0 {
					currentValue = extValue.Items[0].Value.MilliValue() / 1000 // Convert to base units
					logger.Info("Got external metric", "metricName", metricName, "value", currentValue)
				} else {
					logger.Info("No external metric items found for", "metricName", metricName)
					continue
				}
			} else {
				currentValue = value.Value.MilliValue() / 1000 // Convert to base units if it's a milli-value
				logger.Info("Got custom metric", "metricName", metricName, "value", currentValue)
			}


			if metricSpec.Pods.Target.AverageValue != nil {
				targetValue = metricSpec.Pods.Target.AverageValue.Value()
				if metricSpec.Pods.Target.AverageValue.Type == resource.String {
					// Potentially parse string quantities like "100m" if needed, though Value() should handle it for simple numbers
					parsedQty, err := resource.ParseQuantity(metricSpec.Pods.Target.AverageValue.String())
					if err != nil {
						logger.Error(err, "Failed to parse target averageValue", "value", metricSpec.Pods.Target.AverageValue.String())
						continue
					}
					targetValue = parsedQty.Value() // For CPU, this might be milli-cores, ensure units match currentValue
				}
			} else {
				logger.Info("Target AverageValue not set for metric", "metricName", metricName)
				continue // Skip if target is not defined
			}

		default:
			logger.Info("Unsupported metric type", "type", metricSpec.Type)
			continue
		}

		if targetValue == 0 {
			logger.Info("Target value is 0, skipping metric to avoid division by zero", "metricName", metricSpec.Pods.Metric.Name)
			continue
		}

		ratio := float64(currentValue) / float64(targetValue)
		syntheticUsageRatio += (float64(metricSpec.Weight) / 100.0) * ratio // Weight is a percentage
		logger.Info("Calculated partial ratio", "metric", metricSpec.Pods.Metric.Name, "current", currentValue, "target", targetValue, "weight", metricSpec.Weight, "ratio", ratio, "cumulativeSyntheticRatio", syntheticUsageRatio)
	}

	// Update status
	statusSyntheticUsageRatioStr := fmt.Sprintf("%.2f", syntheticUsageRatio)
	if syntheticMetric.Status.SyntheticUsageRatio == nil || *syntheticMetric.Status.SyntheticUsageRatio != statusSyntheticUsageRatioStr {
		syntheticMetric.Status.SyntheticUsageRatio = &statusSyntheticUsageRatioStr
		if err := r.Status().Update(ctx, syntheticMetric); err != nil {
			logger.Error(err, "Failed to update SyntheticMetric status")
			return ctrl.Result{}, err
		}
		logger.Info("Updated SyntheticMetric status", "syntheticUsageRatio", statusSyntheticUsageRatioStr)
	}

	// Expose syntheticUsageRatio as an external metric (conceptual)
	// This typically involves a custom metrics adapter that reads this status.
	// For now, we've updated the status. The HPA would then need to be configured
	// to read an external metric that this controller (or an adapter) exposes.
	// A simple way for an adapter to get this value is by reading the CR status.

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SynthenticMetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Config = mgr.GetConfig() // Get the config for creating clientsets
	return ctrl.NewControllerManagedBy(mgr).
		For(&scalingv1alpha1.SynthenticMetric{}).
		Complete(r)
}
