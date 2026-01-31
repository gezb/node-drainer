package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gezb/node-drainer/test/utils"
)

// k8sRole is the role of the node reserved for drain testing
const k8sRole = "draintest"

// k8sCuster is the name of the k3d cluster used for e2e testing
const k8sCuster = "nd-e2e"

// K8sVersionRegex is the regex to match the k8s version used in the e2e tests
const K8sVersionRegex = "^v1\\.34\\..*$"

// k8sKubeconfig is the path to the kubeconfig file used for e2e testing
const k8sKubeconfig = "/tmp/nd-e2e-kubeconfig.yaml"

// worker2Node is the name of the node reserved for drain testing
const worker2Node = "k3d-nd-e2e-agent-1"

// worker3Node is the name of the node reserved for further testing
const worker3Node = "k3d-nd-e2e-agent-2"

// worker4Node is the name of the node reserved for further testing
const worker4Node = "k3d-nd-e2e-agent-3"

var (
	// Optional Environment Variables:
	// - PROMETHEUS_INSTALL_SKIP=true: Skips Prometheus Operator installation during test setup.
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if Prometheus or CertManager is already installed, avoiding
	// re-installation and conflicts.
	skipPrometheusInstall = os.Getenv("PROMETHEUS_INSTALL_SKIP") == "true"
	// skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	skipCertManagerInstall = true
	// isPrometheusOperatorAlreadyInstalled will be set true when prometheus CRDs be found on the cluster
	isPrometheusOperatorAlreadyInstalled = false
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false
	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "node-drain-controller:e2e-tests"
	// testNodes is an array of the nodes that are reserved for drain testing
	testNodes = []string{worker2Node, worker3Node, worker4Node}
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the the purposed to be used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager and Prometheus.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting node-drainer integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("Setting KUBECONFIG environment variable")
	err := os.Setenv("KUBECONFIG", k8sKubeconfig)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to set KUBECONFIG environment variable")

	By("Labeling and cordening the test nodes so only our workloads run on them")

	// worker2
	utils.LabelNode(worker2Node, k8sRole)
	utils.CordonNode(worker2Node)

	// worker 3
	utils.LabelNode(worker3Node, k8sRole)
	utils.CordonNode(worker3Node)

	// worker 4
	utils.LabelNode(worker4Node, k8sRole)
	utils.CordonNode(worker4Node)

	By("Draining the test nodes to ensure a clean state")
	utils.DrainNodes([]string{worker2Node, worker3Node, worker4Node})

	By("Ensure that Prometheus is enabled")
	_ = utils.UncommentCode("config/default/kustomization.yaml", "#- ../prometheus", "#")

	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	By("loading the manager(Operator) image on k3d")
	err = utils.LoadImageToK3dClusterWithName(k8sCuster, projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with Prometheus or CertManager already installed,
	// we check for their presence before execution.
	// Setup Prometheus and CertManager before the suite if not skipped and if not already installed
	if !skipPrometheusInstall {
		By("checking if prometheus is installed already")
		isPrometheusOperatorAlreadyInstalled = utils.IsPrometheusCRDsInstalled()
		if !isPrometheusOperatorAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing Prometheus Operator...\n")
			Expect(utils.InstallPrometheusOperator()).To(Succeed(), "Failed to install Prometheus Operator")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Prometheus Operator is already installed. Skipping installation...\n")
		}
	}
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}
})

var _ = AfterSuite(func() {
	// Teardown Prometheus and CertManager after the suite if not skipped and if they were not already installed
	if !skipPrometheusInstall && !isPrometheusOperatorAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling Prometheus Operator...\n")
		utils.UninstallPrometheusOperator()
	}
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}
	By("Unsetting KUBECONFIG environment variable")
	err := os.Unsetenv("KUBECONFIG")
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to unset KUBECONFIG environment variable")
})
