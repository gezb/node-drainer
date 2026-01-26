package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/fields"

	"github.com/gezb/node-drainer/internal/events"
	"github.com/gezb/node-drainer/internal/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubectl/pkg/drain"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gezbcoukalphav1 "github.com/gezb/node-drainer/api/v1"
)

const (
	FixedDurationReconcileLog = "Reconciling with fixed duration"
	// An expected error from fetchNode function
	expectedNodeNotFoundErrorMsg = "nodes \"%s\" not found"
	DrainerTimeout               = 30 * time.Second
)

// NodeDrainReconciler reconciles a NodeDrain object
type NodeDrainReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	logger           logr.Logger
	MgrConfig        *rest.Config
	Recorder         events.Recorder
	PodStatusChecker utils.PodStatusChecker
}

// +kubebuilder:rbac:groups=k8s.gezb.co.uk,resources=nodedrains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.gezb.co.uk,resources=nodedrains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8s.gezb.co.uk,resources=nodedrains/finalizers,verbs=update
// +kubebuilder:rbac:groups=k8s.gezb.co.uk,resources=drainchecks,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;update;patch;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;create
// +kubebuilder:rbac:groups="apps",resources=deployments;daemonsets;replicasets;statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="policy",resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=list;watch;create;update;patch;delete

func (r *NodeDrainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger = log.FromContext(ctx)

	nodeDrain := &gezbcoukalphav1.NodeDrain{}
	err := r.Get(ctx, req.NamespacedName, nodeDrain)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			r.logger.Info("NodeDrain not found", "name", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.logger.Info("Error reading the request object, re-queuing.")
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(nodeDrain, gezbcoukalphav1.NodeDrainFinalizer) && nodeDrain.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer when object is created
		controllerutil.AddFinalizer(nodeDrain, gezbcoukalphav1.NodeDrainFinalizer)

	} else if controllerutil.ContainsFinalizer(nodeDrain, gezbcoukalphav1.NodeDrainFinalizer) && !nodeDrain.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted

		// Do nothing special on deletion for now

		// Remove finalizer
		controllerutil.RemoveFinalizer(nodeDrain, gezbcoukalphav1.NodeDrainFinalizer)
		if err := r.Client.Update(ctx, nodeDrain); err != nil {
			return r.onReconcileError(ctx, nil, nodeDrain, err)
		}
		return ctrl.Result{}, nil
	}

	// create a drain Helper
	drainer, err := createDrainer(ctx, r.MgrConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	// new CRD
	if nodeDrain.Status.Phase == "" {
		setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhasePending)

		if err = r.Client.Status().Update(ctx, nodeDrain); err != nil {
			r.logger.Error(err, "Failed to update NodeDrain with \"Pending\" status")
			return r.onReconcileError(ctx, drainer, nodeDrain, err)
		}
		return ctrl.Result{}, nil
	}
	needUpdate := false
	drainError := false
	needsRequeue := false
	// switch on status
	switch nodeDrain.Status.Phase {
	case gezbcoukalphav1.NodeDrainPhasePending:
		needUpdate, err = r.reconcilePending(ctx, drainer, nodeDrain)
	case gezbcoukalphav1.NodeDrainPhaseCordoned:
		needUpdate, needsRequeue, err = r.reconcileCordoned(ctx, drainer, nodeDrain)
	case gezbcoukalphav1.NodeDrainPhasePodsBlocking:
		needUpdate, needsRequeue, err = r.reconcileCordoned(ctx, drainer, nodeDrain)
	case gezbcoukalphav1.NodeDrainPhaseOtherNodesNotCordoned:
		needUpdate, needsRequeue, err = r.reconcileCordoned(ctx, drainer, nodeDrain)
	case gezbcoukalphav1.NodeDrainPhaseDraining:
		needUpdate = true
		needsRequeue, drainError, err = r.reconcileDraining(drainer, nodeDrain)
	case gezbcoukalphav1.NodeDrainPhaseWaitForPodsToRestart:
		needUpdate = true
		needsRequeue = r.reconcileWaitForPodsToRestart(ctx, nodeDrain)

	}

	if err != nil {
		if drainError {
			waitOnReconcile := 5 * time.Second
			return r.onReconcileErrorWithRequeue(ctx, drainer, nodeDrain, err, &waitOnReconcile)
		}
		return r.onReconcileError(ctx, drainer, nodeDrain, err)
	}

	if needUpdate {
		newStatus := nodeDrain.Status.DeepCopy()
		updateErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Fetch the latest version of NodeDrain before attempting update
			getErr := r.Get(ctx, req.NamespacedName, nodeDrain)
			if getErr != nil {
				return getErr
			}
			nodeDrain.Status = *newStatus
			return r.Client.Status().Update(ctx, nodeDrain)
		})
		if updateErr != nil {
			r.logger.Error(err, "Failed to update NodeDrain Status")
			return r.onReconcileError(ctx, drainer, nodeDrain, updateErr)
		}
	}
	if needsRequeue {
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

// reconcilePending checks the node exists, cordens the node if it needs to and updated
func (r *NodeDrainReconciler) reconcilePending(ctx context.Context, drainer *drain.Helper, nodeDrain *gezbcoukalphav1.NodeDrain) (bool, error) {
	nodeName := nodeDrain.Spec.NodeName
	node, err := r.fetchNode(ctx, drainer, nodeName)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			r.logger.Error(err, "Didn't find a node matching the NodeName field", "NodeName", nodeName)
			message := fmt.Sprintf("Node: %s not found", nodeName)
			publishEvent(r.Recorder, nodeDrain, corev1.EventTypeWarning, events.EventReasonNodeNotFound, message)
			setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhaseFailed)
			return true, nil
		} else {
			r.logger.Error(err, "Unexpected error for the NodeName field", "NodeName", nodeName)
			return false, err
		}
	}

	if nodeDrain.Spec.DisableCordon {
		r.logger.Info("NOT Cordoning node", "node", nodeName)
	} else {
		if !node.Spec.Unschedulable {
			// cordon node
			r.logger.Info("Cordoning node", "node", nodeName)
			if err = drain.RunCordonOrUncordon(drainer, node, true); err != nil {
				return false, err
			}
		}
	}

	podlist, err := drainer.Client.CoreV1().Pods(metav1.NamespaceAll).List(ctx,
		metav1.ListOptions{
			FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeDrain.Spec.NodeName}).String(),
		})
	if err != nil {
		return false, err
	}
	nodeDrain.Status.TotalPods = len(podlist.Items)

	err = r.updatePendingPodCount(drainer, nodeDrain)
	nodeDrain.Status.EvictionPodCount = len(nodeDrain.Status.PendingPods)
	if err != nil {
		return false, err
	}
	setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhaseCordoned)

	return true, nil
}

