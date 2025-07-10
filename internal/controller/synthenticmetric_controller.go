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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/metrics/pkg/client/custom_metrics"
	// "k8s.io/metrics/pkg/client/external_metrics" // No longer needed

	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	scalingv1alpha1 "github.com/jpztech/hpa-admin-controller/api/v1alpha1"
)

var (
	SyntheticUsageRatioMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "syntheticUsageRatio",
			Help: "Synthetic usage ratio per ScaleTargetRef",
		},
		[]string{"apiVersion", "kind", "name"},
	)
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

	// Create a discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(r.Config)
	if err != nil {
		logger.Error(err, "Failed to create discovery client")
		return ctrl.Result{}, err
	}

	// Create a REST mapper
	cachedDiscoveryClient := memory.NewMemCacheClient(discoveryClient)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscoveryClient)

	customMetricsClient := custom_metrics.NewForConfig(r.Config, mapper, custom_metrics.NewAvailableAPIsGetter(cachedDiscoveryClient))
	// externalMetricsClient was removed as it's no longer used after focusing on custom pod metrics.
	// externalMetricsClient, err := external_metrics.NewForConfig(r.Config)
	// if err != nil {
	// 	logger.Error(err, "Failed to create external metrics client")
	// 	return ctrl.Result{}, err
	// }

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
			// Convert metav1.GroupResource to schema.GroupKind
			gk := schema.GroupKind{Group: syntheticMetric.Spec.ScaleTargetRef.APIVersion, Kind: syntheticMetric.Spec.ScaleTargetRef.Kind}
			if gk.Group == "v1" { // Core v1 types have no group in terms of API path, but GroupKind should reflect it if specified
				gk.Group = "" // Normalize for core types if APIVersion is "v1"
			}

			// Convert metav1.LabelSelector to labels.Selector
			selector, err := metav1.LabelSelectorAsSelector(metricSpec.Pods.Metric.Selector)
			if err != nil {
				logger.Error(err, "Failed to convert label selector", "selector", metricSpec.Pods.Metric.Selector)
				continue
			}

			value, err := customMetricsClient.NamespacedMetrics(namespace).GetForObject(
				gk,
				syntheticMetric.Spec.ScaleTargetRef.Name,
				metricName,
				selector,
			)

			if err != nil {
				logger.Error(err, "Failed to get custom metric for object", "metricName", metricName, "groupKind", gk, "name", syntheticMetric.Spec.ScaleTargetRef.Name)
				continue
			}

			currentValue = value.Value.MilliValue() / 1000
			logger.Info("Got custom metric", "metricName", metricName, "value", currentValue)

			if metricSpec.Pods.Target.AverageValue != nil {
				targetValue = metricSpec.Pods.Target.AverageValue.Value()
				// The .Type field on resource.Quantity was removed.
				// Value() should return the int64 representation.
				// For string parsing, resource.ParseQuantity is still valid if you have a string.
			} else {
				logger.Info("Target AverageValue not set for metric", "metricName", metricName)
				continue
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

	// Update Prometheus metric
	SyntheticUsageRatioMetric.With(prometheus.Labels{
		"apiVersion": syntheticMetric.Spec.ScaleTargetRef.APIVersion,
		"kind":       syntheticMetric.Spec.ScaleTargetRef.Kind,
		"name":       syntheticMetric.Spec.ScaleTargetRef.Name,
	}).Set(syntheticUsageRatio)
	logger.Info("Updated Prometheus metric", "syntheticUsageRatio", syntheticUsageRatio)

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SynthenticMetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Config = mgr.GetConfig() // Get the config for creating clientsets
	return ctrl.NewControllerManagedBy(mgr).
		For(&scalingv1alpha1.SynthenticMetric{}).
		Complete(r)
}
