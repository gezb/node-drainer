package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

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
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

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

		It("should drain a node when there are no pods blocking the drain", func() {

			By("Uncordening our test node")
			utils.UncordonNode(worker2Node)

			By("Verifying the number of pods on the node(s) is 4")
			expectNumberOfPodsRunning(4) // two nodes

			createStatefulSetWithName("nginx")
			createStatefulSetWithName("blocking")

			By("Waiting for all pods to be running")

			expectNumberOfPodsRunning(6)

			By("Creating a nodeDrain for our node")
			nodeDrain := fmt.Sprintf(`
apiVersion: k8s.gezb.co.uk/v1
kind: NodeDrain
metadata:
  name: nodedrain-sample
spec:
  nodeName: %s
  ignoreVersion: true
`, worker2Node)
			nodeDrainFile, err := utils.CreateTempFile(nodeDrain)
			Expect(err).NotTo(HaveOccurred(), "Failed apply nodeDrain")
			cmd := exec.Command("kubectl", "apply", "-f", nodeDrainFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed apply nodeDrain")

			By("Waiting for all pods to be removed")
			expectNumberOfPodsRunning(4)

			nodeDrainIsPhaseCompleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "nodedrain", "nodedrain-sample")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("nodedrain-sample   Completed"))
			}
			Eventually(nodeDrainIsPhaseCompleted, 5*time.Minute).Should(Succeed())

			cmd = exec.Command("kubectl", "delete", "-f", nodeDrainFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed delete nodeDrain")
			err = os.Remove(nodeDrainFile)
			Expect(err).NotTo(HaveOccurred(), "Failed remove nodeDrainFile")

			By(" Deleting test statefulsets")
			cmd = exec.Command("kubectl", "delete", "statefulset", "nginx")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed delete nginx statefulset")
			cmd = exec.Command("kubectl", "delete", "statefulset", "blocking")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed delete blocking statefulset")

			By("re-cordening our test node")
			utils.CordonNode(worker2Node)

		})

		It("drain should be blocked by a DrainCheck until pods matching the regex are deleted", func() {

			By("Uncordening our test node")
			utils.UncordonNode(worker2Node)

			By("Verifying the number of pods on the node(s) is 2")

			expectNumberOfPodsRunning(4)

			createStatefulSetWithName("nginx")
			createStatefulSetWithName("blocking")

			By("Waiting for all pods to be running")
			expectNumberOfPodsRunning(6)

			By("creating a DrainCheck for `blocking-.` pods")

			drainCheck := `
apiVersion: k8s.gezb.co.uk/v1
kind: DrainCheck
metadata:
  name: draincheck-sample
  #namespace: %s
spec:
  podRegex: ^blocking-.*
`
			drainCheckFile, err := utils.CreateTempFile(drainCheck)
			Expect(err).NotTo(HaveOccurred(), "Failed apply "+drainCheckFile)

			cmd := exec.Command("kubectl", "apply", "-f", drainCheckFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed apply DrainCheck")

			By("Creating a nodeDrain for our node")
			nodeDrain := fmt.Sprintf(`
apiVersion: k8s.gezb.co.uk/v1
kind: NodeDrain
metadata:
  name: nodedrain-sample
spec:
  nodeName: %s
  ignoreVersion: true
`, worker2Node)

			nodeDrainFile, err := utils.CreateTempFile(nodeDrain)
			Expect(err).NotTo(HaveOccurred(), "Failed apply nodeDrain")
			cmd = exec.Command("kubectl", "apply", "-f", nodeDrainFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed apply nodeDrain")

			By("Checking we still have all pods running")
			expectNumberOfPodsRunning(6)

			nodedrainIsPhaseCordened := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "nodedrain", "nodedrain-sample")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("nodedrain-sample   PodsBlockingDrain"),
					"coredened NodeDrain not found")
			}
			Eventually(nodedrainIsPhaseCordened, 5*time.Minute).Should(Succeed())

			By("Deleting the blocking statefulsets")
			cmd = exec.Command("kubectl", "delete", "statefulset", "blocking")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed delete nodeDrain")

			By("Drain should run and we should be left with only deamonsets")
			expectNumberOfPodsRunning(4)

			nodeDrainIsPhaseCompleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "nodedrain", "nodedrain-sample")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("nodedrain-sample   Completed"))
			}
			Eventually(nodeDrainIsPhaseCompleted, 5*time.Minute).Should(Succeed())

			By(" Deleting nginx statefulsets")
			cmd = exec.Command("kubectl", "delete", "statefulset", "nginx")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed delete nginx statefulset")

			By("deleting the NpdeDrain")
			cmd = exec.Command("kubectl", "delete", "-f", nodeDrainFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed delete nodeDrain")
			err = os.Remove(nodeDrainFile)
			Expect(err).NotTo(HaveOccurred(), "Failed remove nodeDrainFile")

			By("removing the drainCheck")
			cmd = exec.Command("kubectl", "delete", "-f", drainCheckFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed remove drainCheck")
			err = os.Remove(drainCheckFile)
			Expect(err).NotTo(HaveOccurred(), "Failed remove nodeDrainFile")

			By("re-cordening our test node")
			utils.CordonNode(worker2Node)
		})
	})

	It("if another node with the same 'role' & version exists uncodened should hold until it is cordened", func() {

		By("Uncordening our test node")
		utils.UncordonNode(worker2Node)

		By("Verifying the number of pods on the node(s)")

		expectNumberOfPodsRunning(4)

		createStatefulSetWithName("nginx")
		createStatefulSetWithName("nginx2")

		By("Waiting for all pods to be running")
		expectNumberOfPodsRunning(6)

		By("Cordening the worker2 now it has workload on it")
		utils.CordonNode(worker2Node)

		By("Uncordening the other node")
		utils.UncordonNode(worker3Node)

		By("Creating a nodeDrain for our node")
		nodeDrain := fmt.Sprintf(`
apiVersion: k8s.gezb.co.uk/v1
kind: NodeDrain
metadata:
  name: nodedrain-sample
spec:
  nodeName: %s
  ignoreVersion: true
  disableCordon: true # manually cordening nodes
   
`, worker2Node)

		nodeDrainFile, err := utils.CreateTempFile(nodeDrain)
		Expect(err).NotTo(HaveOccurred(), "Failed apply nodeDrain")
		cmd := exec.Command("kubectl", "apply", "-f", nodeDrainFile)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed apply nodeDrain")

		nodeDrainIsPhaseOtherNodesNotCordoned := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", "nodedrain-sample")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("nodedrain-sample   OtherNodesNotCordoned"))
		}
		Eventually(nodeDrainIsPhaseOtherNodesNotCordoned, 5*time.Minute).Should(Succeed())

		By("Cordening worker3")
		cmd = exec.Command("kubectl", "cordon", worker3Node)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed delete nodeDrain")

		By("Drain should run and we should be left with only deamonsets")
		expectNumberOfPodsRunning(4)

		nodeDrainIsPhaseCompleted := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", "nodedrain-sample")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("nodedrain-sample   Completed"))
		}
		Eventually(nodeDrainIsPhaseCompleted, 5*time.Minute).Should(Succeed())

		By(" Deleting nginx statefulsets")
		cmd = exec.Command("kubectl", "delete", "statefulset", "nginx")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed delete nginx statefulset")

		By(" Deleting nginx2 statefulsets")
		cmd = exec.Command("kubectl", "delete", "statefulset", "nginx2")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed delete nginx statefulset")

		By("deleting the NodeDrain")
		cmd = exec.Command("kubectl", "delete", "-f", nodeDrainFile)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed delete nodeDrain")
		err = os.Remove(nodeDrainFile)
		Expect(err).NotTo(HaveOccurred(), "Failed remove nodeDrainFile")

		By("re-cordening our test nodes")
		utils.CordonNode(worker2Node)
		utils.CordonNode(worker3Node)
	})

	It("if configured will wait for evicted pods to get to running state", func() {

		By("Uncordening our test nodes")
		utils.UncordonNode(worker2Node)

		By("Verifying the number of pods on the node(s)")

		expectNumberOfPodsRunning(4)

		createStatefulSetWithName("nginx")
		createStatefulSetWithName("nginx2")

		By("Waiting for all pods to be running")
		expectNumberOfPodsRunning(6)

		By("Creating a nodeDrain for our node")
		nodeDrain := fmt.Sprintf(`
apiVersion: k8s.gezb.co.uk/v1
kind: NodeDrain
metadata:
  name: nodedrain-sample
spec:
  nodeName: %s
  ignoreVersion: true
  waitForPodsToRestart: true 
   
`, worker2Node)

		nodeDrainFile, err := utils.CreateTempFile(nodeDrain)
		Expect(err).NotTo(HaveOccurred(), "Failed apply nodeDrain")
		cmd := exec.Command("kubectl", "apply", "-f", nodeDrainFile)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed apply nodeDrain")

		By("Drain should run and we should be left with only deamonsets")
		expectNumberOfPodsRunning(4)

		nodeDrainIsPhaseWaitForPodsToRestart := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", "nodedrain-sample")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("nodedrain-sample   WaitForPodsToRestart"))
		}
		Eventually(nodeDrainIsPhaseWaitForPodsToRestart, 5*time.Minute).Should(Succeed())

		By("Uncordoning node (simulate new node) Pods should start and drain should go to completed")

		utils.UncordonNode(worker2Node)

		nodeDrainIsPhaseCompleted := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "nodedrain", "nodedrain-sample")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("nodedrain-sample   Completed"))
		}
		Eventually(nodeDrainIsPhaseCompleted, 5*time.Minute).Should(Succeed())

		By(" Deleting nginx statefulsets")
		cmd = exec.Command("kubectl", "delete", "statefulset", "nginx")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed delete nginx statefulset")

		By(" Deleting nginx2 statefulsets")
		cmd = exec.Command("kubectl", "delete", "statefulset", "nginx2")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed delete nginx statefulset")

		By("deleting the NodeDrain")
		cmd = exec.Command("kubectl", "delete", "-f", nodeDrainFile)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed delete nodeDrain")
		err = os.Remove(nodeDrainFile)
		Expect(err).NotTo(HaveOccurred(), "Failed remove nodeDrainFile")

		By("re-cordening our test nodes")
		utils.CordonNode(worker2Node)
	})
})

