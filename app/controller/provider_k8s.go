package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
	"go.uber.org/zap"

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
	Logger         *zap.Logger
	client         kubernetes.Interface
	ns             string
	image          string
	tag            string
	replicas       int32
	env            []corev1.EnvVar
	resReq         *corev1.ResourceRequirements
	enableHPA      bool
	hpaMin         int32
	hpaMax         int32
	hpaCPU         int32 // target CPU utilization %
	tqPrefix       string
	pullPolicy     corev1.PullPolicy
	pullSecretName string
}

var _ Provider = (*K8sProvider)(nil)

// NewK8sProviderFromEnv creates a new K8sProvider instance using the current Kubernetes context.
func NewK8sProviderFromEnv(logger *zap.Logger) (*K8sProvider, error) {
	log := logger.With(zap.String("component", "k8s_provider"))

	var (
		cfg *rest.Config
		err error
		src string
	)

	if cfg, err = rest.InClusterConfig(); err == nil {
		src = "in_cluster"
	} else {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = clientcmd.RecommendedHomeFile
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Error("kube config build failed", zap.Error(err))
			return nil, fmt.Errorf("build kube config: %w", err)
		}
		src = "kubeconfig"
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error("k8s client init failed", zap.Error(err))
		return nil, fmt.Errorf("k8s client: %w", err)
	}

	ns := mustEnv("K8S_NAMESPACE")
	image := mustEnv("INDEXER_IMAGE")
	tag := utils.Env("INDEXER_TAG", "")

	replicas := int32FromEnv("INDEXER_REPLICAS", 1)
	// HPA is something that we may want to delete and move to use KEDA and expose
	// prometheus metrics that will lead the scaling.
	enableHPA := boolFromEnv("INDEXER_ENABLE_HPA", false)
	hpaMin := int32FromEnv("INDEXER_HPA_MIN", 1)
	hpaMax := int32FromEnv("INDEXER_HPA_MAX", 5)
	hpaCPU := int32FromEnv("INDEXER_HPA_CPU_TARGET", 80)
	tqPrefix := getEnv("TEMPORAL_TASK_QUEUE_PREFIX", "index:")

	// Image pull configuration
	pullPolicy := parsePullPolicy(getEnv("INDEXER_PULL_POLICY", "Always"))
	pullSecretName := getEnv("INDEXER_PULL_SECRET", "")

	// Container env (common + per-chain overrides later).
	env := []corev1.EnvVar{
		{Name: "CHAIN_ID", Value: ""},
		{Name: "TEMPORAL_HOSTPORT", Value: mustEnv("TEMPORAL_HOSTPORT")},
		{Name: "TEMPORAL_NAMESPACE", Value: mustEnv("TEMPORAL_NAMESPACE")},
		{Name: "CLICKHOUSE_ADDR", Value: mustEnv("CLICKHOUSE_ADDR")},
		// ClickHouse connection strategy: in_order (default), round_robin, or random
		// in_order provides read-after-write consistency for indexer (same-replica routing)
		{Name: "CLICKHOUSE_CONN_STRATEGY", Value: getEnv("CLICKHOUSE_CONN_STRATEGY", "in_order")},
		{Name: "CLICKHOUSE_CONN_MAX_LIFETIME", Value: getEnv("CLICKHOUSE_CONN_MAX_LIFETIME", "1h")},
		// Parallelism configuration - controls concurrent block processing
		// Indexer calculates connection pool sizes and Temporal worker limits from these values
		// Formula: (parallel_blocks × 35 activities × 2 connections) + 100 buffer
		{Name: "LIVE_PARALLEL_BLOCKS", Value: getEnv("LIVE_PARALLEL_BLOCKS", "5")},
		{Name: "HISTORICAL_PARALLEL_BLOCKS", Value: getEnv("HISTORICAL_PARALLEL_BLOCKS", "10")},
		{Name: "REINDEX_PARALLEL_BLOCKS", Value: getEnv("REINDEX_PARALLEL_BLOCKS", "10")},
		{Name: "REDIS_HOST", Value: mustEnv("REDIS_HOST")},
		{Name: "REDIS_PORT", Value: mustEnv("REDIS_PORT")},
		// Scheduler configuration - Customizable for different chain block times
		{Name: "SCHEDULER_CATCHUP_THRESHOLD", Value: getEnv("SCHEDULER_CATCHUP_THRESHOLD", "")},
		{Name: "DIRECT_SCHEDULE_BATCH_SIZE", Value: getEnv("DIRECT_SCHEDULE_BATCH_SIZE", "")},
		{Name: "SCHEDULER_BATCH_SIZE", Value: getEnv("SCHEDULER_BATCH_SIZE", "")},
		{Name: "BLOCK_TIME_SECONDS", Value: getEnv("BLOCK_TIME_SECONDS", "")},
		{Name: "SCHEDULER_BATCH_MAX_PARALLELISM", Value: getEnv("SCHEDULER_BATCH_MAX_PARALLELISM", "")},
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		env = append(env, corev1.EnvVar{Name: "LOG_LEVEL", Value: v})
	}
	// admin db
	if v := os.Getenv("INDEXER_DB"); v != "" {
		env = append(env, corev1.EnvVar{Name: "INDEXER_DB", Value: v})
	}

	// REDIS_PASSWORD: Read from Kubernetes secret for security
	env = append(env, corev1.EnvVar{
		Name: "REDIS_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "redis"},
				Key:                  "redis-password",
			},
		},
	})
	if v := os.Getenv("REDIS_DB"); v != "" {
		env = append(env, corev1.EnvVar{Name: "REDIS_DB", Value: v})
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

	p := &K8sProvider{
		Logger:         log,
		client:         cs,
		ns:             ns,
		image:          image,
		tag:            tag,
		replicas:       replicas,
		env:            env,
		resReq:         res,
		enableHPA:      enableHPA,
		hpaMin:         hpaMin,
		hpaMax:         hpaMax,
		hpaCPU:         hpaCPU,
		tqPrefix:       tqPrefix,
		pullPolicy:     pullPolicy,
		pullSecretName: pullSecretName,
	}

	log.Info("provider initialized",
		zap.String("config_source", src),
		zap.String("namespace", ns),
		zap.String("image", image),
		zap.String("tag", tag),
		zap.Int32("replicas_default", replicas),
		zap.Bool("hpa_enabled", enableHPA),
		zap.Int32("hpa_min", hpaMin),
		zap.Int32("hpa_max", hpaMax),
		zap.Int32("hpa_cpu_target", hpaCPU),
		zap.String("tq_prefix", tqPrefix),
		zap.String("pull_policy", string(pullPolicy)),
		zap.String("pull_secret", pullSecretName),
		zap.Bool("resources_configured", res != nil),
	)

	return p, nil
}

