package controller

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gezb/node-drainer/internal/events"

	gezbcoukalphav1 "github.com/gezb/node-drainer/api/v1"
)

const (
	testNamespace = "test-namespace"

	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

var _ = Describe("Node Drain", func() {

	BeforeEach(func() {
		// create testNamespace ns
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(namespace), &corev1.Namespace{}); err != nil {
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		}
		ClearEventsCache()
	})

	AfterEach(func() {
		statusChecker = alwaysTruePodStatusChecker{}
	})

	It("NodeDrain CR for a node that is empty", func() {
		var nodeDrain *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		node := getTestNode("node1", false)
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		nodeDrain = getTestNodeDrain("node1", 1, false)
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())
		By("should goto a completed state")
		nodeDrain = &gezbcoukalphav1.NodeDrain{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(1))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(0))
		Expect(nodeDrain.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(100))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())
		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNode("node1")
	})

	It("NodeDrain CR for a node that is empty, already cordened", func() {

		By("Setup for test")
		node := getTestNode("node1", true)
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		nodeDrain := getTestNodeDrain("node1", 1, false)
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())

		By("should goto a completed state")
		nodeDrain = &gezbcoukalphav1.NodeDrain{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(1))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.TotalPods).To(Equal(0))
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(100))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNode("node1")
	})

	It("NodeDrain CR for a node that is not empty", func() {
		var node *corev1.Node
		var nodeDrain *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		node = getTestNode("node1", false)
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		pod1 := getTestPod("pod1")
		Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
		pod2 := getTestPod("pod2")
		Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
		pod3 := getTestPod("pod3")
		Expect(k8sClient.Create(ctx, pod3)).To(Succeed())
		// this will be done by the drain
		// DeferCleanup(k8sClient.Delete, ctx, pod1)

		nodeDrain = getTestNodeDrain("node1", 1, false)
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())

		By("should goto a completed state")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(1))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(3))
		Expect(nodeDrain.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToBeEvicted).To(Equal([]gezbcoukalphav1.NamespaceAndName{
			{
				Namespace: testNamespace,
				Name:      "pod1",
			},
			{
				Namespace: testNamespace,
				Name:      "pod2",
			},
			{
				Namespace: testNamespace,
				Name:      "pod3",
			},
		}))
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(3))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(100))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNode("node1")
	})

	It("Drain gets blocked by a draincheck", func() {
		var blockpod *corev1.Pod
		var blockpod2 *corev1.Pod
		var node *corev1.Node
		var nodeDrain *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		node = getTestNode("node1", false)
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		pod1 := getTestPod("pod1")
		Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
		pod2 := getTestPod("shouldnotblock-1")
		Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
		blockpod = getTestPod("block-1")
		Expect(k8sClient.Create(ctx, blockpod)).To(Succeed())
		blockpod2 = getTestPod("block-2")
		Expect(k8sClient.Create(ctx, blockpod2)).To(Succeed())

		// this will be done by the drain
		// DeferCleanup(k8sClient.Delete, ctx, pod1)
		// DeferCleanup(k8sClient.Delete, ctx, pod2)
		// DeferCleanup(k8sClient.Delete, ctx, drainPod1)

		drainCheck := getTestDrainCheck()
		Expect(k8sClient.Create(ctx, drainCheck)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "draincheck",
			Namespace: testNamespace}, drainCheck)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, drainCheck)

		nodeDrain = getTestNodeDrain("node1", 1, false)
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())

		By("should be blocked by pods block-1 and block-2")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "draincheck", Namespace: testNamespace}, drainCheck)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhasePodsBlocking))
		}, timeout, interval).Should(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nodeDrain.Spec.NodeName}, node)).To(Succeed())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePodsBlocking)

		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal("block-1,block-2"))

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(1))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(4))
		Expect(nodeDrain.Status.PodsToBeEvicted).To(BeEmpty())
		Expect(nodeDrain.Status.PendingEvictionPods).To(Equal([]string{"block-1", "block-2", "pod1", "shouldnotblock-1"}))
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(4))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(0))

		By("Deleting the blocking block-1")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(blockpod), blockpod)).To(Succeed())
		Expect(k8sClient.Delete(ctx, blockpod)).To(Succeed())

		By("Check the status has been updated")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.PendingEvictionPods).To(HaveLen(3))
		}, timeout, interval).Should(Succeed())

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(1))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(4))
		Expect(nodeDrain.Status.PodsToBeEvicted).To(BeNil())
		Expect(nodeDrain.Status.PendingEvictionPods).To(Equal([]string{"block-2", "pod1", "shouldnotblock-1"}))
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(4))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(25))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal("block-2"))

		By("Deleting the blocking block-2")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(blockpod2), blockpod2)).To(Succeed())
		Expect(k8sClient.Delete(ctx, blockpod2)).To(Succeed())

		By("The drain should run to completion")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(1))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(4))
		podsToBeEvicted := []gezbcoukalphav1.NamespaceAndName{
			{
				Namespace: testNamespace,
				Name:      "pod1",
			},
			{
				Namespace: testNamespace,
				Name:      "shouldnotblock-1",
			},
		}
		Expect(nodeDrain.Status.PodsToBeEvicted).To(Equal(podsToBeEvicted))
		Expect(nodeDrain.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(4))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(100))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNode("node1")
	})

	It("NodeDrain CR for a node that is not empty", func() {
		var node *corev1.Node
		var nodeDrain *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		node = getTestNode("node1", false)
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		pod1 := getTestPod("pod1")
		Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
		pod2 := getTestPod("pod2")
		Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
		pod3 := getTestPod("pod3")
		Expect(k8sClient.Create(ctx, pod3)).To(Succeed())
		// this will be done by the drain
		// DeferCleanup(k8sClient.Delete, ctx, pod1)
		// DeferCleanup(k8sClient.Delete, ctx, pod2)
		// DeferCleanup(k8sClient.Delete, ctx, pod3)

		nodeDrain = getTestNodeDrain("node1", 1, false)
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())

		By("should goto a completed state")
		nodeDrain = &gezbcoukalphav1.NodeDrain{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(1))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(3))
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(3))
		podsToBeEvicted := []gezbcoukalphav1.NamespaceAndName{
			{
				Namespace: testNamespace,
				Name:      "pod1",
			},
			{
				Namespace: testNamespace,
				Name:      "pod2",
			},
			{
				Namespace: testNamespace,
				Name:      "pod3",
			},
		}
		Expect(nodeDrain.Status.PodsToBeEvicted).To(Equal(podsToBeEvicted))
		Expect(nodeDrain.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.DrainProgress).To(Equal(100))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNode("node1")
	})

	It("NodeDrain CR for a node that does not exist", func() {
		var nodeDrain *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		nodeDrain = getTestNodeDrain("node1", 1, false)
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())

		By("should goto a failed state as the node does not exist")
		nodeDrain = &gezbcoukalphav1.NodeDrain{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseFailed))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyNoStatusEvent(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseFailed)

		verifyEvent(events.EventReasonNodeNotFound, "Node: node1 not found")

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(0))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(0))
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
		Expect(nodeDrain.Status.PodsToBeEvicted).To(BeEmpty())
		Expect(nodeDrain.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
	})

	It("NodeDrain CR for a node that is not empty, waits for pods to restart when requested", func() {
		var node *corev1.Node
		var nodeDrain *gezbcoukalphav1.NodeDrain

		By("Setup for test")
		statusChecker = neverTruePodStatusChecker{}
		node = getTestNode("node1", false)
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		pod1 := getTestPod("pod1")
		Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
		pod2 := getTestPod("pod2")
		Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
		pod3 := getTestPod("pod3")
		Expect(k8sClient.Create(ctx, pod3)).To(Succeed())
		// this will be done by the drain
		// DeferCleanup(k8sClient.Delete, ctx, pod1)
		// DeferCleanup(k8sClient.Delete, ctx, pod2)
		// DeferCleanup(k8sClient.Delete, ctx, pod3)

		nodeDrain = getTestNodeDrain("node1", 1, true)
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())

		By("should goto WaitForPodsToRestartStatus")
		nodeDrain = &gezbcoukalphav1.NodeDrain{}

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseWaitForPodsToRestart))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseWaitForPodsToRestart)

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(1))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(3))
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(3))
		podsToBeEvicted := []gezbcoukalphav1.NamespaceAndName{
			{
				Namespace: testNamespace,
				Name:      "pod1",
			},
			{
				Namespace: testNamespace,
				Name:      "pod2",
			},
			{
				Namespace: testNamespace,
				Name:      "pod3",
			},
		}
		Expect(nodeDrain.Status.PodsToBeEvicted).To(Equal(podsToBeEvicted))
		Expect(nodeDrain.Status.PendingEvictionPods).To(Equal([]string{"pod1", "pod2", "pod3"}))
		Expect(nodeDrain.Status.PodsToRestart).To(Equal(podsToBeEvicted))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		deleteNode("node1")
	})

	It("NodeDrain CR for a node that does not match the role", func() {
		var node *corev1.Node
		var nodeDrain *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		node = getTestNode("node1", false)
		node.Labels["role"] = "not-test-role"
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		nodeDrain = getTestNodeDrain("node1", 1, false)
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())

		By("should goto a failed state")
		nodeDrain = &gezbcoukalphav1.NodeDrain{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseFailed))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyNoStatusEvent(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseFailed)

		verifyEvent(events.EventReasonNodeNotFound, "Node role mismatch: expected test-role got not-test-role")

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(0))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(0))
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToBeEvicted).To(BeEmpty())
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		deleteNode("node1")
	})

	It("NodeDrain CR for a node that has version does not match the regex", func() {
		var node *corev1.Node
		var nodeDrain *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		node = getTestNode("node1", false)
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		pod1 := getTestPod("pod1")
		Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
		pod2 := getTestPod("pod2")
		Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
		pod3 := getTestPod("pod3")
		Expect(k8sClient.Create(ctx, pod3)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, pod1)
		DeferCleanup(k8sClient.Delete, ctx, pod2)
		DeferCleanup(k8sClient.Delete, ctx, pod3)

		nodeDrain = getTestNodeDrain("node1", 1, false)
		nodeDrain.Spec.VersionToDrainRegex = "^1.34.*$"
		Expect(k8sClient.Create(ctx, nodeDrain)).To(Succeed())

		By("should goto a failed state")
		nodeDrain = &gezbcoukalphav1.NodeDrain{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain)).To(Succeed())
			g.Expect(nodeDrain.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseFailed))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyNoStatusEvent(gezbcoukalphav1.NodeDrainPhaseWaitForPodsToRestart)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseFailed)

		verifyEvent(events.EventReasonNodeNotFound, "Node version mismatch: expected a value that satisfies ^1.34.*$ got 1.35.5-eks")

		Expect(nodeDrain.Status.NodeDrainCount).To(Equal(0))
		Expect(nodeDrain.Status.LastError).To(Equal(""))
		Expect(nodeDrain.Status.TotalPods).To(Equal(0))
		Expect(nodeDrain.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToBeEvicted).To(BeEmpty())
		Expect(nodeDrain.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain.Status.EvictionPodCount).To(Equal(0))
		Expect(nodeDrain.Status.DrainProgress).To(Equal(0))
		Expect(nodeDrain.Status.PodsBlockingDrain).To(Equal(""))
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		deleteNode("node1")
	})

	It("waits for other nodes before the drain starts", func() {
		var blockpod *corev1.Pod
		var blockpod2 *corev1.Pod
		var node1 *corev1.Node
		var nodeDrain1 *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		node1 = getTestNode("node1", false)
		Expect(k8sClient.Create(ctx, node1)).To(Succeed())

		pod1 := getTestPod("pod1")
		Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
		pod2 := getTestPod("shouldnotblock-1")
		Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
		blockpod = getTestPod("block-1")
		Expect(k8sClient.Create(ctx, blockpod)).To(Succeed())
		blockpod2 = getTestPod("block-2")
		Expect(k8sClient.Create(ctx, blockpod2)).To(Succeed())

		// this will be done by the drain
		// DeferCleanup(k8sClient.Delete, ctx, pod1)
		// DeferCleanup(k8sClient.Delete, ctx, pod2)
		// DeferCleanup(k8sClient.Delete, ctx, drainPod1)

		drainCheck := getTestDrainCheck()
		Expect(k8sClient.Create(ctx, drainCheck)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "draincheck",
			Namespace: testNamespace}, drainCheck)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, drainCheck)

		nodeDrain1 = getTestNodeDrain("node1", 3, false)
		Expect(k8sClient.Create(ctx, nodeDrain1)).To(Succeed())

		By("Should wait for 3 nodeDrains before starting")

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
			g.Expect(nodeDrain1.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes))
		}, timeout, interval).Should(Succeed())

		node2 := getTestNode("node2", false)
		Expect(k8sClient.Create(ctx, node2)).To(Succeed())
		node3 := getTestNode("node3", false)
		Expect(k8sClient.Create(ctx, node3)).To(Succeed())

		// second node
		nodeDrain2 := getTestNodeDrain("node2", 3, false)
		Expect(k8sClient.Create(ctx, nodeDrain2)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
			g.Expect(nodeDrain1.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes))
			g.Expect(nodeDrain1.Status.NodeDrainCount).To(Equal(2))
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node2", Namespace: testNamespace}, nodeDrain2)).To(Succeed())
			g.Expect(nodeDrain2.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes))
			g.Expect(nodeDrain2.Status.NodeDrainCount).To(Equal(2))
		}, timeout, interval).Should(Succeed())

		// third node
		nodeDrain3 := getTestNodeDrain("node3", 3, false)
		Expect(k8sClient.Create(ctx, nodeDrain3)).To(Succeed())

		By("drain of node1  should be blocked by pods block-1 and block-2")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
			g.Expect(nodeDrain1.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhasePodsBlocking))
			g.Expect(nodeDrain1.Status.NodeDrainCount).To(Equal(3))
			node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePending)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCordoned)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhasePodsBlocking)

		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
		Expect(nodeDrain1.Status.PodsBlockingDrain).To(Equal("block-1,block-2"))

		Expect(nodeDrain1.Status.NodeDrainCount).To(Equal(3))
		Expect(nodeDrain1.Status.LastError).To(Equal(""))
		Expect(nodeDrain1.Status.TotalPods).To(Equal(4))
		Expect(nodeDrain1.Status.PodsToBeEvicted).To(BeEmpty())
		Expect(nodeDrain1.Status.PendingEvictionPods).To(Equal([]string{"block-1", "block-2", "pod1", "shouldnotblock-1"}))
		Expect(nodeDrain1.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain1.Status.EvictionPodCount).To(Equal(4))
		Expect(nodeDrain1.Status.DrainProgress).To(Equal(0))

		By("Deleting the blocking block-1")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(blockpod), blockpod)).To(Succeed())
		Expect(k8sClient.Delete(ctx, blockpod)).To(Succeed())

		By("Check the status has been updated")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
			g.Expect(nodeDrain1.Status.PendingEvictionPods).To(HaveLen(3))
		}, timeout, interval).Should(Succeed())

		Expect(nodeDrain1.Status.NodeDrainCount).To(Equal(3))
		Expect(nodeDrain1.Status.LastError).To(Equal(""))
		Expect(nodeDrain1.Status.TotalPods).To(Equal(4))
		Expect(nodeDrain1.Status.PodsToBeEvicted).To(BeNil())
		Expect(nodeDrain1.Status.PendingEvictionPods).To(Equal([]string{"block-2", "pod1", "shouldnotblock-1"}))
		Expect(nodeDrain1.Status.EvictionPodCount).To(Equal(4))
		Expect(nodeDrain1.Status.DrainProgress).To(Equal(25))
		Expect(nodeDrain1.Status.PodsBlockingDrain).To(Equal("block-2"))

		By("Deleting the blocking block-2")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(blockpod2), blockpod2)).To(Succeed())
		Expect(k8sClient.Delete(ctx, blockpod2)).To(Succeed())

		By("The drain should run to completion")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
			g.Expect(nodeDrain1.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
		}, timeout, interval).Should(Succeed())

		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseDraining)
		verifyStatusEvent(gezbcoukalphav1.NodeDrainPhaseCompleted)

		Expect(nodeDrain1.Status.NodeDrainCount).To(Equal(3))
		Expect(nodeDrain1.Status.LastError).To(Equal(""))
		Expect(nodeDrain1.Status.TotalPods).To(Equal(4))
		podsToBeEvicted := []gezbcoukalphav1.NamespaceAndName{
			{
				Namespace: testNamespace,
				Name:      "pod1",
			},
			{
				Namespace: testNamespace,
				Name:      "shouldnotblock-1",
			},
		}
		Expect(nodeDrain1.Status.PodsToBeEvicted).To(Equal(podsToBeEvicted))
		Expect(nodeDrain1.Status.PendingEvictionPods).To(BeEmpty())
		Expect(nodeDrain1.Status.PodsToRestart).To(BeEmpty())
		Expect(nodeDrain1.Status.EvictionPodCount).To(Equal(4))
		Expect(nodeDrain1.Status.DrainProgress).To(Equal(100))
		Expect(nodeDrain1.Status.PodsBlockingDrain).To(Equal(""))
		node1, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node1.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())

		// check the other pods have gone to Complete
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node2", Namespace: testNamespace}, nodeDrain2)).To(Succeed())
			g.Expect(nodeDrain2.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			g.Expect(nodeDrain2.Status.NodeDrainCount).To(Equal(3))
			node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node2", metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node3", Namespace: testNamespace}, nodeDrain3)).To(Succeed())
			g.Expect(nodeDrain3.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			g.Expect(nodeDrain3.Status.NodeDrainCount).To(Equal(3))
			node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node3", metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNodeDrain("node1")
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNodeDrain("node2")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node2", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNodeDrain("node3")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node3", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNode("node1")
		deleteNode("node2")
		deleteNode("node3")
	})

	It("deletion of nodedrain while in progress removes annoations", func() {
		var blockpod *corev1.Pod
		var blockpod2 *corev1.Pod
		var node1 *corev1.Node
		var nodeDrain1 *gezbcoukalphav1.NodeDrain
		By("Setup for test")
		node1 = getTestNode("node1", false)
		Expect(k8sClient.Create(ctx, node1)).To(Succeed())

		pod1 := getTestPod("pod1")
		Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
		pod2 := getTestPod("shouldnotblock-1")
		Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
		blockpod = getTestPod("block-1")
		Expect(k8sClient.Create(ctx, blockpod)).To(Succeed())
		blockpod2 = getTestPod("block-2")
		Expect(k8sClient.Create(ctx, blockpod2)).To(Succeed())

		// this will be done by the drain
		// DeferCleanup(k8sClient.Delete, ctx, pod1)
		// DeferCleanup(k8sClient.Delete, ctx, pod2)
		// DeferCleanup(k8sClient.Delete, ctx, drainPod1)

		drainCheck := getTestDrainCheck()
		Expect(k8sClient.Create(ctx, drainCheck)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "draincheck",
			Namespace: testNamespace}, drainCheck)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, drainCheck)

		nodeDrain1 = getTestNodeDrain("node1", 3, false)
		Expect(k8sClient.Create(ctx, nodeDrain1)).To(Succeed())

		By("Should wait for 3 nodeDrains before starting")

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
			g.Expect(nodeDrain1.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes))
		}, timeout, interval).Should(Succeed())

		node2 := getTestNode("node2", false)
		Expect(k8sClient.Create(ctx, node2)).To(Succeed())
		node3 := getTestNode("node3", false)
		Expect(k8sClient.Create(ctx, node3)).To(Succeed())

		// second node
		nodeDrain2 := getTestNodeDrain("node2", 3, false)
		Expect(k8sClient.Create(ctx, nodeDrain2)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
			g.Expect(nodeDrain1.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes))
			g.Expect(nodeDrain1.Status.NodeDrainCount).To(Equal(2))
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node2", Namespace: testNamespace}, nodeDrain2)).To(Succeed())
			g.Expect(nodeDrain2.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseWaitingForNodes))
			g.Expect(nodeDrain2.Status.NodeDrainCount).To(Equal(2))
		}, timeout, interval).Should(Succeed())

		// third node
		nodeDrain3 := getTestNodeDrain("node3", 3, false)
		Expect(k8sClient.Create(ctx, nodeDrain3)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node1", Namespace: testNamespace}, nodeDrain1)).To(Succeed())
			g.Expect(nodeDrain1.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhasePodsBlocking))
			g.Expect(nodeDrain1.Status.NodeDrainCount).To(Equal(3))
			node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node2", Namespace: testNamespace}, nodeDrain2)).To(Succeed())
			g.Expect(nodeDrain2.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			g.Expect(nodeDrain2.Status.NodeDrainCount).To(Equal(3))
			node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node2", metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "node3", Namespace: testNamespace}, nodeDrain3)).To(Succeed())
			g.Expect(nodeDrain3.Status.Phase).To(Equal(gezbcoukalphav1.NodeDrainPhaseCompleted))
			g.Expect(nodeDrain3.Status.NodeDrainCount).To(Equal(3))
			node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node3", metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		By("deletion of NodeDrains removes the annotations")

		deleteNodeDrain("node1")
		node, err := k8sClientSet.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNodeDrain("node2")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node2", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())
		deleteNodeDrain("node3")
		node, err = k8sClientSet.CoreV1().Nodes().Get(ctx, "node3", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.HasAnnotation(node.ObjectMeta, gezbcoukalphav1.NodeDrainAnnotation)).To(BeFalse())

		By("Cleanup after test")
		// if these are left to deferCleanup() we get timing issues
		// where the node does not exist when the controller comes to delete the annotation
		deleteNode("node1")
		deleteNode("node2")
		deleteNode("node3")
	})
})