// reconcileCordoned Checks that no pods exist that block draining this node & Checks all nodes for this k8s version&role are cordened,
// once both are true update status to Draining
func (r *NodeDrainReconciler) reconcileCordoned(ctx context.Context, drainer *drain.Helper, nodeDrain *gezbcoukalphav1.NodeDrain) (bool, bool, error) {
	// update status with pods to be deleted
	// ignore updated here we need to save the CR anyway
	err := r.updatePendingPodCount(drainer, nodeDrain)
	if err != nil {
		return false, true, err
	}

	podsBlocking, err := r.getBlockingPods(ctx, nodeDrain)
	if err != nil {
		return false, false, err
	}
	nodeDrain.Status.PodsBlockingDrain = strings.Join(podsBlocking, ",")
	if len(podsBlocking) > 0 {
		setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhasePodsBlocking)
		// requeue this request so we check when the blocking pod(s) have been removed
		return true, true, nil
	}

	allNodesCordoned, err := r.areAllNodesCordoned(ctx, nodeDrain)
	if err != nil {
		return true, false, err
	}
	if !allNodesCordoned {
		setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhaseOtherNodesNotCordoned)
		// requeue this request so we check when the blocking pod(s) have been removed
		return true, true, nil
	}
	setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhaseDraining)
	return true, false, nil
}

