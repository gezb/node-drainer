package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gezb/node-drainer/test/utils"
)

// namespace where the project is deployed in
const namespace = "node-drainer-system"

// serviceAccountName created for the project
const serviceAccountName = "node-drainer-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "node-drainer-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "node-drainer-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * 2 * time.Minute)
	SetDefaultEventuallyPollingInterval(5 * time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {

			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=node-drainer-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("validating that the ServiceMonitor for Prometheus is applied in the namespace")
			cmd = exec.Command("kubectl", "get", "ServiceMonitor", "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "ServiceMonitor should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccount": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*2*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))

			By("deleting the curl-metrics pod")
			cmd = exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete curl-metrics pod")

		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		It("should drain a node if there are no drianchecks", func() {

			By("Un-cordoned our test node")
			utils.UncordonNode(worker2Node)

			By("Verifying the number of pods on the node(s) is 0")
			utils.ExpectNumberOfPodsRunning(testNodes, 0)

			utils.CreateStatefulSetWithName("nginx", 1, k8sRole)
			utils.CreateStatefulSetWithName("blocking", 2, k8sRole)

			By("Waiting for all pods to be running")

			utils.ExpectNumberOfPodsRunning(testNodes, 3)

			preDrainPodStartTimes := utils.GetPodStartTimesOnNodes(testNodes)

			utils.ApplyNodeDrain(worker2Node, false, K8sVersionRegex, k8sRole)

			By("Drain should run and there should be no pods left")
			utils.ExpectNumberOfPodsRunning(testNodes, 0)

			By("Un-cordoned other test node to allow pods to restart there (simulate new node)")
			utils.UncordonNode(worker3Node)

			By("Waiting for all pods to be restarted on other nodes")
			utils.ExpectNumberOfPodsRunning(testNodes, 3)

			postDrainPodEndTimes := utils.GetPodStartTimesOnNodes(testNodes)

			for podName, preDrainStartTime := range preDrainPodStartTimes {
				newStartTime, exists := postDrainPodEndTimes[podName]
				Expect(exists).To(BeTrue(), "Pod %s not found after drain", podName)
				Expect(newStartTime.After(preDrainStartTime)).To(BeTrue())
			}

			nodeDrainIsPhaseCompleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "nodedrain", worker2Node)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(fmt.Sprintf("%s   %s", worker2Node, "Completed")))

			}
			Eventually(nodeDrainIsPhaseCompleted, 2*time.Minute).Should(Succeed())

			By("Clean up after the test")

			By("Deleting statefulsets")
			utils.DeleteStatefulSet("nginx")
			utils.DeleteStatefulSet("blocking")

			By("deleting the NodeDrain")
			utils.DeleteNodeDrain(worker2Node)

			By("Re-cordoned our test nodes")
			utils.CordonNode(worker2Node)
			utils.CordonNode(worker3Node)
		})
	})

	It("drain should be blocked by a DrainCheck until pods matching the regex are deleted", func() {

		By("Un-cordoned our test node")
		utils.UncordonNode(worker2Node)

		By("Verifying the number of pods on the node(s) is 2")

		utils.ExpectNumberOfPodsRunning(testNodes, 0)

		utils.CreateStatefulSetWithName("nginx", 1, k8sRole)
		utils.CreateStatefulSetWithName("blocking", 2, k8sRole)

		By("Waiting for all pods to be running")
		utils.ExpectNumberOfPodsRunning(testNodes, 3)

		By("Creating a DrainCheck for `blocking-.` pods")

		utils.ApplyDrainCheck("blocking", "^blocking-.*")

		preDrainPodStartTimes := utils.GetPodStartTimesOnNodes(testNodes)

		utils.ApplyNodeDrain(worker2Node, false, K8sVersionRegex, k8sRole)

		By("Checking all pods are still running")
		utils.ExpectNumberOfPodsRunning(testNodes, 3)

		nodedrainIsPhaseCordened := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", worker2Node)
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring(fmt.Sprintf("%s   %s", worker2Node, "PodsBlockingDrain")))
		}
		Eventually(nodedrainIsPhaseCordened, 2*time.Minute).Should(Succeed())

		By("Deleting the first blocking pod the drain should be held")
		utils.DeletePod("blocking-0")

		By("Checking 2 pods are running on our test nodes")
		utils.ExpectNumberOfPodsRunning(testNodes, 2)

		Eventually(nodedrainIsPhaseCordened, 2*time.Minute).Should(Succeed())

		By("Deleting the final blocking pod to allow the drain to continue")
		utils.DeletePod("blocking-1")

		By("Drain should run and there should be left with no pods running")
		utils.ExpectNumberOfPodsRunning(testNodes, 0)

		nodeDrainIsPhaseCompleted := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", worker2Node)
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring(fmt.Sprintf("%s   %s", worker2Node, "Completed")))
		}
		Eventually(nodeDrainIsPhaseCompleted, 2*time.Minute).Should(Succeed())

		By("Un-cordoned other test node to allow pods to restart there (simulate new node)")
		utils.UncordonNode(worker3Node)

		By("Waiting for all pods to be restarted on other nodes")
		utils.ExpectNumberOfPodsRunning(testNodes, 3)

		havePodsRestarted(preDrainPodStartTimes)

		By("Clean up after the test")

		By("Deleting statefulsets")
		utils.DeleteStatefulSet("nginx")
		utils.DeleteStatefulSet("blocking")

		By("deleting the NodeDrain")
		utils.DeleteNodeDrain(worker2Node)

		By("removing the drainCheck")
		utils.DeleteDrainCheck("blocking")

		By("re-cordoned our test node")
		utils.CordonNode(worker2Node)
		utils.CordonNode(worker3Node)
	})

	It("if configured will wait for evicted pods to get to running state", func() {

		By("Un-cordoned our test nodes")
		utils.UncordonNode(worker2Node)

		By("Verifying the number of pods on the node(s) is 0")

		utils.ExpectNumberOfPodsRunning(testNodes, 0)

		utils.CreateStatefulSetWithName("nginx", 1, k8sRole)
		utils.CreateStatefulSetWithName("nginx2", 1, k8sRole)

		By("Waiting for all pods to be running")
		utils.ExpectNumberOfPodsRunning(testNodes, 2)

		preDrainPodStartTimes := utils.GetPodStartTimesOnNodes(testNodes)

		utils.ApplyNodeDrain(worker2Node, true, K8sVersionRegex, k8sRole)

		By("Drain should run and there should be left with no pods running")
		utils.ExpectNumberOfPodsRunning(testNodes, 0)

		nodeDrainIsPhaseWaitForPodsToRestart := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", worker2Node)
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring(fmt.Sprintf("%s   %s", worker2Node, "WaitForPodsToRestart")))
		}
		Eventually(nodeDrainIsPhaseWaitForPodsToRestart, 2*time.Minute).Should(Succeed())

		By("Un-cordoning node (simulate new node) Pods should start and drain should go to completed")

		utils.UncordonNode(worker2Node)

		nodeDrainIsPhaseCompleted := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", worker2Node)
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring(fmt.Sprintf("%s   %s", worker2Node, "WaitForPodsToRestart")))
		}
		Eventually(nodeDrainIsPhaseCompleted, 2*time.Minute).Should(Succeed())

		By("Un-cordoned other test node to allow pods to restart there (simulate new node)")
		utils.UncordonNode(worker3Node)

		By("Waiting for all pods to be restarted on other nodes")
		utils.ExpectNumberOfPodsRunning(testNodes, 2)

		havePodsRestarted(preDrainPodStartTimes)

		By("Clean up after the test")

		By("Deleting statefulsets")
		utils.DeleteStatefulSet("nginx")
		utils.DeleteStatefulSet("nginx2")

		By("deleting the NodeDrain")
		utils.DeleteNodeDrain(worker2Node)

		By("re-cordoned our test node")
		utils.CordonNode(worker2Node)
		utils.CordonNode(worker3Node)
	})

	It("if NodeDrain Role does not match the nodes role the drain will fail", func() {

		By("Un-cordoned our test nodes")
		utils.UncordonNode(worker2Node)

		utils.ApplyNodeDrain(worker2Node, true, K8sVersionRegex, "wrong-role")

		nodeDrainIsPhaseFailed := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", worker2Node)
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring(fmt.Sprintf("%s   %s", worker2Node, "Failed")))
		}
		Eventually(nodeDrainIsPhaseFailed, 2*time.Minute).Should(Succeed())

		By("Clean up after the test")

		By("deleting the NodeDrain")
		utils.DeleteNodeDrain(worker2Node)

		By("Re-cordoned our test nodes")
		utils.CordonNode(worker2Node)
	})

	It("if NodeDrain K8S Version regexp does not match the nodes kubelet version the drain will fail", func() {

		By("Un-cordoned our test nodes")
		utils.UncordonNode(worker2Node)

		utils.ApplyNodeDrain(worker2Node, true, "^v1\\.30\\..*$", k8sRole)

		nodeDrainIsPhaseFailed := nodeDrainInPhaseFunc(worker2Node, "Failed")
		Eventually(nodeDrainIsPhaseFailed, 2*time.Minute).Should(Succeed())

		By("Clean up after the test")

		By("deleting the NodeDrain")
		utils.DeleteNodeDrain(worker2Node)

		By("Re-cordoned our test nodes")
		utils.CordonNode(worker2Node)
	})

	It("multi node with multiple drianchecks", func() {

		By("Un-cordoned our test node(s)")
		utils.UncordonNode(worker2Node)
		utils.UncordonNode(worker3Node)

		By("Starting some test workload across both nodes")

		utils.ExpectNumberOfPodsRunning(testNodes, 0)
		utils.CreateStatefulSetWithName("nginx", 4, k8sRole)
		utils.CreateStatefulSetWithName("test-app-1", 2, k8sRole)
		utils.CreateStatefulSetWithName("test-app-2", 2, k8sRole)
		utils.CreateStatefulSetWithName("vault", 3, k8sRole)
		utils.CreateStatefulSetWithName("elasticsearch-data", 3, k8sRole)

		totalPods := 14

		By("Verifying the number of pods on the node(s) is totalPods")
		utils.ExpectNumberOfPodsRunning(testNodes, totalPods)

		preDrainPodStartTimes := utils.GetPodStartTimesOnNodes(testNodes)

		By("Creating a DrainCheck(s)")

		utils.ApplyDrainCheck("vault", "^vault-.*")
		utils.ApplyDrainCheck("elasticsearch-data", "^elasticsearch-data-.*")

		utils.ApplyNodeDrain(worker2Node, true, K8sVersionRegex, k8sRole)

		By("Checking all pods are still running")
		utils.ExpectNumberOfPodsRunning(testNodes, totalPods)

		By("Verifying both nodedrains are in PodsBlockingDrain phase")
		worker2IsPhasePodsBlockingDrain := nodeDrainInPhaseFunc(worker2Node, "PodsBlockingDrain")
		Eventually(worker2IsPhasePodsBlockingDrain, 2*time.Minute).Should(Succeed())

		By("Applying a NodeDrain to the other test node")
		utils.ApplyNodeDrain(worker3Node, true, K8sVersionRegex, k8sRole)

		By("Verifying both nodedrains are in PodsBlockingDrain phase")
		Eventually(worker2IsPhasePodsBlockingDrain, 2*time.Minute).Should(Succeed())

		worker3IsPhasePodsBlockingDrain := nodeDrainInPhaseFunc(worker3Node, "PodsBlockingDrain")
		Eventually(worker3IsPhasePodsBlockingDrain, 2*time.Minute).Should(Succeed())

		// validate all pods are still running
		utils.ExpectNumberOfPodsRunning(testNodes, totalPods)

		By("Removing all blocking pods on worker 2 allows it move on and drain the node")

		blockingPods := []string{"vault-0", "vault-1", "vault-2", "elasticsearch-data-0", "elasticsearch-data-1", "elasticsearch-data-2"}
		podsToBeDeleted := []string{}
		podsOnNode := utils.GetPodsOnNode(worker2Node)

		for _, blockingPod := range blockingPods {
			for _, pod := range podsOnNode {
				if pod == blockingPod {
					podsToBeDeleted = append(podsToBeDeleted, pod)
				}
			}
		}

		Expect(len(podsOnNode)-len(podsToBeDeleted)).NotTo(BeZero(), "We need non blocking pods on the node")

		for _, pod := range podsToBeDeleted {
			// Phase should still be  Pods Blocking Drain on both nodes
			Eventually(worker2IsPhasePodsBlockingDrain, 2*time.Minute).Should(Succeed())
			Eventually(worker3IsPhasePodsBlockingDrain, 2*time.Minute).Should(Succeed())
			utils.DeletePod(pod)
		}

		// We should now be running totalPods - however many pods were on worker 2
		utils.ExpectNumberOfPodsRunning(testNodes, totalPods-len(podsOnNode))
		utils.ExpectNumberOfPodsRunning([]string{worker2Node}, 0)
		utils.ExpectNumberOfPodsRunning([]string{worker3Node}, totalPods-len(podsOnNode))

		// worker 3 should still be Phase PodsBlocking Drain
		Eventually(worker3IsPhasePodsBlockingDrain, 2*time.Minute).Should(Succeed())

		// Worker 2 should be Phase WaitForPodsToRestart (they cant start atm as all nodes are cordoned)
		worker2IsPhaseWaitForPodsToRestart := nodeDrainInPhaseFunc(worker2Node, "WaitForPodsToRestart")
		Eventually(worker2IsPhaseWaitForPodsToRestart, 2*time.Minute).Should(Succeed())

		// Un-cordon worker 4 so pods can restart
		utils.UncordonNode(worker4Node)

		worker2IsPhaseComplete := nodeDrainInPhaseFunc(worker2Node, "Complete")
		Eventually(worker2IsPhaseComplete, 2*time.Minute).Should(Succeed())

		// validate all pods are now running
		utils.ExpectNumberOfPodsRunning(testNodes, totalPods)

		By("Removing all blocking pods on worker 3 allows it move on and drain the node")

		podsToBeDeleted = []string{}
		podsOnNode = utils.GetPodsOnNode(worker3Node)

		for _, blockingPod := range blockingPods {
			for _, pod := range podsOnNode {
				if pod == blockingPod {
					podsToBeDeleted = append(podsToBeDeleted, pod)
				}
			}
		}

		Expect(len(podsOnNode)-len(podsToBeDeleted)).NotTo(BeZero(), "We need non blocking pods on the node")

		for _, pod := range podsToBeDeleted {
			// Phase should still be  Pods Blocking Drain on both nodes
			Eventually(worker2IsPhaseComplete, 2*time.Minute).Should(Succeed())
			Eventually(worker3IsPhasePodsBlockingDrain, 2*time.Minute).Should(Succeed())
			utils.DeletePod(pod)
		}

		worker3IsPhaseComplete := nodeDrainInPhaseFunc(worker3Node, "Complete")
		Eventually(worker2IsPhaseComplete, 2*time.Minute).Should(Succeed())
		Eventually(worker3IsPhaseComplete, 2*time.Minute).Should(Succeed())

		// validate all pods are now running
		utils.ExpectNumberOfPodsRunning(testNodes, totalPods)

		Eventually(worker2IsPhaseComplete, 2*time.Minute).Should(Succeed())
		Eventually(worker3IsPhaseComplete, 2*time.Minute).Should(Succeed())

		// We should now be running totalPods - however many pods were on worker 3
		utils.ExpectNumberOfPodsRunning(testNodes, totalPods)
		utils.ExpectNumberOfPodsRunning([]string{worker2Node}, 0)
		utils.ExpectNumberOfPodsRunning([]string{worker3Node}, 0)

		// Check all pods have restarted
		havePodsRestarted(preDrainPodStartTimes)

		By("Clean up after the test")

		By("Deleting statefulset(s)")

		utils.DeleteStatefulSet("nginx")
		utils.DeleteStatefulSet("test-app-1")
		utils.DeleteStatefulSet("test-app-2")
		utils.DeleteStatefulSet("vault")
		utils.DeleteStatefulSet("elasticsearch-data")

		By("deleting the NodeDrain(s)")
		utils.DeleteNodeDrain(worker2Node)
		utils.DeleteNodeDrain(worker3Node)

		By("removing the drainCheck(s)")
		utils.DeleteDrainCheck("vault")
		utils.DeleteDrainCheck("elasticsearch-data")

		By("re-cordoned our test node")
		utils.CordonNode(worker2Node)
		utils.CordonNode(worker3Node)
		utils.CordonNode(worker4Node)
	})

})

func nodeDrainInPhaseFunc(node string, phase string) func(g Gomega) {
	return func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "nodedrain", node)
		output, err := utils.Run(cmd)
		g.ExpectWithOffset(1, err).NotTo(HaveOccurred())
		g.ExpectWithOffset(1, output).To(ContainSubstring(fmt.Sprintf("%s   %s", node, phase)))
	}
}

// havePodsRestarted checks that all pods in preDrainPodStartTimes have restarted
func havePodsRestarted(preDrainPodStartTimes map[string]time.Time) {
	postDrainPodEndTimes := utils.GetPodStartTimesOnNodes(testNodes)

	for podName, preDrainStartTime := range preDrainPodStartTimes {
		newStartTime, exists := postDrainPodEndTimes[podName]
		ExpectWithOffset(1, exists).To(BeTrue(), "Pod %s not found after drain", podName)
		ExpectWithOffset(1, newStartTime.After(preDrainStartTime)).To(BeTrue(), fmt.Sprintf("Pod %s has not been restarted", podName))
	}
}

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
