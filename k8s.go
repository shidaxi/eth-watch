package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultServiceRegex = `.*(rpc|geth|reth|erigon|besu|nethermind).*`
	defaultPortRegex    = `.+45$`
)

func isInK8s() bool {
	return os.Getenv("KUBERNETES_SERVICE_HOST") != ""
}

// discoveryRegexes returns the compiled service-name and port regexes,
// sourced from DISCOVERY_K8S_SERVICE_REGEX / DISCOVERY_K8S_PORT_REGEX env
// vars with fallback to sensible defaults.
func discoveryRegexes() (svcRe *regexp.Regexp, portRe *regexp.Regexp) {
	svcPat := os.Getenv("DISCOVERY_K8S_SERVICE_REGEX")
	if svcPat == "" {
		svcPat = defaultServiceRegex
	}
	portPat := os.Getenv("DISCOVERY_K8S_PORT_REGEX")
	if portPat == "" {
		portPat = defaultPortRegex
	}
	svcRe = regexp.MustCompile(svcPat)
	portRe = regexp.MustCompile(portPat)
	return
}

// discoverK8sRPCCandidates lists all services across all namespaces and
// returns URLs for ports whose service name or port number matches the
// configured discovery regexes.
func discoverK8sRPCCandidates() ([]string, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}

	svcs, err := client.CoreV1().Services("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	svcRe, portRe := discoveryRegexes()

	var candidates []string
	for _, svc := range svcs.Items {
		// ExternalName services don't have cluster-internal IPs.
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			continue
		}
		svcMatch := svcRe.MatchString(svc.Name)
		for _, port := range svc.Spec.Ports {
			portMatch := portRe.MatchString(strconv.Itoa(int(port.Port)))
			if svcMatch || portMatch {
				url := fmt.Sprintf("http://%s.%s:%d", svc.Name, svc.Namespace, port.Port)
				candidates = append(candidates, url)
			}
		}
	}
	return candidates, nil
}

// probeRPCCandidates concurrently probes each URL with a lightweight
// eth_chainId call and returns only those that respond as a valid JSON-RPC node.
func probeRPCCandidates(candidates []string, _ time.Duration) []string {
	const probeTimeout = 2 * time.Second

	type result struct {
		url  string
		live bool
	}
	ch := make(chan result, len(candidates))
	for _, url := range candidates {
		go func(u string) {
			ch <- result{url: u, live: probeNode(u, probeTimeout)}
		}(url)
	}
	var live []string
	for range candidates {
		r := <-ch
		if r.live {
			live = append(live, r.url)
		}
	}
	return live
}