// EnsureChain ensures a Kubernetes Deployment exists for the specified chain, updating or creating it as necessary.
func (p *K8sProvider) EnsureChain(ctx context.Context, c *Chain) error {
	start := time.Now()
	chainIDUint, parseErr := strconv.ParseUint(c.ID, 10, 64)
	if parseErr != nil {
		p.Logger.Error("invalid chain ID format", zap.String("chain_id", c.ID), zap.Error(parseErr))
		return fmt.Errorf("invalid chain ID: %w", parseErr)
	}
	name := deploymentName(chainIDUint)
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
		default:
			env = append(env, e)
		}
	}
	ck := checksumEnv(env)

	replicas := int32(0)
	if !c.Paused && !c.Deleted {
		if c.Replicas > 0 {
			replicas = c.Replicas
		} else {
			replicas = p.replicas
		}
	}

	image := strings.TrimSpace(c.Image)
	if image == "" {
		image = p.image
		if p.tag != "" {
			image = fmt.Sprintf("%s:%s", p.image, p.tag)
		}
	}

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:            "indexer",
			Image:           image,
			ImagePullPolicy: p.pullPolicy,
			Env:             env,
			Resources: func() corev1.ResourceRequirements {
				if p.resReq != nil {
					return *p.resReq
				}
				return corev1.ResourceRequirements{}
			}(),
		}},
		// Topology spread constraints for high availability
		// Ensures pods are distributed across different nodes with maxSkew of 1
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
			{
				MaxSkew:           1,
				TopologyKey:       "kubernetes.io/hostname",
				WhenUnsatisfiable: corev1.ScheduleAnyway,
				LabelSelector: &meta.LabelSelector{
					MatchLabels: labels,
				},
			},
		},
	}

	// Add image pull secrets if configured
	if p.pullSecretName != "" {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: p.pullSecretName},
		}
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
				Spec: podSpec,
			},
		},
	}

	p.Logger.Info("ensure chain begin",
		zap.String("chain_id", c.ID),
		zap.Bool("paused", c.Paused),
		zap.Bool("deleted", c.Deleted),
		zap.String("deployment", name),
		zap.String("image", desired.Spec.Template.Spec.Containers[0].Image),
		zap.Int32("replicas_desired", replicas),
	)

	curr, err := p.client.AppsV1().Deployments(p.ns).Get(ctx, name, meta.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := p.client.AppsV1().Deployments(p.ns).Create(ctx, desired, meta.CreateOptions{}); err != nil {
			p.Logger.Error("deployment create failed", zap.String("deployment", name), zap.Error(err))
			return fmt.Errorf("create deployment: %w", err)
		}
		p.Logger.Info("deployment created", zap.String("deployment", name))
	} else if err != nil {
		p.Logger.Error("deployment get failed", zap.String("deployment", name), zap.Error(err))
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
				p.Logger.Error("deployment update failed", zap.String("deployment", name), zap.Error(err))
				return fmt.Errorf("update deployment: %w", err)
			}
			p.Logger.Info("deployment updated", zap.String("deployment", name))
		} else {
			p.Logger.Debug("deployment up-to-date", zap.String("deployment", name))
		}
	}

	// HPA
	if p.enableHPA {
		if err := p.ensureHPA(ctx, name, labels, c); err != nil {
			p.Logger.Error("hpa ensure failed", zap.String("deployment", name), zap.Error(err))
			return err
		}
	} else {
		if err := p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Delete(ctx, name, meta.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			p.Logger.Warn("hpa delete failed", zap.String("deployment", name), zap.Error(err))
		} else {
			p.Logger.Debug("hpa deleted or not present", zap.String("deployment", name))
		}
	}

	p.Logger.Info("ensure chain finished",
		zap.String("chain_id", c.ID),
		zap.String("deployment", name),
		zap.Duration("elapsed", time.Since(start)),
	)
	return nil
}

