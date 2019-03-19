/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package collector

import (
	"k8s.io/kube-state-metrics/pkg/metric"

	autoscaling "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var (
	descHorizontalPodAutoscalerLabelsName          = "kube_hpa_labels"
	descHorizontalPodAutoscalerLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
	descHorizontalPodAutoscalerLabelsDefaultLabels = []string{"namespace", "hpa"}

	hpaMetricFamilies = []metric.FamilyGenerator{
		{
			Name: "kube_hpa_metadata_generation",
			Type: metric.Gauge,
			Help: "The generation observed by the HorizontalPodAutoscaler controller.",
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(a.ObjectMeta.Generation),
						},
					},
				}
			}),
		},
		{
			Name: "kube_hpa_spec_max_replicas",
			Type: metric.Gauge,
			Help: "Upper limit for the number of pods that can be set by the autoscaler; cannot be smaller than MinReplicas.",
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(a.Spec.MaxReplicas),
						},
					},
				}
			}),
		},
		{
			Name: "kube_hpa_spec_min_replicas",
			Type: metric.Gauge,
			Help: "Lower limit for the number of pods that can be set by the autoscaler, default 1.",
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(*a.Spec.MinReplicas),
						},
					},
				}
			}),
		},
		{
			Name: "kube_hpa_spec_metrics",
			Type: metric.Gauge,
			Help: "Metrics used to calculate the desired replica count",
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: generateMetricsFromMetricSpec(a.Spec.Metrics),
				}
			}),
		},
		{
			Name: "kube_hpa_status_current_replicas",
			Type: metric.Gauge,
			Help: "Current number of replicas of pods managed by this autoscaler.",
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(a.Status.CurrentReplicas),
						},
					},
				}
			}),
		},
		{
			Name: "kube_hpa_status_desired_replicas",
			Type: metric.Gauge,
			Help: "Desired number of replicas of pods managed by this autoscaler.",
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(a.Status.DesiredReplicas),
						},
					},
				}
			}),
		},
		{
			Name: descHorizontalPodAutoscalerLabelsName,
			Type: metric.Gauge,
			Help: descHorizontalPodAutoscalerLabelsHelp,
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				labelKeys, labelValues := kubeLabelsToPrometheusLabels(a.Labels)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
							Value:       1,
						},
					},
				}
			}),
		},
		{
			Name: "kube_hpa_status_condition",
			Type: metric.Gauge,
			Help: "The condition of this autoscaler.",
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}

				for _, c := range a.Status.Conditions {
					metrics := addConditionMetrics(c.Status)

					for _, m := range metrics {
						metric := m
						metric.LabelKeys = []string{"condition", "status"}
						metric.LabelValues = append(metric.LabelValues, string(c.Type))
						ms = append(ms, metric)
					}
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_hpa_status_currentmetrics",
			Type: metric.Gauge,
			Help: "Current metrics is the last read state of the metrics used by this autoscaler",
			GenerateFunc: wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: generateMetricsFromMetricStatus(a.Status.CurrentMetrics),
				}
			}),
		},
	}
)

func wrapHPAFunc(f func(*autoscaling.HorizontalPodAutoscaler) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		hpa := obj.(*autoscaling.HorizontalPodAutoscaler)

		metricFamily := f(hpa)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(descHorizontalPodAutoscalerLabelsDefaultLabels, m.LabelKeys...)
			m.LabelValues = append([]string{hpa.Namespace, hpa.Name}, m.LabelValues...)
		}

		return metricFamily
	}
}

func createHPAListWatch(kubeClient clientset.Interface, ns string) cache.ListWatch {
	return cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(ns).List(opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(ns).Watch(opts)
		},
	}
}

func generateMetricsFromMetricSpec(mss []autoscaling.MetricSpec) []*metric.Metric {
	out := make([]*metric.Metric, 0)

	for _, ms := range mss {
		m := &metric.Metric{
			LabelKeys:   []string{"type"},
			LabelValues: []string{string(ms.Type)},
		}

		if ms.Type == autoscaling.ResourceMetricSourceType {
			m.LabelKeys = append(m.LabelKeys, "name")
			m.LabelValues = append(m.LabelValues, string(ms.Resource.Name))

			if v := ms.Resource.TargetAverageUtilization; v != nil {
				m.LabelKeys = append(m.LabelKeys, "target_type")
				m.LabelValues = append(m.LabelValues, "AverageUtilization")
				m.Value = float64(*v)
				out = append(out, m)
			} else if v := ms.Resource.TargetAverageValue; v != nil {
				m.LabelKeys = append(m.LabelKeys, "target_type")
				m.LabelValues = append(m.LabelValues, "AverageValue")
				m.Value = float64(v.MilliValue())
				out = append(out, m)
			}
		}
	}

	return out
}

func generateMetricsFromMetricStatus(mss []autoscaling.MetricStatus) []*metric.Metric {
	out := make([]*metric.Metric, 0)

	for _, ms := range mss {
		m := &metric.Metric{
			LabelKeys:   []string{"type"},
			LabelValues: []string{string(ms.Type)},
		}

		if ms.Type == autoscaling.ResourceMetricSourceType {
			m.LabelKeys = append(m.LabelKeys, "name")
			m.LabelValues = append(m.LabelValues, string(ms.Resource.Name))

			if v := ms.Resource.CurrentAverageUtilization; v != nil {
				mcopy := &metric.Metric{}
				copy(mcopy.LabelKeys, m.LabelKeys)
				copy(mcopy.LabelValues, m.LabelValues)

				mcopy.LabelKeys = append(m.LabelKeys, "target_type")
				mcopy.LabelValues = append(m.LabelValues, "AverageUtilization")
				mcopy.Value = float64(*v)
				out = append(out, mcopy)
			}

			v := ms.Resource.CurrentAverageValue;
			m.LabelKeys = append(m.LabelKeys, "target_type")
			m.LabelValues = append(m.LabelValues, "AverageValue")
			m.Value = float64(v.MilliValue())
			out = append(out, m)
		}
	}

	return out
}
