package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	queuev1alpha1 "github.com/Qinfff/taskqueue-operator/api/v1alpha1"
)

const (
	componentWorker = "worker"
	componentAPI    = "api"

	// 这两个名字来自 deploy/helm/taskqueue/templates/config.yaml
	configMapName = "app-config"
	secretName    = "app-secret"

	defaultImageRepo = "ghcr.io/qinfff/task_queue-api"
	defaultImageTag  = "latest"

	requeueAfter = 30 * time.Second
)

type TaskQueueReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=queue.example.com,resources=taskqueues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=queue.example.com,resources=taskqueues/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps,verbs=get;list;watch;create;update;patch

// Reconcile 让"实际状态"追上"期望状态"，会被无限次调用。
func (r *TaskQueueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. 拿期望状态。req 里只有名字，所以必须自己去查。
	var tq queuev1alpha1.TaskQueue
	if err := r.Get(ctx, req.NamespacedName, &tq); err != nil {
		// 对象被删了是正常情况，返回 nil 表示"处理完了，别重试"
		if apierrors.IsNotFound(err) {
			logger.Info("TaskQueue 已被删除，子资源由 OwnerReference 级联清理")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. 用户可能只写了两行，其余字段补默认值
	applyDefaults(&tq)

	// 3. 确保业务负载存在且符合期望
	for _, comp := range []string{componentWorker, componentAPI} {
		if err := r.ensureDeployment(ctx, &tq, comp); err != nil {
			return r.markDegraded(ctx, &tq, err)
		}
	}

	// 4. 确保 api 的 Service 存在（Prometheus 抓指标、外部访问都靠它）
	if err := r.ensureService(ctx, &tq); err != nil {
		return r.markDegraded(ctx, &tq, err)
	}

	// 5. 写回状态，并预约 30 秒后复查
	return r.markRunning(ctx, &tq)
}

func (r *TaskQueueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&queuev1alpha1.TaskQueue{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// ensureDeployment 是控制器的灵魂：查一次，没有就建，不一致就改，一致就什么都不做。
func (r *TaskQueueReconciler) ensureDeployment(ctx context.Context, tq *queuev1alpha1.TaskQueue, component string) error {
	logger := log.FromContext(ctx)
	desired := r.deploymentFor(tq, component)

	// desired 同时提供了三样东西：查用的名字、比对用的期望值、创建用的完整对象
	if err := ctrl.SetControllerReference(tq, desired, r.Scheme); err != nil {
		return fmt.Errorf("设置 OwnerReference 失败: %w", err)
	}

	var existing appsv1.Deployment
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("创建 Deployment", "component", component, "name", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// 只比对两个字段：全量比对会因为 K8s 注入默认值（strategy 等）导致永远不相等 → 死循环
	if *existing.Spec.Replicas == *desired.Spec.Replicas &&
		existing.Spec.Template.Spec.Containers[0].Image == desired.Spec.Template.Spec.Containers[0].Image {
		return nil
	}

	logger.Info("修正 Deployment", "component", component,
		"oldReplicas", *existing.Spec.Replicas, "newReplicas", *desired.Spec.Replicas)
	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.Template = desired.Spec.Template
	return r.Update(ctx, &existing)
}

func (r *TaskQueueReconciler) ensureService(ctx context.Context, tq *queuev1alpha1.TaskQueue) error {
	desired := r.serviceFor(tq)

	if err := ctrl.SetControllerReference(tq, desired, r.Scheme); err != nil {
		return fmt.Errorf("设置 OwnerReference 失败: %w", err)
	}

	var existing corev1.Service
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	return err
}

// deploymentFor 把 spec 翻译成一个"期望的 Deployment"。
// 这里是 spec 里每个字段真正被使用的地方。
func (r *TaskQueueReconciler) deploymentFor(tq *queuev1alpha1.TaskQueue, component string) *appsv1.Deployment {
	labels := labelsFor(tq, component)

	replicas := tq.Spec.Worker.ReplicaCount
	command := []string{"/app/worker"}
	resources := tq.Spec.Worker.Resources
	containerPort := tq.Spec.Worker.MetricsPort
	httpPort := int32(0)

	if component == componentAPI {
		replicas = tq.Spec.API.ReplicaCount
		command = []string{"/app/server"}
		resources = tq.Spec.API.Resources
		containerPort = tq.Spec.API.MetricsPort
		httpPort = tq.Spec.API.ServicePort
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", tq.Name, component),
			Namespace: tq.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            component,
							Image:           imageRef(tq),
							ImagePullPolicy: corev1.PullPolicy(tq.Spec.Image.PullPolicy),
							Command:         command,
							Ports: []corev1.ContainerPort{
								{Name: "metrics", ContainerPort: containerPort},
							},
							Resources: resources,
							Env:       envFor(httpPort, containerPort),
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
								}},
								{SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
								}},
							},
						},
					},
				},
			},
		},
	}
}