func deleteNode(name string) {
	ExpectWithOffset(1, k8sClientSet.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{})).To(Succeed())
	EventuallyWithOffset(1, func(g Gomega) {
		_, err := k8sClientSet.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		g.Expect(apiErrors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func deleteNodeDrain(name string) {
	nodeDrain := &gezbcoukalphav1.NodeDrain{}
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: testNamespace}, nodeDrain)).To(Succeed())
	ExpectWithOffset(1, k8sClient.Delete(ctx, nodeDrain)).To(Succeed())
	EventuallyWithOffset(1, func(g Gomega) {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: testNamespace}, nodeDrain)
		g.Expect(apiErrors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

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

func getTestNodeDrain(name string, expectedNodes int, waitForRestart bool) *gezbcoukalphav1.NodeDrain {
	return &gezbcoukalphav1.NodeDrain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: gezbcoukalphav1.NodeDrainSpec{
			NumberOfNodes:            expectedNodes,
			NodeName:                 name,
			VersionToDrainRegex:      "^1.35.*$",
			NodeRole:                 "test-role",
			SkipWaitForPodsToRestart: !waitForRestart,
		},
	}
}

func getTestDrainCheck() *gezbcoukalphav1.DrainCheck {
	return &gezbcoukalphav1.DrainCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "draincheck",
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
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion: "1.35.5-eks",
			},
		},
	}
}

func verifyStatusEvent(phase gezbcoukalphav1.NodeDrainPhase) {
	message := fmt.Sprintf("Status updated to %s", phase)
	verifyEvent(events.EventReasonStatusUpdated, message)
}

func verifyEvent(reason string, message string) {
	ExpectWithOffset(2, fakeRecorder.DetectedEvent(reason, message)).To(BeTrue())
}

func verifyNoStatusEvent(phase gezbcoukalphav1.NodeDrainPhase) {
	message := fmt.Sprintf("Status updated to %s", phase)
	verifyNoEvent(events.EventReasonStatusUpdated, message)
}

func verifyNoEvent(reason string, message string) {
	ExpectWithOffset(2, fakeRecorder.DetectedEvent(reason, message)).To(BeFalse())
}
