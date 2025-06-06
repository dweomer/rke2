package multus

import (
	"flag"
	"fmt"

	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/rke2/tests/e2e"
)

// Valid nodeOS: bento/ubuntu-24.04, opensuse/Leap-15.6.x86_64
var nodeOS = flag.String("nodeOS", "bento/ubuntu-24.04", "VM operating system")
var serverCount = flag.Int("serverCount", 1, "number of server nodes")
var agentCount = flag.Int("agentCount", 1, "number of agent nodes")
var ci = flag.Bool("ci", false, "running on CI")
var local = flag.Bool("local", false, "deploy a locally built RKE2")

// Environment Variables Info:
// E2E_RELEASE_VERSION=v1.23.1+rke2r1 or nil for latest commit from master

func Test_E2Emultus(t *testing.T) {
	flag.Parse()
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	RunSpecs(t, "Validate multus Test Suite", suiteConfig, reporterConfig)
}

var tc *e2e.TestConfig
var _ = ReportAfterEach(e2e.GenReport)

var _ = Describe("Verify multus Configuration", Ordered, func() {

	It("Starts up with no issues", func() {
		var err error
		if *local {
			tc, err = e2e.CreateLocalCluster(*nodeOS, *serverCount, *agentCount)
		} else {
			tc, err = e2e.CreateCluster(*nodeOS, *serverCount, *agentCount)
		}
		Expect(err).NotTo(HaveOccurred(), e2e.GetVagrantLog(err))
		By("CLUSTER CONFIG")
		By("OS: " + *nodeOS)
		By(tc.Status())
		tc.KubeconfigFile, err = e2e.GenKubeConfigFile(tc.Servers[0])
		Expect(err).NotTo(HaveOccurred())
	})

	It("Checks Node Status", func() {
		Eventually(func(g Gomega) {
			nodes, err := e2e.ParseNodes(tc.KubeconfigFile, false)
			g.Expect(err).NotTo(HaveOccurred())
			for _, node := range nodes {
				g.Expect(node.Status).Should(Equal("Ready"))
			}
		}, "620s", "5s").Should(Succeed())
		_, err := e2e.ParseNodes(tc.KubeconfigFile, true)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Checks Pod Status", func() {
		Eventually(func(g Gomega) {
			pods, err := e2e.ParsePods(tc.KubeconfigFile, false)
			g.Expect(err).NotTo(HaveOccurred())
			for _, pod := range pods {
				if strings.Contains(pod.Name, "helm-install") {
					g.Expect(pod.Status).Should(Equal("Completed"), pod.Name)
				} else {
					g.Expect(pod.Status).Should(Equal("Running"), pod.Name)
				}
			}
		}, "420s", "5s").Should(Succeed())
		_, err := e2e.ParsePods(tc.KubeconfigFile, true)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Verifies multus daemonset comes up", func() {
		Eventually(func() (string, error) {
			cmd := "kubectl get ds rke2-multus -n kube-system -o jsonpath='{.status.numberReady}' --kubeconfig=" + tc.KubeconfigFile
			return e2e.RunCommand(cmd)
		}, "120s", "5s").Should(ContainSubstring("2"))
	})

	It("Verifies macvlan communication via multus is working", func() {
		_, err := tc.DeployWorkload("multus-pods.yaml")
		Expect(err).NotTo(HaveOccurred())
		cmd := "kubectl exec pod-macvlan --kubeconfig=" + tc.KubeconfigFile + " -- ping -c 1 -w 2 10.1.1.102"
		Eventually(func() (string, error) {
			return e2e.RunCommand(cmd)
		}, "20s", "3s").Should(ContainSubstring("0% packet loss"), "failed cmd: "+cmd)

	})
})

var failed bool
var _ = AfterEach(func() {
	failed = failed || CurrentSpecReport().Failed()
})

var _ = AfterSuite(func() {
	if failed && !*ci {
		fmt.Println("FAILED!")
	} else {
		Expect(e2e.DestroyCluster()).To(Succeed())
		Expect(os.Remove(tc.KubeconfigFile)).To(Succeed())
	}
})
