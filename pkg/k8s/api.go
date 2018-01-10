package k8s

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/runconduit/conduit/pkg/healthcheck"
	"github.com/runconduit/conduit/pkg/shell"
	"k8s.io/client-go/rest"
	"regexp"
)

const (
	kubernetesConfigFilePathEnvVariable = "KUBECONFIG"
	KubeapiSubsystemName                = "kubernetes-api"
	KubeapiClientCheckDescription       = "can initialize the client"
	KubeapiAccessCheckDescription       = "can query the Kubernetes API"
)
var /* const */ httpPrefix = regexp.MustCompile(`://`)

type KubernetesApi interface {
	UrlFor(namespace string, extraPathStartingWithSlash string) (*url.URL, error)
	NewClient() (*http.Client, error)
	healthcheck.StatusChecker
}

type kubernetesApi struct {
	config               *rest.Config
	apiSchemeHostAndPort string
}

func (kubeapi *kubernetesApi) NewClient() (*http.Client, error) {
	secureTransport, err := rest.TransportFor(kubeapi.config)
	if err != nil {
		return nil, fmt.Errorf("error instantiating Kubernetes API client: %v", err)
	}

	return &http.Client{
		Transport: secureTransport,
	}, nil
}

func (kubeapi *kubernetesApi) SelfCheck() ([]healthcheck.CheckResult, error) {
	apiConnectivityCheck := healthcheck.CheckResult{
		Status:           healthcheck.CheckError,
		SubsystemName:    KubeapiSubsystemName,
		CheckDescription: KubeapiClientCheckDescription,
	}

	client, err := kubeapi.NewClient()
	if err != nil {
		apiConnectivityCheck.Status = healthcheck.CheckError
		apiConnectivityCheck.NextSteps = fmt.Sprintf("Error connecting to the API. Error message is [%s]", err.Error())
	} else {
		apiConnectivityCheck.Status = healthcheck.CheckOk
	}

	apiAccessCheck := healthcheck.CheckResult{
		Status:           healthcheck.CheckError,
		SubsystemName:    KubeapiSubsystemName,
		CheckDescription: KubeapiAccessCheckDescription,
	}

	endpointToCheck, err := generateBaseKubernetesApiUrl(kubeapi.apiSchemeHostAndPort)
	if err != nil {
		apiAccessCheck.Status = healthcheck.CheckError
		apiAccessCheck.NextSteps = fmt.Sprintf("Error querying Kubernetes API. Configured host is [%s], error message is [%s]", kubeapi.apiSchemeHostAndPort, err.Error())
	} else {
		if client != nil {
			resp, err := client.Get(endpointToCheck.String())
			if err != nil {
				apiAccessCheck.Status = healthcheck.CheckError
				apiAccessCheck.NextSteps = fmt.Sprintf("HTTP GET request to endpoint [%s] resulted in error: [%s]", endpointToCheck, err.Error())
			} else {
				statusCodeReturnedIsWithinSuccessRange := resp.StatusCode < 400
				if statusCodeReturnedIsWithinSuccessRange {
					apiAccessCheck.Status = healthcheck.CheckOk
				} else {
					bytes, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						apiAccessCheck.Status = healthcheck.CheckError
						apiAccessCheck.NextSteps = fmt.Sprintf("HTTP GET request to endpoint [%s] resulted in invalid response: [%v]", endpointToCheck, resp)
					} else {
						body := string(bytes)

						apiAccessCheck.Status = healthcheck.CheckFailed
						apiAccessCheck.NextSteps = fmt.Sprintf("HTTP GET request to endpoint [%s] resulted in Status: [%s], body: [%s]", endpointToCheck, resp.Status, body)
					}
				}
			}
		}
	}
	results := []healthcheck.CheckResult{
		apiConnectivityCheck,
		apiAccessCheck,
	}
	return results, nil
}

func (kubeapi *kubernetesApi) UrlFor(namespace string, extraPathStartingWithSlash string) (*url.URL, error) {
	return generateKubernetesApiBaseUrlFor(kubeapi.apiSchemeHostAndPort, namespace, extraPathStartingWithSlash)
}

func NewK8sAPI(shell shell.Shell, k8sConfigFilesystemPathOverride string, apiHostAndPortOverride string) (KubernetesApi, error) {
	kubeconfigEnvVar := os.Getenv(kubernetesConfigFilePathEnvVariable)

	config, err := parseK8SConfig(findK8sConfigFile(k8sConfigFilesystemPathOverride, kubeconfigEnvVar, shell.HomeDir()))
	if err != nil {
		return nil, fmt.Errorf("error configuring Kubernetes API client: %v", err)
	}

	if apiHostAndPortOverride == "" {
		return &kubernetesApi{
			apiSchemeHostAndPort: config.Host,
			config:               config,
		}, nil
	} else {
		parsedURL, err := url.Parse(apiHostAndPortOverride)
		if err != nil {
			return nil, fmt.Errorf("error parsing override url: %v", err)
		}

		if parsedURL.Scheme == "" {
			apiHostAndPortOverride = fmt.Sprintf("https://%s", apiHostAndPortOverride)
		}
		return &kubernetesApi{
			apiSchemeHostAndPort: apiHostAndPortOverride,
			config:               config,
		}, nil
	}
}
