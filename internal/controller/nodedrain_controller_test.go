package controller

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gezbcoukalphav1 "github.com/gezb/node-drainer/api/v1"
)

const (
	nodeName      = "node1"
	testNamespace = "test-namespace"

	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

var _ = Describe("Node Drain", func() {

	BeforeEach(func() {
		// create testNamespace ns
		testNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testNs), &corev1.Namespace{}); err != nil {
			Expect(k8sClient.Create(ctx, testNs)).To(Succeed())
		}
		DeferCleanup(clearEvents)
	})
	var nd *gezbcoukalphav1.NodeDrain
	When("NodeDrain CR for a node that is empty", func() {
		BeforeEach(func() {
			node := getTestNode("node1", false)
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, node)

			nd = getTestNodeDrain()
			Expect(k8sClient.Create(ctx, nd)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, nd)
		})
		It("should goto a completed state", func() {
			nodeDrain := &gezbcoukalphav1.NodeDrain{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			}, timeout, interval).Should(Succeed())
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

			verifyEvent(corev1.EventTypeNormal, EventReasonNodeCordoned, "Node node1 cordoned")
			verifyEvent(corev1.EventTypeNormal, EventReasonNodeDraining, "Node node1 draining")

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(BeEmpty())
			Expect(nodeDrain.Status.TotalPods).To(Equal(0))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		})
	})

	When("NodeDrain CR for a node that is empty, already cordened", func() {
		BeforeEach(func() {
			node := getTestNode("node1", true)
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, node)

			nd = getTestNodeDrain()
			Expect(k8sClient.Create(ctx, nd)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, nd)
		})
		It("should goto a completed state", func() {
			nodeDrain := &gezbcoukalphav1.NodeDrain{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			}, timeout, interval).Should(Succeed())
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

			verifyNoEvent(corev1.EventTypeNormal, EventReasonNodeCordoned, "Node node1 cordoned")
			verifyEvent(corev1.EventTypeNormal, EventReasonNodeDraining, "Node node1 draining")

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(BeEmpty())
			Expect(nodeDrain.Status.TotalPods).To(Equal(0))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		})
	})

	When("NodeDrain CR for a node that is not empty", func() {
		BeforeEach(func() {
			node := getTestNode("node1", false)
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, node)

			pod1 := getTestPod("pod1")
			Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
			pod2 := getTestPod("pod2")
			Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
			pod3 := getTestPod("pod3")
			Expect(k8sClient.Create(ctx, pod3)).To(Succeed())
			// this will be done by the drain
			// DeferCleanup(k8sClient.Delete, ctx, pod1)

			nd = getTestNodeDrain()
			Expect(k8sClient.Create(ctx, nd)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, nd)
		})
		It("should goto a completed state", func() {
			nodeDrain := &gezbcoukalphav1.NodeDrain{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			}, timeout, interval).Should(Succeed())
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

			verifyEvent(corev1.EventTypeNormal, EventReasonNodeCordoned, "Node node1 cordoned")
			verifyEvent(corev1.EventTypeNormal, EventReasonNodeDraining, "Node node1 draining")

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(BeEmpty())
			Expect(nodeDrain.Status.TotalPods).To(Equal(0))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		})
	})

	When("Drain gets blocked by a draincheck", func() {
		var blockpod *corev1.Pod
		BeforeEach(func() {
			node := getTestNode("node1", false)
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, node)

			pod1 := getTestPod("pod1")
			Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
			pod2 := getTestPod("shouldnotblock-1")
			Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
			blockpod = getTestPod("block-1")
			Expect(k8sClient.Create(ctx, blockpod)).To(Succeed())
			// this will be done by the drain
			// DeferCleanup(k8sClient.Delete, ctx, pod1)
			// DeferCleanup(k8sClient.Delete, ctx, pod2)
			// DeferCleanup(k8sClient.Delete, ctx, drainPod1)

			drainCheck := getTestDrainCheck()
			Expect(k8sClient.Create(ctx, drainCheck)).To(Succeed())
			// this will be done later in the test to complete the drain
			DeferCleanup(k8sClient.Delete, ctx, drainCheck)

			nd = getTestNodeDrain()
			Expect(k8sClient.Create(ctx, nd)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, nd)
		})
		It("should goto a completed state once the blocking pod has been deleted", func() {
			By("Drain should be blocked by pods block-1")
			nodeDrain := &gezbcoukalphav1.NodeDrain{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCordoned))
			}, timeout, interval).Should(Succeed())
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)

			verifyEvent(corev1.EventTypeNormal, EventReasonNodeCordoned, "Node node1 cordoned")
			verifyEvent(corev1.EventTypeWarning, EventReasonDrainBlocked, "Node node1 is blocked from draining by pod: block-1")

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(HaveLen(3))
			Expect(nodeDrain.Status.TotalPods).To(Equal(3))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(3))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))

			By("Deleting the blocking pod")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(blockpod), blockpod)).To(Succeed())
			Expect(k8sClient.Delete(ctx, blockpod)).To(Succeed())

			By("The drain should run to completion")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			}, timeout, interval).Should(Succeed())
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

			verifyEvent(corev1.EventTypeNormal, EventReasonNodeDraining, "Node node1 draining")

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(BeEmpty())
			Expect(nodeDrain.Status.TotalPods).To(Equal(0))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		})
	})

	When("Drain does not run if another node for the same role & version hs not been cordened ", func() {
		var node2 *corev1.Node
		BeforeEach(func() {
			node := getTestNode("node1", false)
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, node)
			node2 = getTestNode("uncodened-node", false)
			Expect(k8sClient.Create(ctx, node2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, node2)

			pod1 := getTestPod("pod1")
			Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
			// this will be done by the drain
			// DeferCleanup(k8sClient.Delete, ctx, pod1)

			nd = getTestNodeDrain()
			Expect(k8sClient.Create(ctx, nd)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, nd)
		})
		It("should stop at corendned when another node is not cordened", func() {
			nodeDrain := &gezbcoukalphav1.NodeDrain{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCordoned))
			}, timeout, interval).Should(Succeed())
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)

			verifyEvent(corev1.EventTypeNormal, EventReasonNodeCordoned, "Node node1 cordoned")

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(HaveLen(1))
			Expect(nodeDrain.Status.TotalPods).To(Equal(1))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(1))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))

			By("Codrdening the node")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(node2), node2)).To(Succeed())
			node2.Spec.Unschedulable = true
			Expect(k8sClient.Update(ctx, node2)).To(Succeed())

			By("The drain should run to completion")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCordoned))
			}, timeout, interval).Should(Succeed())

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(HaveLen(1))
			Expect(nodeDrain.Status.TotalPods).To(Equal(1))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(1))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))

			By("Cordoning the node, the drain can complete")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(node2), node2)).To(Succeed())
			node2.Spec.Unschedulable = true
			Expect(k8sClient.Update(ctx, node2)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			}, timeout, interval).Should(Succeed())

			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

			verifyEvent(corev1.EventTypeNormal, EventReasonNodeDraining, "Node node1 draining")

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(BeEmpty())
			Expect(nodeDrain.Status.TotalPods).To(Equal(0))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		})
	})

	When("NodeDrain CR for a node that is not empty", func() {
		BeforeEach(func() {
			node := getTestNode("node1", false)
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, node)

			pod1 := getTestPod("pod1")
			Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
			pod2 := getTestPod("pod2")
			Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
			pod3 := getTestPod("pod3")
			Expect(k8sClient.Create(ctx, pod3)).To(Succeed())
			// this will be done by the drain
			// DeferCleanup(k8sClient.Delete, ctx, pod1)

			nd = getTestNodeDrain()
			Expect(k8sClient.Create(ctx, nd)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, nd)
		})
		It("should goto a completed state", func() {
			nodeDrain := &gezbcoukalphav1.NodeDrain{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), nodeDrain)).To(Succeed())
				g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			}, timeout, interval).Should(Succeed())
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

			verifyEvent(corev1.EventTypeNormal, EventReasonNodeCordoned, "Node node1 cordoned")
			verifyEvent(corev1.EventTypeNormal, EventReasonNodeDraining, "Node node1 draining")

			Expect(nodeDrain.Status.LastError).To(Equal(""))
			Expect(nodeDrain.Status.PendingPods).To(BeEmpty())
			Expect(nodeDrain.Status.TotalPods).To(Equal(0))
			Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
			Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		})
	})

	When("NodeDrain CR for a node that does not exist", func() {
		BeforeEach(func() {
			nd = getTestNodeDrain()
			Expect(k8sClient.Create(ctx, nd)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, nd)
		})
		It("should goto a failed state as the node does not exist", func() {
			By("check nodeDrain CR status was Failed")
			maintenance := &gezbcoukalphav1.NodeDrain{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nd), maintenance)).To(Succeed())
				g.Expect(maintenance.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseFailed))
			}, timeout, interval).Should(Succeed())
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
			verifyEvent(corev1.EventTypeWarning, EventReasonNodeNoFound, "Node: node1 not found")
			verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseFailed)
		})
	})
})