// reconcileDraining Drains the given node
func (r *NodeDrainReconciler) reconcileDraining(drainer *drain.Helper, nodeDrain *gezbcoukalphav1.NodeDrain) (bool, bool, error) {
	if nodeDrain.Status.Phase == gezbcoukalphav1.NodeDrainPhaseDraining {

		pendingList, errlist := drainer.GetPodsForDeletion(nodeDrain.Spec.NodeName)
		if errlist != nil {
			return false, false, fmt.Errorf("failed to get pods for eviction while initializing status: %v", errlist)
		}
		if pendingList != nil {
			nodeDrain.Status.PodsToBeEvicted = GetNameSpaceAndName(pendingList.Pods())
		}
		nodeName := nodeDrain.Spec.NodeName
		r.logger.Info(fmt.Sprintf("Evict all Pods from Node %s", nodeName), "nodeName", nodeName)
		if err := drain.RunNodeDrain(drainer, nodeName); err != nil {
			r.logger.Info("Not all pods evicted", "nodeName", nodeName, "error", err)
			setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhaseFailed)
			// ignore updated here we need to save the CR anyway
			err = r.updatePendingPodCount(drainer, nodeDrain)
			if err != nil {
				return false, true, err
			}
			return false, true, nil
		}
		err := r.updatePendingPodCount(drainer, nodeDrain)
		if err != nil {
			return false, false, err
		}
		nodeDrain.Status.DrainProgress = 100

	}
	if nodeDrain.Spec.WaitForPodsToRestart {
		message := fmt.Sprintf("Waiting for %d pod(s) to restart", len(nodeDrain.Status.PodsToBeEvicted))
		publishEvent(r.Recorder, nodeDrain, corev1.EventTypeNormal, events.EventReasonWaitingForPodsToRestart, message)
		setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhaseWaitForPodsToRestart)
		// requeue to wait for the pods to be running
		return true, true, nil
	} else {
		setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhaseCompleted)
		return false, false, nil
	}
}

// reconcileWaitForPodsToRestart wiats for all evicted pods to be running again
func (r *NodeDrainReconciler) reconcileWaitForPodsToRestart(ctx context.Context, nodeDrain *gezbcoukalphav1.NodeDrain) bool {
	allPodsRunning := false

	for _, nameAndNamespace := range nodeDrain.Status.PodsToBeEvicted {
		pod := corev1.Pod{}
		err := r.Client.Get(ctx, client.ObjectKey{Namespace: nameAndNamespace.Namespace, Name: nameAndNamespace.Name}, &pod)
		if err != nil {
			r.logger.Info(fmt.Sprintf("Failed to get pod %s in namespace %s", nameAndNamespace.Name, nameAndNamespace.Namespace))

		} else {
			// check if the pod is running
			if !r.PodStatusChecker.CheckStatus(&pod) {
				r.logger.Info("Pod not running yet", "Pod.Namespace", nameAndNamespace.Namespace, "Pod.Name", nameAndNamespace.Name)
				allPodsRunning = false
			} else {
				r.logger.Info("Pod running", "Pod.Namespace", nameAndNamespace.Namespace, "Pod.Name", nameAndNamespace.Name)
				allPodsRunning = true
			}
		}
	}

	if allPodsRunning {
		r.logger.Info("All pods running, updating status to complete")
		setStatus(r.Recorder, nodeDrain, gezbcoukalphav1.NodeDrainPhaseCompleted)
		return false
	}
	return true
}

func (r *NodeDrainReconciler) getBlockingPods(ctx context.Context, nodeDrain *gezbcoukalphav1.NodeDrain) ([]string, error) {
	podsBlocking := []string{}
	// get a list of drainChecks
	drainCheckList := &gezbcoukalphav1.DrainCheckList{}
	err := r.Client.List(ctx, drainCheckList, &client.ListOptions{})
	if err != nil {
		return podsBlocking, err
	}
	for _, drainCheck := range drainCheckList.Items {
		pattern := regexp.MustCompile(drainCheck.Spec.PodRegex)
		for _, podName := range nodeDrain.Status.PendingPods {
			if pattern.MatchString(podName) {
				blockingMessage := fmt.Sprintf("Node %s is blocked from draining by pod: %s", nodeDrain.Spec.NodeName, podName)
				r.logger.Info(blockingMessage)
				publishEvent(r.Recorder, nodeDrain, corev1.EventTypeWarning, events.EventReasonDrainBlockedByPods, blockingMessage)
				r.logger.Info(fmt.Sprintf("Node %s is blocked from draining by pod: %s", nodeDrain.Spec.NodeName, podName))
				podsBlocking = append(podsBlocking, podName)
			}
		}
	}
	return podsBlocking, nil
}

