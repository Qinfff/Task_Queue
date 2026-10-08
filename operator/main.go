package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	queuev1alpha1 "github.com/Qinfff/taskqueue-operator/api/v1alpha1"
	"github.com/Qinfff/taskqueue-operator/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	// 这两行决定 client 能认识哪些类型。
	// 少一行就会在 r.Get() 时报 "no kind is registered for the type"。
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(queuev1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8081", "Operator 自身指标监听地址")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8082", "健康检查监听地址")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"多副本时只让一个实例真正干活，避免互相覆盖 status")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// 用 kubeconfig 或 in-cluster ServiceAccount 连集群
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "taskqueue-operator.queue.example.com",
	})
	if err != nil {
		setupLog.Error(err, "创建 manager 失败")
		os.Exit(1)
	}

	// 把 Reconciler 注册进去 —— 这一步之后 watch 才开始工作
	if err := (&controllers.TaskQueueReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "注册 TaskQueueReconciler 失败")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "添加 healthz 失败")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "添加 readyz 失败")
		os.Exit(1)
	}

	setupLog.Info("启动 Operator，开始 watch TaskQueue")
	// 这行会一直阻塞 —— 里面就是那个永不停止的 reconcile 循环
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "manager 异常退出")
		os.Exit(1)
	}
}