// PauseChain scales down the Kubernetes deployment associated with the given chainID to zero replicas, effectively pausing it.
func (p *K8sProvider) PauseChain(ctx context.Context, chainID string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		p.Logger.Error("invalid chain ID format", zap.String("chain_id", chainID), zap.Error(parseErr))
		return fmt.Errorf("invalid chain ID: %w", parseErr)
	}
	name := deploymentName(chainIDUint)
	p.Logger.Info("pause requested", zap.String("chain_id", chainID), zap.String("deployment", name))

	deploy, err := p.client.AppsV1().Deployments(p.ns).Get(ctx, name, meta.GetOptions{})
	if apierrors.IsNotFound(err) {
		p.Logger.Debug("deployment not found on pause (noop)", zap.String("deployment", name))
		return nil
	}
	if err != nil {
		p.Logger.Error("deployment get failed on pause", zap.String("deployment", name), zap.Error(err))
		return fmt.Errorf("get deployment: %w", err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 0 {
		deploy.Spec.Replicas = int32Ptr(0)
		if _, err := p.client.AppsV1().Deployments(p.ns).Update(ctx, deploy, meta.UpdateOptions{}); err != nil {
			p.Logger.Error("scale to zero failed", zap.String("deployment", name), zap.Error(err))
			return fmt.Errorf("scale to zero: %w", err)
		}
		p.Logger.Info("deployment scaled to zero", zap.String("deployment", name))
	} else {
		p.Logger.Debug("deployment already at zero", zap.String("deployment", name))
	}
	return nil
}

// DeleteChain removes the Kubernetes resources associated with a specific chain by its chainID.
// It deletes the deployment and its corresponding horizontal pod autoscaler, if present.
func (p *K8sProvider) DeleteChain(ctx context.Context, chainID string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		p.Logger.Error("invalid chain ID format", zap.String("chain_id", chainID), zap.Error(parseErr))
		return fmt.Errorf("invalid chain ID: %w", parseErr)
	}
	name := deploymentName(chainIDUint)
	p.Logger.Info("delete requested", zap.String("chain_id", chainID), zap.String("deployment", name))

	_ = p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Delete(ctx, name, meta.DeleteOptions{})

	propagation := meta.DeletePropagationForeground
	if err := p.client.AppsV1().Deployments(p.ns).Delete(ctx, name, meta.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !apierrors.IsNotFound(err) {
		p.Logger.Error("deployment delete failed", zap.String("deployment", name), zap.Error(err))
		return fmt.Errorf("delete deployment: %w", err)
	}
	p.Logger.Info("deployment delete issued", zap.String("deployment", name))
	return nil
}

// GetDeploymentHealth checks the health status of a chain's deployment by inspecting
// the Kubernetes deployment and pod status.
func (p *K8sProvider) GetDeploymentHealth(ctx context.Context, chainID string) (status, message string, err error) {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return "unknown", fmt.Sprintf("invalid chain ID format: %v", parseErr), parseErr
	}
	name := deploymentName(chainIDUint)

	// Get the deployment
	deploy, err := p.client.AppsV1().Deployments(p.ns).Get(ctx, name, meta.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "unknown", fmt.Sprintf("deployment %s not found", name), nil
	}
	if err != nil {
		return "unknown", fmt.Sprintf("failed to get deployment: %v", err), err
	}

	// Check if deployment exists but has no replicas specified (should not happen)
	if deploy.Spec.Replicas == nil {
		return "degraded", "deployment has no replica spec", nil
	}

	desiredReplicas := *deploy.Spec.Replicas
	readyReplicas := deploy.Status.ReadyReplicas
	availableReplicas := deploy.Status.AvailableReplicas
	updatedReplicas := deploy.Status.UpdatedReplicas
	unavailableReplicas := deploy.Status.UnavailableReplicas

	// If desired replicas is 0, this is expected (paused state)
	if desiredReplicas == 0 {
		return "healthy", "deployment is paused (0 replicas desired)", nil
	}

	// Check for various failure conditions
	conditions := deploy.Status.Conditions
	for _, condition := range conditions {
		// Check for Progressing=False which indicates deployment is stuck
		if condition.Type == appsv1.DeploymentProgressing &&
			condition.Status == corev1.ConditionFalse &&
			condition.Reason == "ProgressDeadlineExceeded" {
			return "failed", fmt.Sprintf("deployment progress deadline exceeded: %s", condition.Message), nil
		}

		// Check for Available=False which indicates deployment is not available
		if condition.Type == appsv1.DeploymentAvailable &&
			condition.Status == corev1.ConditionFalse {
			return "failed", fmt.Sprintf("deployment not available: %s", condition.Message), nil
		}
	}

	// If no pods are ready but some are desired, check pod status for more details
	if readyReplicas == 0 && desiredReplicas > 0 {
		// Get pods for this deployment
		labelSelector := fmt.Sprintf("app=indexer,chain=%s", chainID)
		pods, err := p.client.CoreV1().Pods(p.ns).List(ctx, meta.ListOptions{
			LabelSelector: labelSelector,
		})
		if err == nil && len(pods.Items) > 0 {
			// Check for common failure states in pods
			for _, pod := range pods.Items {
				for _, containerStatus := range pod.Status.ContainerStatuses {
					if waiting := containerStatus.State.Waiting; waiting != nil {
						if waiting.Reason == "CrashLoopBackOff" ||
							waiting.Reason == "ImagePullBackOff" ||
							waiting.Reason == "ErrImagePull" {
							return "failed", fmt.Sprintf("0/%d replicas ready, pod error: %s - %s",
								desiredReplicas, waiting.Reason, waiting.Message), nil
						}
					}
					if terminated := containerStatus.State.Terminated; terminated != nil {
						if terminated.ExitCode != 0 {
							return "failed", fmt.Sprintf("0/%d replicas ready, pod terminated with exit code %d: %s",
								desiredReplicas, terminated.ExitCode, terminated.Reason), nil
						}
					}
				}
			}
		}

		return "failed", fmt.Sprintf("0/%d replicas ready", desiredReplicas), nil
	}

	// Check if all replicas are ready and updated
	if readyReplicas == desiredReplicas &&
		availableReplicas == desiredReplicas &&
		updatedReplicas == desiredReplicas &&
		unavailableReplicas == 0 {
		return "healthy", fmt.Sprintf("%d/%d replicas ready", readyReplicas, desiredReplicas), nil
	}

	// Partial readiness - degraded state
	if readyReplicas > 0 && readyReplicas < desiredReplicas {
		return "degraded", fmt.Sprintf("%d/%d replicas ready (expected %d)",
			readyReplicas, desiredReplicas, desiredReplicas), nil
	}

	// Replicas are ready but not all updated (rolling update in progress)
	if readyReplicas == desiredReplicas && updatedReplicas < desiredReplicas {
		return "degraded", fmt.Sprintf("%d/%d replicas ready, %d/%d updated (rolling update in progress)",
			readyReplicas, desiredReplicas, updatedReplicas, desiredReplicas), nil
	}

	// Default to degraded if we don't have perfect health
	return "degraded", fmt.Sprintf("replicas: %d ready, %d available, %d updated, %d unavailable (desired: %d)",
		readyReplicas, availableReplicas, updatedReplicas, unavailableReplicas, desiredReplicas), nil
}

