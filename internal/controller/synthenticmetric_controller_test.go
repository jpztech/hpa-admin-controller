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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	scalingv1alpha1 "github.com/jpztech/hpa-admin-controller/api/v1alpha1"
)

var _ = Describe("SynthenticMetric Controller", func() {
	Context("When reconciling a SynthenticMetric resource", func() {
		const resourceName = "test-sm-resource"
		const namespace = "default"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}

		AfterEach(func() {
			resource := &scalingv1alpha1.SynthenticMetric{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance SynthenticMetric")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, resource)
					return errors.IsNotFound(err)
				}, time.Second*10, time.Millisecond*250).Should(BeTrue())
			}
		})

		It("should successfully reconcile the resource and update status", func() {
			By("Creating a new SynthenticMetric resource")
			sm := &scalingv1alpha1.SynthenticMetric{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: scalingv1alpha1.SynthenticMetricSpec{
					ScaleTargetRef: scalingv1alpha1.ScaleTargetRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "test-deployment",
					},
					Metrics: []scalingv1alpha1.MetricSpec{
						{
							Weight: 50,
							Type:   autoscalingv2.PodsMetricSourceType,
							Pods: &autoscalingv2.PodsMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: "cpu_usage_rate",
								},
								Target: autoscalingv2.MetricTarget{
									Type:         autoscalingv2.AverageValueMetricType,
									AverageValue: resource.NewQuantity(60, resource.DecimalSI),
								},
							},
						},
						{
							Weight: 50,
							Type:   autoscalingv2.PodsMetricSourceType,
							Pods: &autoscalingv2.PodsMetricSource{
								Metric: autoscalingv2.MetricIdentifier{
									Name: "memory_usage_bytes",
								},
								Target: autoscalingv2.MetricTarget{
									Type:         autoscalingv2.AverageValueMetricType,
									AverageValue: resource.NewQuantity(500*1024*1024, resource.BinarySI), // 500Mi
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sm)).To(Succeed())

			// We need to use the Reconciler from the manager to get the Config
			// This assumes `k8sManager` is set up similarly to how it's done in main.go and suite_test.go
			// For this test, we'll directly initialize as `cfg` is available from test setup.
			controllerReconciler := &SynthenticMetricReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Config: cfg, // cfg should be available from the test environment setup in suite_test.go
			}

			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if the status is updated")
			updatedSm := &scalingv1alpha1.SynthenticMetric{}
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, typeNamespacedName, updatedSm)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updatedSm.Status.SyntheticUsageRatio).NotTo(BeNil())
				// Since metrics clients will fail to fetch actual metrics in envtest,
				// the calculated ratio will be 0.
				g.Expect(*updatedSm.Status.SyntheticUsageRatio).To(Equal("0.00"))
			}, time.Second*10, time.Millisecond*250).Should(Succeed())
		})
	})
})
