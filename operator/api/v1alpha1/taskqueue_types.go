package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TaskQueueSpec 是用户写的期望状态。
// 这里只放"真的会被 Operator 使用"的字段 —— 声明了却不实现比不声明更糟糕。
type TaskQueueSpec struct {
	Image  ImageSpec  `json:"image,omitempty"`
	Worker WorkerSpec `json:"worker,omitempty"`
	API    APISpec    `json:"api,omitempty"`
}

type ImageSpec struct {
	Repository string `json:"repository,omitempty"`
	Tag        string `json:"tag,omitempty"`
	PullPolicy string `json:"pullPolicy,omitempty"`
}

type WorkerSpec struct {
	ReplicaCount int32                       `json:"replicaCount,omitempty"`
	MetricsPort  int32                       `json:"metricsPort,omitempty"`
	Resources    corev1.ResourceRequirements `json:"resources,omitempty"`
}

type APISpec struct {
	ReplicaCount int32                       `json:"replicaCount,omitempty"`
	ServicePort  int32                       `json:"servicePort,omitempty"`
	MetricsPort  int32                       `json:"metricsPort,omitempty"`
	Resources    corev1.ResourceRequirements `json:"resources,omitempty"`
}

// TaskQueueStatus 是 Operator 写的实际状态。用户不写这个字段。
type TaskQueueStatus struct {
	Phase              string `json:"phase,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	Ready              bool   `json:"ready,omitempty"`
	Message            string `json:"message,omitempty"`
}

const (
	PhasePending  = "Pending"
	PhaseRunning  = "Running"
	PhaseDegraded = "Degraded"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type TaskQueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskQueueSpec   `json:"spec,omitempty"`
	Status TaskQueueStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type TaskQueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TaskQueue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TaskQueue{}, &TaskQueueList{})
}

// ---- DeepCopy ----
// 值类型只需 *out = *in；只有 ResourceRequirements 内部有 map，要调它自带的实现。

func (in *ImageSpec) DeepCopyInto(out *ImageSpec) { *out = *in }

func (in *WorkerSpec) DeepCopyInto(out *WorkerSpec) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
}

func (in *APISpec) DeepCopyInto(out *APISpec) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
}

func (in *TaskQueueSpec) DeepCopyInto(out *TaskQueueSpec) {
	*out = *in
	out.Image = in.Image
	in.Worker.DeepCopyInto(&out.Worker)
	in.API.DeepCopyInto(&out.API)
}

func (in *TaskQueueStatus) DeepCopyInto(out *TaskQueueStatus) { *out = *in }

func (in *TaskQueue) DeepCopyInto(out *TaskQueue) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *TaskQueue) DeepCopy() *TaskQueue {
	if in == nil {
		return nil
	}
	out := new(TaskQueue)
	in.DeepCopyInto(out)
	return out
}

func (in *TaskQueue) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *TaskQueueList) DeepCopyInto(out *TaskQueueList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		l := make([]TaskQueue, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&l[i])
		}
		out.Items = l
	}
}

func (in *TaskQueueList) DeepCopy() *TaskQueueList {
	if in == nil {
		return nil
	}
	out := new(TaskQueueList)
	in.DeepCopyInto(out)
	return out
}

func (in *TaskQueueList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
