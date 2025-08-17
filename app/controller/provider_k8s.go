package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sProvider represents a Kubernetes provider responsible for deploying and managing resources in a Kubernetes cluster.
type K8sProvider struct {
	client    kubernetes.Interface
	ns        string
	image     string
	tag       string
	replicas  int32
	env       []corev1.EnvVar
	resReq    *corev1.ResourceRequirements
	enableHPA bool
	hpaMin    int32
	hpaMax    int32
	hpaCPU    int32 // target CPU utilization %
	tqPrefix  string
}

var _ Provider = (*K8sProvider)(nil)

// NewK8sProviderFromEnv creates a new K8sProvider instance using the current Kubernetes context.
func NewK8sProviderFromEnv() (*K8sProvider, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = clientcmd.RecommendedHomeFile
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build kube config: %w", err)
		}
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}

	ns := mustEnv("K8S_NAMESPACE")
	image := mustEnv("INDEXER_IMAGE")
	tag := mustEnv("INDEXER_TAG")

	replicas := int32FromEnv("INDEXER_REPLICAS", 1)
	enableHPA := boolFromEnv("INDEXER_ENABLE_HPA", false)
	hpaMin := int32FromEnv("INDEXER_HPA_MIN", 1)
	hpaMax := int32FromEnv("INDEXER_HPA_MAX", 5)
	hpaCPU := int32FromEnv("INDEXER_HPA_CPU_TARGET", 80)
	tqPrefix := getEnv("TEMPORAL_TASK_QUEUE_PREFIX", "index:")

	// Container env (common + per-chain overrides later).
	env := []corev1.EnvVar{
		{Name: "CHAIN_ID", Value: ""}, // set per chain
		{Name: "TASK_QUEUE", Value: ""},

		{Name: "TEMPORAL_ADDRESS", Value: mustEnv("TEMPORAL_ADDRESS")},
		{Name: "TEMPORAL_NAMESPACE", Value: mustEnv("TEMPORAL_NAMESPACE")},
		{Name: "TEMPORAL_TLS", Value: getEnv("TEMPORAL_TLS", "false")},

		{Name: "CLICKHOUSE_ADDR", Value: mustEnv("CLICKHOUSE_ADDR")},
	}
	if v := os.Getenv("CLICKHOUSE_USER"); v != "" {
		env = append(env, corev1.EnvVar{Name: "CLICKHOUSE_USER", Value: v})
	}
	if v := os.Getenv("CLICKHOUSE_PASSWORD"); v != "" {
		env = append(env, corev1.EnvVar{Name: "CLICKHOUSE_PASSWORD", Value: v})
	}
	if v := os.Getenv("CLICKHOUSE_INDEX_DB"); v != "" {
		env = append(env, corev1.EnvVar{Name: "CLICKHOUSE_INDEX_DB", Value: v})
	}
	if v := os.Getenv("CLICKHOUSE_REPORTS_DB"); v != "" {
		env = append(env, corev1.EnvVar{Name: "CLICKHOUSE_REPORTS_DB", Value: v})
	}

	// Optional CPU/Memory
	var res *corev1.ResourceRequirements
	if cpu := os.Getenv("INDEXER_CPU"); cpu != "" || os.Getenv("INDEXER_MEM") != "" {
		req := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
			Limits:   corev1.ResourceList{},
		}
		if cpu := os.Getenv("INDEXER_CPU"); cpu != "" {
			q := resource.MustParse(cpu)
			req.Requests[corev1.ResourceCPU] = q
			req.Limits[corev1.ResourceCPU] = q
		}
		if mem := os.Getenv("INDEXER_MEM"); mem != "" {
			q := resource.MustParse(mem)
			req.Requests[corev1.ResourceMemory] = q
			req.Limits[corev1.ResourceMemory] = q
		}
		res = &req
	}

	return &K8sProvider{
		client:    cs,
		ns:        ns,
		image:     image,
		tag:       tag,
		replicas:  replicas,
		env:       env,
		resReq:    res,
		enableHPA: enableHPA,
		hpaMin:    hpaMin,
		hpaMax:    hpaMax,
		hpaCPU:    hpaCPU,
		tqPrefix:  tqPrefix,
	}, nil
}

