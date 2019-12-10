package e2e

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"golang.org/x/exp/rand"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func round(num int, ratio float64) int {
	return int(math.Floor((float64(num) * ratio) + 0.5))
}

var (
	scaleTestPolicyRules = flag.Int("scale.policiesNum", 10000, "Number of policies for scale tests")
	scaleTestPodNum      = flag.Int("scale.podsNum", 5000, "Number of pods for scale tests")
	scaleNeedSetup       = flag.Bool("scale.setup", false, "Set up scale test pods")
	scaleCleanup         = flag.Bool("scale.scaleCleanup", false, "Clean up scale test after running")
	scalePrometheusAddr  = flag.String("scale.prometheus", "localhost:9090", "Address of the prometheus server")
)

func generateScaleTestTopo(podsNum int) map[int]int {
	return map[int]int{
		round(podsNum, 1):     round(*scaleTestPolicyRules, 0.001),
		round(podsNum, 0.05):  round(*scaleTestPolicyRules, 0.039),
		round(podsNum, 0.005): round(*scaleTestPolicyRules, 0.16),
		round(podsNum, 0.001): round(*scaleTestPolicyRules, 0.80),
	}
}

func withScaleTestSetup(fn func(data *TestData), b *testing.B) {
	data, err := setupTest(b)
	if err != nil {
		b.Fatalf("Error when setting up test: %v", err)
	}
	defer teardownTest(b, data)
	if *scaleCleanup {
		defer func() {
			b.Logf("Cleaning up the entire scale test namespce")
			data.clientset.CoreV1().Namespaces().Delete(testNamespace, new(metav1.DeleteOptions))
		}()
	}
	fn(data)
}

// generateScaleTestLabels generates Pod labels for the specific topo.
func generateScaleTestLabels(topo map[int]int) []map[string]string {
	log.Printf("Using topo: %v", topo)
	tags := make([]map[string]string, *scaleTestPodNum)
	for i := 0; i < *scaleTestPodNum; i++ {
		tags[i] = make(map[string]string)
	}
	seq := make([]int, *scaleTestPodNum)
	for i := range seq {
		seq[i] = i
	}
	for podNum, npNum := range topo {
		for npIdx := 0; npIdx < npNum; npIdx++ {
			rand.Shuffle(*scaleTestPodNum, func(i, j int) { seq[i], seq[j] = seq[j], seq[i] })
			for _, podIdx := range seq[:podNum] {
				tags[podIdx][fmt.Sprintf("%d_%d", podNum, npIdx)] = "yes"
			}
		}
	}
	rand.Shuffle(*scaleTestPodNum, func(i, j int) { tags[i], tags[j] = tags[j], tags[i] })
	for _, podTag := range tags {
		fmt.Printf("%v\n", podTag)
		fmt.Println()
	}
	return tags
}

func deployScalePods(labels []map[string]string, data *TestData) error {
	gracePeriod := int64(0)
	fgPropagation := metav1.DeletePropagationForeground

	err := data.clientset.CoreV1().Pods(testNamespace).DeleteCollection(&metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
		PropagationPolicy:  &fgPropagation,
	}, metav1.ListOptions{})
	if err != nil {
		log.Printf("Try to delete ns failed: %v", err)
	}

	_, err = data.clientset.CoreV1().Namespaces().Create(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		log.Printf("Error when creating ns %s: %v", testNamespace, err)
		return err
	}

	var wg sync.WaitGroup
	taskCh := make(chan *corev1.Pod, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case pod := <-taskCh:
					_, err := data.clientset.CoreV1().Pods(testNamespace).Create(pod)
					if err != nil {
						log.Fatalf("error when creating pod#%d: %v", i, err)
					}
				case <-time.After(time.Second):
					log.Println("A worker finished")
					return
				}

			}
		}()
	}

	for i := 0; i < *scaleTestPodNum; i++ {
		taskCh <- &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("scale-test-idle-%d", i),
				Namespace: testNamespace,
				Labels:    labels[i],
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:            "idle",
						Image:           "tangweiqiang/idle",
						ImagePullPolicy: corev1.PullIfNotPresent,
					},
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "node-role.kubernetes.io/master",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			},
		}
	}
	wg.Wait()
	return nil
}