func (r *NodeDrainReconciler) fetchNode(ctx context.Context, drainer *drain.Helper, nodeName string) (*corev1.Node, error) {
	node, err := drainer.Client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil && apiErrors.IsNotFound(err) {
		r.logger.Error(err, "Node cannot be found", "nodeName", nodeName)
		return nil, err
	} else if err != nil {
		r.logger.Error(err, "Failed to get node", "nodeName", nodeName)
		return nil, err
	}
	return node, nil
}

func (r *NodeDrainReconciler) areAllNodesCordoned(ctx context.Context, nodeDrain *gezbcoukalphav1.NodeDrain) (bool, error) {
	nodeList := &corev1.NodeList{}
	err := r.Client.List(ctx, nodeList, &client.ListOptions{})
	if err != nil {
		return false, err
	}

	pattern := regexp.MustCompile(nodeDrain.Spec.VersionToDrainRegex)
	unschedulable := true
	for _, node := range nodeList.Items {
		if node.Name != nodeDrain.Spec.NodeName && // ignore ourselves
			node.Labels["role"] == nodeDrain.Spec.NodeRole &&
			pattern.MatchString(node.Status.NodeInfo.KubeletVersion) {
			if !node.Spec.Unschedulable {
				unschedulable = false
			}
		}
	}
	return unschedulable, nil
}

func (r *NodeDrainReconciler) updatePendingPodCount(drainer *drain.Helper, nodeDrain *gezbcoukalphav1.NodeDrain) error {
	setLastUpdate(nodeDrain)
	pendingList, errlist := drainer.GetPodsForDeletion(nodeDrain.Spec.NodeName)
	if errlist != nil {
		return fmt.Errorf("failed to get pods for eviction while initializing status: %v", errlist)
	}
	if pendingList != nil {
		nodeDrain.Status.PendingPods = GetPodNameList(pendingList.Pods())
	} else {
		nodeDrain.Status.PendingPods = []string{}
	}
	if nodeDrain.Status.EvictionPodCount > 0 {
		nodeDrain.Status.DrainProgress = (nodeDrain.Status.EvictionPodCount - len(nodeDrain.Status.PendingPods)) * 100 / nodeDrain.Status.EvictionPodCount
	}
	return nil
}

func (r *NodeDrainReconciler) onReconcileErrorWithRequeue(ctx context.Context, drainer *drain.Helper, nodeDrain *gezbcoukalphav1.NodeDrain, err error, duration *time.Duration) (ctrl.Result, error) {
	nodeDrain.Status.LastError = err.Error()
	setLastUpdate(nodeDrain)

	if nodeDrain.Spec.NodeName != "" {
		pendingList, _ := drainer.GetPodsForDeletion(nodeDrain.Spec.NodeName)
		if pendingList != nil {
			nodeDrain.Status.PendingPods = GetPodNameList(pendingList.Pods())
			if nodeDrain.Status.EvictionPodCount != 0 {
				nodeDrain.Status.DrainProgress = (nodeDrain.Status.EvictionPodCount - len(nodeDrain.Status.PendingPods)) * 100 / nodeDrain.Status.EvictionPodCount
			}
		}
	}
	updateErr := r.Client.Status().Update(ctx, nodeDrain)
	if updateErr != nil {
		r.logger.Error(updateErr, "Failed to update NodeDrain with \"Failed\" status")
	}
	if nodeDrain.Spec.NodeName != "" && err.Error() == fmt.Sprintf(expectedNodeNotFoundErrorMsg, nodeDrain.Spec.NodeName) {
		// don't return an error in case of a missing node, as it won't be found in the future.
		return ctrl.Result{}, nil
	}
	if duration != nil {
		r.logger.Info(FixedDurationReconcileLog)
		return ctrl.Result{RequeueAfter: *duration}, nil
	}
	r.logger.Info("Reconciling with exponential duration")
	return ctrl.Result{}, err
}

func (r *NodeDrainReconciler) onReconcileError(ctx context.Context, drainer *drain.Helper, nodeDrain *gezbcoukalphav1.NodeDrain, err error) (ctrl.Result, error) {
	return r.onReconcileErrorWithRequeue(ctx, drainer, nodeDrain, err, nil)
}