// Close releases resources or performs cleanup tasks associated with the K8sProvider instance.
func (p *K8sProvider) Close() error {
	p.Logger.Info("provider closed")
	return nil
}

// EnsureReindexWorker ensures that a reindex worker deployment exists with the specified replicas.
func (p *K8sProvider) EnsureReindexWorker(ctx context.Context, c *Chain) error {
	start := time.Now()
	chainIDUint, parseErr := strconv.ParseUint(c.ID, 10, 64)
	if parseErr != nil {
		p.Logger.Error("invalid chain ID format", zap.String("chain_id", c.ID), zap.Error(parseErr))
		return fmt.Errorf("invalid chain ID: %w", parseErr)
	}
	name := reindexDeploymentName(chainIDUint)
	labels := map[string]string{
		"app":         "indexer-reindex",
		"managed-by":  "canopyx-controller",
		"chain":       c.ID,
		"worker-type": "reindex",
	}

	// Per-chain env (same as regular indexer, but with WORKER_MODE=reindex)
	env := make([]corev1.EnvVar, 0, len(p.env)+1)
	for _, e := range p.env {
		switch e.Name {
		case "CHAIN_ID":
			env = append(env, corev1.EnvVar{Name: "CHAIN_ID", Value: c.ID})
		default:
			env = append(env, e)
		}
	}
	// Add WORKER_MODE to distinguish reindex workers
	env = append(env, corev1.EnvVar{Name: "WORKER_MODE", Value: "reindex"})

	// Build deployment spec (same as regular indexer)
	replicas := c.Replicas

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:            "indexer-reindex",
			Image:           c.Image,
			ImagePullPolicy: p.pullPolicy,
			Env:             env,
		}},
		// Topology spread constraints for high availability
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
			{
				MaxSkew:           1,
				TopologyKey:       "kubernetes.io/hostname",
				WhenUnsatisfiable: corev1.ScheduleAnyway,
				LabelSelector: &meta.LabelSelector{
					MatchLabels: labels,
				},
			},
		},
	}

	// Add image pull secrets if configured
	if p.pullSecretName != "" {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: p.pullSecretName},
		}
	}

	// Add resources if configured
	if p.resReq != nil {
		podSpec.Containers[0].Resources = *p.resReq
	}

	dep := &appsv1.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      name,
			Namespace: p.ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &meta.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: meta.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}

	// Get or create deployment
	existing, err := p.client.AppsV1().Deployments(p.ns).Get(ctx, name, meta.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new deployment
			if _, err := p.client.AppsV1().Deployments(p.ns).Create(ctx, dep, meta.CreateOptions{}); err != nil {
				p.Logger.Error("reindex deployment create failed", zap.String("chain_id", c.ID), zap.String("deployment", name), zap.Error(err))
				return fmt.Errorf("create reindex deployment %s: %w", name, err)
			}
			p.Logger.Info("reindex deployment created",
				zap.String("chain_id", c.ID),
				zap.String("deployment", name),
				zap.Int32("replicas", replicas),
				zap.Duration("elapsed", time.Since(start)))
			return nil
		}
		return fmt.Errorf("get reindex deployment %s: %w", name, err)
	}

	// Update if needed
	if needsUpdate(existing, dep) {
		existing.Spec = dep.Spec
		if _, err := p.client.AppsV1().Deployments(p.ns).Update(ctx, existing, meta.UpdateOptions{}); err != nil {
			p.Logger.Error("reindex deployment update failed", zap.String("chain_id", c.ID), zap.String("deployment", name), zap.Error(err))
			return fmt.Errorf("update reindex deployment %s: %w", name, err)
		}
		p.Logger.Info("reindex deployment updated",
			zap.String("chain_id", c.ID),
			zap.String("deployment", name),
			zap.Int32("replicas", replicas),
			zap.Duration("elapsed", time.Since(start)))
	} else {
		p.Logger.Debug("reindex deployment unchanged", zap.String("chain_id", c.ID), zap.String("deployment", name))
	}

	return nil
}