func applyNetworkPolicies(topo map[int]int, data *TestData) {
	rndSrc := rand.NewSource(0x59EED)
	var rndMutex sync.Mutex

	applyNetworkPolicy := func(group, idx int) error {
		var cidr string
		func() {
			rndMutex.Lock()
			defer rndMutex.Unlock()
			cidr = randCidr(rndSrc)
		}()
		policyName := fmt.Sprintf("%d-%d.ingress", group, idx)
		labelMatcher := metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf("%d_%d", group, idx): "yes"}}
		_, err := data.clientset.NetworkingV1().NetworkPolicies(testNamespace).Create(&netv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: testNamespace},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: labelMatcher,
				Ingress:     []netv1.NetworkPolicyIngressRule{{From: []netv1.NetworkPolicyPeer{{PodSelector: &labelMatcher}}}},
				Egress:      []netv1.NetworkPolicyEgressRule{{To: []netv1.NetworkPolicyPeer{{IPBlock: &netv1.IPBlock{CIDR: cidr}}}}},
				PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress},
			},
		})
		if err != nil {
			return fmt.Errorf("error when appling %s: %w", fmt.Sprintf("%d_%d.ingress", group, idx), err)
		}
		return nil
	}

	var wg sync.WaitGroup
	concurrency := 64
	taskCh := make(chan struct{ group, idx int }, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case task := <-taskCh:
					err := applyNetworkPolicy(task.group, task.idx)
					if err != nil {
						log.Fatalf("Error when applying network policy: %v", err)
					}
				case <-time.After(time.Second):
					log.Println("A worker finished")
					return
				}

			}
		}()
	}
	for group, num := range topo {
		for idx := 0; idx < num; idx++ {
			taskCh <- struct{ group, idx int }{group, idx}
		}
	}
	wg.Wait()
}

func waitPodToRun(num int, data *TestData) error {
	var mutex sync.Mutex
	realized := make(map[string]struct{})
	watcher, err := data.clientset.CoreV1().Pods(testNamespace).Watch(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error when watching pods: %w", err)
	}

	go func() {
		defer watcher.Stop()
		for evt := range watcher.ResultChan() {
			mutex.Lock()
			pod := evt.Object.(*corev1.Pod)
			if pod.Status.Phase == corev1.PodRunning {
				realized[pod.Name] = struct{}{}
			}
			allReady := len(realized) == num
			mutex.Unlock()
			if allReady {
				break
			}
		}
	}()

	return wait.PollUntil(5*time.Second, func() (done bool, err error) {
		mutex.Lock()
		defer mutex.Unlock()
		log.Printf("Pods realized %d/%d", len(realized), num)
		return len(realized) == num, nil
	}, nil)
}

func prometheusClient(data *TestData) (promapi.Client, error) {
	u := url.URL{
		Scheme: "http",
		Host:   *scalePrometheusAddr,
	}
	return promapi.NewClient(promapi.Config{Address: u.String()})
}

func controllerQueueLength(ctx context.Context, metricName string, pClient promapi.Client) int {
	val, err := v1.NewAPI(pClient).Query(ctx, metricName, time.Now())
	if err != nil {
		log.Printf("Error when retrieving %s: %v", metricName, err)
		return 0
	}
	vec := val.(model.Vector)
	if len(vec) < 1 {
		log.Printf("No data found for %s", metricName)
		return 0
	}
	r, err := strconv.ParseInt(vec[0].Value.String(), 10, 32)
	if err != nil {
		log.Printf("Error when parsing data: %v", err)
		return 0
	}
	return int(r)
}

func LenNetworkPolicyQueue(ctx context.Context, pClient promapi.Client) int {
	return controllerQueueLength(ctx, "length_network_policy_queue", pClient)
}
func LenAddressGroupQueue(ctx context.Context, pClient promapi.Client) int {
	return controllerQueueLength(ctx, "length_address_group_queue", pClient)
}

func LenAppliedToGroupQueue(ctx context.Context, pClient promapi.Client) int {
	return controllerQueueLength(ctx, "length_applied_to_group_queue", pClient)
}

func BenchmarkScale(b *testing.B) {
	withScaleTestSetup(func(data *TestData) {
		topo := generateScaleTestTopo(*scaleTestPodNum)
		labels := generateScaleTestLabels(topo)
		pc, err := prometheusClient(data)
		if err != nil {
			b.Fatalf("Error when creating prometheus client: %v", err)
		}
		if *scaleNeedSetup {
			go func() {
				log.Println("Deploying scale test pods")
				err := deployScalePods(labels, data)
				if err != nil {
					b.Fatalf("Error when deploying scale test pods: %v", err)
				}
			}()
		}

		log.Printf("Waiting for all pods to be ready")
		err = waitPodToRun(*scaleTestPodNum, data)
		if err != nil {
			b.Fatalf("Error when waiting for all pods to be ready: %v", err)
		}

		log.Printf("Applying network policies")
		applyNetworkPolicies(topo, data)

		log.Printf("Waiting all network policies to be processed")
		for i := 0; i < 3; i++ {
			ctx := context.Background()
			wait.PollUntil(10*time.Second, func() (bool, error) {
				npq, agq, atgq := LenNetworkPolicyQueue(ctx, pc), LenAddressGroupQueue(ctx, pc), LenAppliedToGroupQueue(ctx, pc)
				log.Printf("LenNetworkPolicyQueue: %d LenAddressGroupQueue: %d LenAppliedToGroupQueue: %d", npq, agq, atgq)
				return npq == 0 && agq == 0 && atgq == 0, nil
			}, nil)
		}
	}, b)
}