func (r *TaskQueueReconciler) serviceFor(tq *queuev1alpha1.TaskQueue) *corev1.Service {
	labels := labelsFor(tq, componentAPI)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", tq.Name, componentAPI),
			Namespace: tq.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       tq.Spec.API.ServicePort,
					TargetPort: intstr.FromInt32(tq.Spec.API.ServicePort),
				},
				{
					Name:       "metrics",
					Port:       tq.Spec.API.MetricsPort,
					TargetPort: intstr.FromInt32(tq.Spec.API.MetricsPort),
				},
			},
		},
	}
}

func imageRef(tq *queuev1alpha1.TaskQueue) string {
	return fmt.Sprintf("%s:%s", tq.Spec.Image.Repository, tq.Spec.Image.Tag)
}

// envFor 只补 code 里没有默认值的那部分；MYSQL_DSN / REDIS_ADDR 走 envFrom
func envFor(httpPort, metricsPort int32) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "METRICS_ADDR", Value: fmt.Sprintf(":%d", metricsPort)},
	}
	if httpPort > 0 {
		env = append(env, corev1.EnvVar{Name: "HTTP_ADDR", Value: fmt.Sprintf(":%d", httpPort)})
	}
	return env
}

// labelsFor 同时用于 Deployment 的 selector 和 Pod 的 labels —— 两边必须完全一致
func labelsFor(tq *queuev1alpha1.TaskQueue, component string) map[string]string {
	return map[string]string{
		"app":                    component,
		"app.kubernetes.io/name": tq.Name,
		"managed-by":             "taskqueue-operator",
	}
}

func (r *TaskQueueReconciler) markRunning(ctx context.Context, tq *queuev1alpha1.TaskQueue) (ctrl.Result, error) {
	// 状态没变就不写，避免每 30 秒白打一次 API
	if tq.Status.Phase != queuev1alpha1.PhaseRunning ||
		tq.Status.ObservedGeneration != tq.Generation ||
		!tq.Status.Ready {
		tq.Status.Phase = queuev1alpha1.PhaseRunning
		tq.Status.Ready = true
		tq.Status.Message = "所有组件已就绪"
		// 记录"我处理到第几版 spec 了"，让用户知道 Operator 有没有跟上
		tq.Status.ObservedGeneration = tq.Generation
		// status 是独立子资源，必须用 Status().Update()
		if err := r.Status().Update(ctx, tq); err != nil {
			return handleStatusConflict(err)
		}
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *TaskQueueReconciler) markDegraded(ctx context.Context, tq *queuev1alpha1.TaskQueue, cause error) (ctrl.Result, error) {
	tq.Status.Phase = queuev1alpha1.PhaseDegraded
	tq.Status.Ready = false
	tq.Status.Message = cause.Error()
	tq.Status.ObservedGeneration = tq.Generation
	if err := r.Status().Update(ctx, tq); err != nil {
		return handleStatusConflict(err)
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// handleStatusConflict 把写 status 的冲突降级成"立刻重来一次"。
//
// 冲突是常态而非故障：Owns() 让"创建 Deployment"本身也触发新一轮 Reconcile，
// 两个 Reconcile 拿着同一个 resourceVersion 写 status 时，后写的必然 409。
// 这种冲突会自愈（重来一次就读到新版本），不该记成 ERROR ——
// 否则值班的人会被噪音训练到忽略告警，那才是真正的损失。
func handleStatusConflict(err error) (ctrl.Result, error) {
	if apierrors.IsConflict(err) {
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, err
}

// applyDefaults 兜住"用户只写了两行"的情况。
// schema 的 minimum 只管上界合理，这里管"从零值补成可用值"。
func applyDefaults(tq *queuev1alpha1.TaskQueue) {
	if tq.Spec.Image.Repository == "" {
		tq.Spec.Image.Repository = defaultImageRepo
	}
	if tq.Spec.Image.Tag == "" {
		tq.Spec.Image.Tag = defaultImageTag
	}
	if tq.Spec.Image.PullPolicy == "" {
		tq.Spec.Image.PullPolicy = string(corev1.PullIfNotPresent)
	}
	if tq.Spec.Worker.ReplicaCount <= 0 {
		tq.Spec.Worker.ReplicaCount = 1
	}
	if tq.Spec.Worker.MetricsPort <= 0 {
		tq.Spec.Worker.MetricsPort = 9090
	}
	if tq.Spec.API.ReplicaCount <= 0 {
		tq.Spec.API.ReplicaCount = 1
	}
	if tq.Spec.API.ServicePort <= 0 {
		tq.Spec.API.ServicePort = 8080
	}
	if tq.Spec.API.MetricsPort <= 0 {
		tq.Spec.API.MetricsPort = 9090
	}
}