// EnsureChain ensures a Kubernetes Deployment exists for the specified chain, updating or creating it as necessary.
func (p *K8sProvider) EnsureChain(ctx context.Context, c *Chain) error {
	name := deploymentName(c.ID)
	labels := map[string]string{
		"app":        "indexer",
		"managed-by": "canopyx-controller",
		"chain":      c.ID,
	}

	// Per-chain env
	env := make([]corev1.EnvVar, 0, len(p.env))
	for _, e := range p.env {
		switch e.Name {
		case "CHAIN_ID":
			env = append(env, corev1.EnvVar{Name: "CHAIN_ID", Value: c.ID})
		case "TASK_QUEUE":
			env = append(env, corev1.EnvVar{Name: "TASK_QUEUE", Value: p.tqPrefix + c.ID})
		default:
			env = append(env, e)
		}
	}
	ck := checksumEnv(env)

	replicas := int32(0)
	if !c.Paused && !c.Deleted {
		replicas = p.replicas
	}

	desired := &appsv1.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: p.ns,
			Labels:    labels,
			Annotations: map[string]string{
				"canopyx/env-checksum": ck,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(replicas),
			Selector: &meta.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: meta.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"canopyx/env-checksum": ck,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "indexer",
						Image: fmt.Sprintf("%s:%s", p.image, p.tag),
						Env:   env,
						Resources: func() corev1.ResourceRequirements {
							if p.resReq != nil {
								return *p.resReq
							}
							return corev1.ResourceRequirements{}
						}(),
					}},
				},
			},
		},
	}

	curr, err := p.client.AppsV1().Deployments(p.ns).Get(ctx, name, meta.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := p.client.AppsV1().Deployments(p.ns).Create(ctx, desired, meta.CreateOptions{}); err != nil {
			return fmt.Errorf("create deployment: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	} else {
		if needsUpdate(curr, desired) {
			curr.Spec.Replicas = desired.Spec.Replicas
			curr.Spec.Template = desired.Spec.Template
			if curr.Annotations == nil {
				curr.Annotations = map[string]string{}
			}
			curr.Annotations["canopyx/env-checksum"] = ck
			if _, err := p.client.AppsV1().Deployments(p.ns).Update(ctx, curr, meta.UpdateOptions{}); err != nil {
				return fmt.Errorf("update deployment: %w", err)
			}
		}
	}

	// HPA
	if p.enableHPA {
		if err := p.ensureHPA(ctx, name, replicas, labels); err != nil {
			return err
		}
	} else {
		_ = p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Delete(ctx, name, meta.DeleteOptions{})
	}

	return nil
}

// PauseChain scales down the Kubernetes deployment associated with the given chainID to zero replicas, effectively pausing it.
func (p *K8sProvider) PauseChain(ctx context.Context, chainID string) error {
	name := deploymentName(chainID)
	deploy, err := p.client.AppsV1().Deployments(p.ns).Get(ctx, name, meta.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 0 {
		deploy.Spec.Replicas = int32Ptr(0)
		if _, err := p.client.AppsV1().Deployments(p.ns).Update(ctx, deploy, meta.UpdateOptions{}); err != nil {
			return fmt.Errorf("scale to zero: %w", err)
		}
	}
	return nil
}

// DeleteChain removes the Kubernetes resources associated with a specific chain by its chainID.
// It deletes the deployment and its corresponding horizontal pod autoscaler, if present.
func (p *K8sProvider) DeleteChain(ctx context.Context, chainID string) error {
	name := deploymentName(chainID)
	propagation := meta.DeletePropagationForeground
	_ = p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Delete(ctx, name, meta.DeleteOptions{})
	if err := p.client.AppsV1().Deployments(p.ns).Delete(ctx, name, meta.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete deployment: %w", err)
	}
	return nil
}

// Close releases resources or performs cleanup tasks associated with the K8sProvider instance.
func (p *K8sProvider) Close() error { return nil }

// ensureHPA ensures that the given HPA exists and is configured as expected.
func (p *K8sProvider) ensureHPA(ctx context.Context, name string, replicas int32, labels map[string]string) error {
	hpaMin := p.hpaMin
	hpaMax := p.hpaMax
	if replicas == 0 {
		hpaMin = 0 // allow paused Deployments without HPA fighting scale-to-zero
	}
	targetCPU := p.hpaCPU

	hpaDesired := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: p.ns,
			Labels:    labels,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       name,
			},
			MinReplicas: int32Ptr(hpaMin),
			MaxReplicas: hpaMax,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: int32Ptr(targetCPU),
					},
				},
			}},
		},
	}

	curr, err := p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Get(ctx, name, meta.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Create(ctx, hpaDesired, meta.CreateOptions{})
		return err
	}
	if err != nil {
		return fmt.Errorf("get hpa: %w", err)
	}

	changed := false
	if curr.Spec.MaxReplicas != hpaDesired.Spec.MaxReplicas {
		curr.Spec.MaxReplicas = hpaDesired.Spec.MaxReplicas
		changed = true
	}
	if (curr.Spec.MinReplicas == nil && hpaDesired.Spec.MinReplicas != nil) ||
		(curr.Spec.MinReplicas != nil && hpaDesired.Spec.MinReplicas != nil && *curr.Spec.MinReplicas != *hpaDesired.Spec.MinReplicas) {
		curr.Spec.MinReplicas = hpaDesired.Spec.MinReplicas
		changed = true
	}
	// Replace metrics if differ
	if len(curr.Spec.Metrics) != 1 ||
		curr.Spec.Metrics[0].Type != autoscalingv2.ResourceMetricSourceType ||
		curr.Spec.Metrics[0].Resource == nil ||
		curr.Spec.Metrics[0].Resource.Name != corev1.ResourceCPU ||
		curr.Spec.Metrics[0].Resource.Target.Type != autoscalingv2.UtilizationMetricType ||
		curr.Spec.Metrics[0].Resource.Target.AverageUtilization == nil ||
		*curr.Spec.Metrics[0].Resource.Target.AverageUtilization != targetCPU {
		curr.Spec.Metrics = hpaDesired.Spec.Metrics
		changed = true
	}

	if changed {
		_, err := p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Update(ctx, curr, meta.UpdateOptions{})
		return err
	}
	return nil
}