func expectNumberOfPodsRunning(expected int) {
	var worker2, worker3 []string
	verifyAllPodsRunning := func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods",
			"-o", "wide",
			"-A",
			"--field-selector", "spec.nodeName="+worker2Node)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		worker2 = strings.Split(output, "\n")
		worker2 = worker2[:len(worker2)-1] // remove the empty last line
		worker2 = worker2[1:]              // remove header
		for _, line := range worker2 {
			g.Expect(line).To(ContainSubstring("Running"))
		}
		cmd = exec.Command("kubectl", "get", "pods",
			"-o", "wide",
			"-A",
			"--field-selector", "spec.nodeName="+worker3Node)
		output, err = utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		worker3 = strings.Split(output, "\n")
		worker3 = worker3[:len(worker3)-1] // remove the empty last line
		worker3 = worker3[1:]              // remove header
		for _, line := range worker3 {
			g.Expect(line).To(ContainSubstring("Running"))
		}
		g.Expect(len(worker2) + len(worker3)).To(Equal(expected))
	}
	EventuallyWithOffset(-2, verifyAllPodsRunning, 3*time.Minute).Should(Succeed())
}

func createStatefulSetWithName(name string) {
	By("starting StatefulSet with name " + name)
	inflate := fmt.Sprintf(`
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{.metadata.name}}
spec:
  selector:
    matchLabels:
      app: {{.metadata.name}}
  replicas: 1
  template:
    metadata:
      labels:
        app: {{.metadata.name}}
    spec:
      terminationGracePeriodSeconds: 60
      containers:
      - name: {{.metadata.name}}
        image: public.ecr.aws/eks-distro/kubernetes/pause:3.7
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
        role: %s
`, k8sRole)
	inflate = strings.Replace(inflate, "{{.metadata.name}}", name, -1)
	inflateFile, err := utils.CreateTempFile(inflate)
	Expect(err).NotTo(HaveOccurred(), "Failed apply "+name)
	cmd := exec.Command("kubectl", "apply", "-f", inflateFile)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed apply "+name)
	err = os.Remove(inflateFile)
	Expect(err).NotTo(HaveOccurred(), "Failed remove nodeDrainFile")

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