func getTestPod(podName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: testNamespace,
		},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{
				{
					Name:  "c1",
					Image: "i1",
				},
			},
			TerminationGracePeriodSeconds: ptr.To[int64](0),
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

func getTestNodeDrain() *gezbcoukalphav1.NodeDrain {
	return &gezbcoukalphav1.NodeDrain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-drain",
			Namespace: testNamespace,
		},
		Spec: gezbcoukalphav1.NodeDrainSpec{
			NodeName: "node1",
		},
	}
}

func getTestDrainCheck() *gezbcoukalphav1.DrainCheck {
	return &gezbcoukalphav1.DrainCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drain-check",
			Namespace: testNamespace,
		},
		Spec: gezbcoukalphav1.DrainCheckSpec{
			PodRegex: "^block-.+",
		},
	}
}

func getTestNode(nodeName string, unschedulable bool) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				"role": "test-role",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: unschedulable,
		},
	}
}

func verifyEvent(eventType, eventReason, eventMessage string) {
	By(fmt.Sprintf("Verifying that event %s with message %s was created", eventReason, eventMessage))
	isEventMatch := isEventOccurred(eventType, eventReason, eventMessage)
	ExpectWithOffset(1, isEventMatch).To(BeTrue())
}

func verifyNoEvent(eventType, eventReason, eventMessage string) {
	By(fmt.Sprintf("Verifying that event %s was not created", eventReason))
	isEventMatch := isEventOccurred(eventType, eventReason, eventMessage)
	ExpectWithOffset(1, isEventMatch).To(BeFalse())
}