func setLastUpdate(nodeDrain *gezbcoukalphav1.NodeDrain) {
	nodeDrain.Status.LastUpdate.Time = time.Now()
}

func setStatus(recorder events.Recorder, nodeDrain *gezbcoukalphav1.NodeDrain, phase gezbcoukalphav1.NodeDrainPhase) {
	setLastUpdate(nodeDrain)
	nodeDrain.Status.Phase = phase
	message := fmt.Sprintf("Status updated to %s", phase)
	publishEvent(recorder, nodeDrain, corev1.EventTypeNormal, events.EventReasonStatusUpdated, message)
}

func publishEvent(recorder events.Recorder, nodeDrain *gezbcoukalphav1.NodeDrain, eventType string, reason string, message string) {
	recorder.Publish(events.Event{
		InvolvedObject: nodeDrain,
		Type:           eventType,
		Reason:         reason,
		Message:        message,
		DedupeValues:   []string{string(nodeDrain.UID)},
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeDrainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gezbcoukalphav1.NodeDrain{}).
		Named("nodedrain").
		Complete(r)
}

// createDrainer creates a drain.Helper struct for external cordon and drain API
func createDrainer(ctx context.Context, mgrConfig *rest.Config) (*drain.Helper, error) {
	drainer := &drain.Helper{}

	// Continue even if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet.
	drainer.Force = true

	// Continue even if there are pods using emptyDir (local data that will be deleted when the node is drained).
	// This is necessary for removing any pod that utilizes an emptyDir volume.
	// The VirtualMachineInstance Pod does use emptryDir volumes,
	// however the data in those volumes are ephemeral which means it is safe to delete after termination.
	drainer.DeleteEmptyDirData = true

	// Ignore DaemonSet-managed pods.
	// This is required because every node running a VirtualMachineInstance will also be running our helper DaemonSet called virt-handler.
	// This flag indicates that it is safe to proceed with the eviction and to just ignore DaemonSets.
	drainer.IgnoreAllDaemonSets = true

	// Period of time in seconds given to each pod to terminate gracefully. If negative, the default value specified in the pod will be used.
	drainer.GracePeriodSeconds = -1

	// The length of time to wait before giving up, zero means infinite
	drainer.Timeout = DrainerTimeout

	cs, err := kubernetes.NewForConfig(mgrConfig)
	if err != nil {
		return nil, err
	}
	drainer.Client = cs
	drainer.DryRunStrategy = util.DryRunNone
	drainer.Ctx = ctx

	drainer.Out = writer{klog.Info}
	drainer.ErrOut = writer{klog.Error}

	// OnPodDeletedOrEvicted function is called when a pod is evicted/deleted; for printing progress output
	drainer.OnPodDeletedOrEvicted = func(pod *corev1.Pod, usingEviction bool) {
		var verbString string
		if usingEviction {
			verbString = "Evicted"
		} else {
			verbString = "Deleted"
		}
		msg := fmt.Sprintf("pod: %s:%s %s from node: %s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name, verbString, pod.Spec.NodeName)
		klog.Info(msg)
	}
	return drainer, nil
}

// GetPodNameList returns a list of pod names from a pod list
func GetPodNameList(pods []corev1.Pod) (result []string) {
	for _, pod := range pods {
		result = append(result, pod.ObjectMeta.Name)
	}
	return result
}

// GetNameSpaceAndName returns a list of pod ObjectKeys(name and namespace) from a pod list
func GetNameSpaceAndName(pods []corev1.Pod) (result []gezbcoukalphav1.NamespaceAndName) {
	for _, pod := range pods {
		result = append(result, gezbcoukalphav1.NamespaceAndName{
			Name:      pod.ObjectMeta.Name,
			Namespace: pod.ObjectMeta.Namespace,
		})
	}
	return result
}

// writer implements io.Writer interface as a pass-through for klog.
type writer struct {
	logFunc func(args ...interface{})
}

// Write passes string(p) into writer's logFunc and always returns len(p)
func (w writer) Write(p []byte) (n int, err error) {
	w.logFunc(string(p))
	return len(p), nil
}
