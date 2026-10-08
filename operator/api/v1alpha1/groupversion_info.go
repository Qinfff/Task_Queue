package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion 是这个 API 组的身份证：queue.example.com/v1alpha1
	GroupVersion = schema.GroupVersion{Group: "queue.example.com", Version: "v1alpha1"}

	// SchemeBuilder 收集所有要注册的类型。
	// types.go 里的 init() 会往这里 Register(&TaskQueue{}, &TaskQueueList{})
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme 交给 Scheme，供 client 编解码用
	AddToScheme = SchemeBuilder.AddToScheme
)