// isEventOccurred checks whether an event has occoured
func isEventOccurred(eventType, eventReason, eventMessage string) bool {
	expected := fmt.Sprintf("%s %s %s", eventType, eventReason, eventMessage)
	isEventMatch := false

	unMatchedEvents := make(chan string, len(fakeRecorder.Events))
	isDone := false
	for {
		select {
		case event := <-fakeRecorder.Events:
			if isEventMatch = event == expected; isEventMatch {
				isDone = true
			} else {
				unMatchedEvents <- event
			}
		default:
			isDone = true
		}
		if isDone {
			break
		}
	}

	close(unMatchedEvents)
	for unMatchedEvent := range unMatchedEvents {
		fakeRecorder.Events <- unMatchedEvent
	}
	return isEventMatch
}

func verifyStatusEvent(status gezbcoukalphav1.NodeDrainPhase) {
	eventType := corev1.EventTypeNormal
	eventReason := EventReasonStatusUpdated
	eventMessage := fmt.Sprintf("Status updated to: %s", status)
	By(fmt.Sprintf("Verifying that event %s with message %s was created", eventReason, eventMessage))
	isEventMatch := isEventOccurred(eventType, eventReason, eventMessage)
	ExpectWithOffset(1, isEventMatch).To(BeTrue())
}

// clearEvents loop over the events channel until it is empty from events
func clearEvents() {
	for len(fakeRecorder.Events) > 0 {
		<-fakeRecorder.Events
	}
}
