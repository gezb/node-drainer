package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type PodDetails struct {
	Name   string
	Status string
}

const (
	prometheusOperatorVersion = "v0.77.1"
	prometheusOperatorURL     = "https://github.com/prometheus-operator/prometheus-operator/" +
		"releases/download/%s/bundle.yaml"

	certmanagerVersion = "v1.16.3"
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed with error: (%v) %s", command, err, string(output))
	}

	return string(output), nil
}

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
func InstallPrometheusOperator() error {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "create", "-f", url)
	_, err := Run(cmd)
	return err
}

// UninstallPrometheusOperator uninstalls the prometheus
func UninstallPrometheusOperator() {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// IsPrometheusCRDsInstalled checks if any Prometheus CRDs are installed
// by verifying the existence of key CRDs related to Prometheus.
func IsPrometheusCRDsInstalled() bool {
	// List of common Prometheus CRDs
	prometheusCRDs := []string{
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"prometheusagents.monitoring.coreos.com",
	}

	cmd := exec.Command("kubectl", "get", "crds", "-o", "custom-columns=NAME:.metadata.name")
	output, err := Run(cmd)
	if err != nil {
		return false
	}
	crdList := GetNonEmptyLines(output)
	for _, crd := range prometheusCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	_, err := Run(cmd)
	return err
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	cluster := "kind"
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	cmd := exec.Command("kind", kindOptions...)
	_, err := Run(cmd)
	return err
}

// LoadImageToK3dClusterWithName loads a local docker image to the k3d cluster
func LoadImageToK3dClusterWithName(clusterName, imageName string) error {
	k3dOptions := []string{"image", "import", imageName, "--cluster", clusterName}
	cmd := exec.Command("k3d", k3dOptions...)
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %s to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		_, err := out.WriteString(strings.TrimPrefix(scanner.Text(), prefix))
		if err != nil {
			return err
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err := out.WriteString("\n"); err != nil {
			return err
		}
	}

	_, err = out.Write(content[idx+len(target):])
	if err != nil {
		return err
	}
	// false positive
	// nolint:gosec
	return os.WriteFile(filename, out.Bytes(), 0644)
}

func ApplyDrainCheck(name string, podRegex string) {
	By("Creating DrainCheck for:" + podRegex)

	drainCheck := fmt.Sprintf(`
apiVersion: k8s.gezb.co.uk/v1
kind: DrainCheck
metadata:
  name: %s
spec:
  podRegex: %s
`, name, podRegex)

	drainCheckFile, err := CreateTempFile(drainCheck)
	Expect(err).NotTo(HaveOccurred(), "Failed creating DrainCheck file to apply: "+drainCheckFile)

	cmd := exec.Command("kubectl", "apply", "-f", drainCheckFile)
	_, err = Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed apply DrainCheck")
}

// DeleteDrainCheck deletes the given DrainCheck resource
func DeleteDrainCheck(drainCheckName string) {
	cmd := exec.Command("kubectl", "delete", "draincheck", drainCheckName)
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed delete drainCheck")
}

// creates and applies a NodeDrain resource for the given node
func ApplyNodeDrain(nodeCount int, nodeName string, waitForPodToRestart bool, versionToDrainRegex string, role string) {
	By("Creating NodeDrain for node :" + nodeName)

	nodeDrain := fmt.Sprintf(`
apiVersion: k8s.gezb.co.uk/v1
kind: NodeDrain
metadata:
  name: %s
spec:
  numberOfNodes: %d
  nodeName: %s
  versionToDrainRegex: %s
  nodeRole: %s
  skipWaitForPodsToRestart: %t
`, nodeName, nodeCount, nodeName, versionToDrainRegex, role, !waitForPodToRestart)

	nodeDrainFile, err := CreateTempFile(nodeDrain)
	Expect(err).NotTo(HaveOccurred(), "Failed creating NodeDrain file to apply: "+nodeDrainFile)

	cmd := exec.Command("kubectl", "apply", "-f", nodeDrainFile)
	_, err = Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed apply NodeDrain")
}

// DeleteNodeDrain deletes the given NodeDrain resource
func DeleteNodeDrain(NodeDrainName string) {
	cmd := exec.Command("kubectl", "delete", "nodedrain", NodeDrainName)
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed delete nodeDrain")
}

// asserts that the number of pods running on the given nodes equals expected
func ExpectNumberOfPodsRunning(nodes []string, expected int) {
	verifyAllPodsRunning := func(g Gomega) {
		numberOfPods := 0
		for _, nodeName := range nodes {
			podDetailsForNode := getPodDetailsForNode(nodeName)
			for _, podDetails := range podDetailsForNode {
				g.Expect(podDetails.Status).To(Equal("Running"))
			}
			numberOfPods += len(podDetailsForNode)
		}
		g.ExpectWithOffset(1, numberOfPods).To(Equal(expected))
	}
	EventuallyWithOffset(1, verifyAllPodsRunning).Should(Succeed())
}

// GePodsForNode returns the list of pods running on a given node
func GetPodsOnNode(nodeName string) []string {
	podDetails := getPodDetailsForNode(nodeName)
	pods := make([]string, 0, len(podDetails))
	for _, pd := range podDetails {
		pods = append(pods, pd.Name)
	}
	return pods
}

// gets the output from Kubectl get pods filtered for a given node
func getPodDetailsForNode(nodeName string) []PodDetails {
	cmd := exec.Command("kubectl", "get", "pods",
		"-o", "wide",
		"-A",
		"--field-selector", fmt.Sprintf("spec.nodeName=%s", nodeName))
	output, err := Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	lines := strings.Split(output, "\n")
	lines = lines[:len(lines)-1] // remove the empty last line
	lines = lines[1:]            // remove header
	podDetails := make([]PodDetails, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		podDetails = append(podDetails, PodDetails{
			Name:   fields[1],
			Status: fields[3],
		})
	}
	return podDetails
}

// creates a StatefuleSet with the given name and replicas that schedules on nodes with the given role
func CreateStatefulSetWithName(name string, replicas int, role string) {
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
  replicas: %d
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
`, replicas, role)
	inflate = strings.ReplaceAll(inflate, "{{.metadata.name}}", name)
	inflateFile, err := CreateTempFile(inflate)
	Expect(err).NotTo(HaveOccurred(), "Failed apply "+name)
	cmd := exec.Command("kubectl", "apply", "-f", inflateFile)
	_, err = Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed apply "+name)
	err = os.Remove(inflateFile)
	Expect(err).NotTo(HaveOccurred(), "Failed remove nodeDrainFile")
}

// DeletePod deletes the given pod
func DeletePod(podName string) {
	cmd := exec.Command("kubectl", "delete", "pod", podName)
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed delete nodeDrain")
}

// DeleteStatefulSet deletes the given StatefulSet
func DeleteStatefulSet(podName string) {
	cmd := exec.Command("kubectl", "delete", "statefulset", podName)
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed delete nodeDrain")
}

// Returns a map of pod names to their start times on the specified nodes
func GetPodStartTimesOnNodes(nodes []string) map[string]time.Time {
	podStartTimes := make(map[string]time.Time)
	for _, nodeName := range nodes {
		cmd := exec.Command("kubectl", "get", "pods",
			"-o", "wide",
			"-A",
			"--field-selector", fmt.Sprintf("spec.nodeName=%s", nodeName))
		output, err := Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		lines := strings.Split(output, "\n")
		lines = lines[:len(lines)-1] // remove the empty last line
		lines = lines[1:]            // remove header
		for _, pod := range lines {
			fields := strings.Fields(pod)
			podName := fields[1]
			namespace := fields[0]
			// Get detailed pod info to extract the start time
			cmdDetail := exec.Command("kubectl", "get", "pod", podName, "-n", namespace, "-o", "jsonpath={.status.startTime}")
			startTimeStr, err := Run(cmdDetail)
			Expect(err).NotTo(HaveOccurred())
			startTime, err := time.Parse(time.RFC3339, startTimeStr)
			Expect(err).NotTo(HaveOccurred())
			podStartTimes[podName] = startTime
		}
	}
	return podStartTimes
}

// CreateTempFile creates a temporary file containing the given content
func CreateTempFile(content string) (string, error) {
	tmpFile, err := os.CreateTemp("", "k8syaml")
	if err != nil {
		return "", err
	}
	if _, err = tmpFile.WriteString(content); err != nil {
		return "", err
	}
	err = tmpFile.Close()
	if err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

func LabelNode(nodeName string, role string) {
	cmd := exec.Command("kubectl", "label", "node", nodeName, "role="+role)
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to label node "+role)
}

func DrainNodes(nodeNames []string) {
	args := append([]string{"drain", "--ignore-daemonsets", "--delete-emptydir-data"}, nodeNames...)
	cmd := exec.Command("kubectl", args...)
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to drain nodes "+strings.Join(nodeNames, ", "))
}

func CordonNode(nodeName string) {
	cmd := exec.Command("kubectl", "cordon", nodeName)
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to corden "+nodeName)
}

func UncordonNode(nodeName string) {
	cmd := exec.Command("kubectl", "uncordon", nodeName)
	_, err := Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to corden "+nodeName)
}