// ---- utils ------------------------------------------------------------------

var dns1123 = regexp.MustCompile(`[^a-z0-9\-]+`)

// mustEnv panics if the given env var is not set.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("missing required env %s", key))
	}
	return v
}

// getEnv returns the given env var, or the given default if not set.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// boolFromEnv returns the given env var as a bool, or the given default if not set.
func boolFromEnv(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

// int32FromEnv returns the given env var as an int, or the given default if not set.
func int32FromEnv(key string, def int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return int32(n)
		}
	}
	return def
}

// int32Ptr returns a pointer to the given int32.
func int32Ptr(i int32) *int32 { return &i }

// deploymentName generates a DNS-compliant deployment name based on the given chainID, ensuring it meets K8s constraints.
func deploymentName(chainID string) string {
	s := strings.ToLower(chainID)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = dns1123.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) == 0 {
		s = "chain"
	}
	if !strings.HasPrefix(s, "indexer-") {
		s = "indexer-" + s
	}
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// needsUpdate determines whether an update is needed between the current and desired Deployment specifications.
func needsUpdate(curr, desired *appsv1.Deployment) bool {
	// replicas
	if (curr.Spec.Replicas == nil) != (desired.Spec.Replicas == nil) {
		return true
	}
	if curr.Spec.Replicas != nil && desired.Spec.Replicas != nil && *curr.Spec.Replicas != *desired.Spec.Replicas {
		return true
	}
	// image
	if len(curr.Spec.Template.Spec.Containers) != 1 || len(desired.Spec.Template.Spec.Containers) != 1 {
		return true
	}
	if curr.Spec.Template.Spec.Containers[0].Image != desired.Spec.Template.Spec.Containers[0].Image {
		return true
	}
	// env checksum
	ca := curr.Spec.Template.Annotations["canopyx/env-checksum"]
	da := desired.Spec.Template.Annotations["canopyx/env-checksum"]
	return ca != da
}

// checksumEnv returns a checksum of the given env vars, sorted by name.
func checksumEnv(env []corev1.EnvVar) string {
	cp := make([]corev1.EnvVar, len(env))
	copy(cp, env)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Name < cp[j].Name })
	var b strings.Builder
	for _, e := range cp {
		b.WriteString(e.Name)
		b.WriteString("=")
		b.WriteString(e.Value)
		b.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