// DeleteReindexWorker removes the reindex worker deployment for the given chain.
func (p *K8sProvider) DeleteReindexWorker(ctx context.Context, chainID string) error {
	start := time.Now()
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		p.Logger.Error("invalid chain ID format", zap.String("chain_id", chainID), zap.Error(parseErr))
		return fmt.Errorf("invalid chain ID: %w", parseErr)
	}
	name := reindexDeploymentName(chainIDUint)

	// Delete deployment
	deletePolicy := meta.DeletePropagationForeground
	if err := p.client.AppsV1().Deployments(p.ns).Delete(ctx, name, meta.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		if !apierrors.IsNotFound(err) {
			p.Logger.Error("reindex deployment delete failed", zap.String("chain_id", chainID), zap.String("deployment", name), zap.Error(err))
			return fmt.Errorf("delete reindex deployment %s: %w", name, err)
		}
		p.Logger.Debug("reindex deployment not found (already deleted)", zap.String("chain_id", chainID), zap.String("deployment", name))
	} else {
		p.Logger.Info("reindex deployment deleted",
			zap.String("chain_id", chainID),
			zap.String("deployment", name),
			zap.Duration("elapsed", time.Since(start)))
	}

	return nil
}

// ensureHPA ensures that the given HPA exists and is configured as expected.
func (p *K8sProvider) ensureHPA(ctx context.Context, name string, labels map[string]string, c *Chain) error {
	start := time.Now()

	replicas := c.Replicas

	hpaMin := c.MinReplicas
	if hpaMin < 0 {
		hpaMin = 0
	}
	hpaMax := c.MaxReplicas
	if hpaMax < hpaMin {
		hpaMax = hpaMin
	}
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
		if _, err := p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Create(ctx, hpaDesired, meta.CreateOptions{}); err != nil {
			p.Logger.Error("hpa create failed", zap.String("deployment", name), zap.Error(err))
			return err
		}
		p.Logger.Info("hpa created",
			zap.String("deployment", name),
			zap.Int32("min", hpaMin),
			zap.Int32("max", hpaMax),
			zap.Int32("cpu_target", targetCPU),
			zap.Duration("elapsed", time.Since(start)),
		)
		return nil
	}
	if err != nil {
		p.Logger.Error("hpa get failed", zap.String("deployment", name), zap.Error(err))
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
		if _, err := p.client.AutoscalingV2().HorizontalPodAutoscalers(p.ns).Update(ctx, curr, meta.UpdateOptions{}); err != nil {
			p.Logger.Error("hpa update failed", zap.String("deployment", name), zap.Error(err))
			return err
		}
		p.Logger.Info("hpa updated",
			zap.String("deployment", name),
			zap.Int32("min", hpaMin),
			zap.Int32("max", hpaMax),
			zap.Int32("cpu_target", targetCPU),
			zap.Duration("elapsed", time.Since(start)),
		)
		return nil
	}

	p.Logger.Debug("hpa up-to-date",
		zap.String("deployment", name),
		zap.Int32("min", hpaMin),
		zap.Int32("max", hpaMax),
		zap.Int32("cpu_target", targetCPU),
		zap.Duration("elapsed", time.Since(start)),
	)
	return nil
}

// ---- utils ------------------------------------------------------------------

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
func deploymentName(chainID uint64) string {
	// Convert uint64 to string for DNS name generation
	s := strconv.FormatUint(chainID, 10)
	// Numeric strings are already DNS-compliant, no need for complex transformations
	if !strings.HasPrefix(s, "canopyx-indexer-") {
		s = "canopyx-indexer-" + s
	}
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// reindexDeploymentName generates a DNS-compliant deployment name for reindex workers.
func reindexDeploymentName(chainID uint64) string {
	s := "canopyx-indexer-reindex-" + strconv.FormatUint(chainID, 10)
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

// parsePullPolicy parses image pull policy from string
func parsePullPolicy(policy string) corev1.PullPolicy {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "always":
		return corev1.PullAlways
	case "never":
		return corev1.PullNever
	case "ifnotpresent":
		return corev1.PullIfNotPresent
	default:
		return corev1.PullAlways // Safe default for production
	}
}
