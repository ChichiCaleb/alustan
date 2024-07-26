package installargocd

import (
	"context"
	"fmt"
	"os"
	"time"
	"sync"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Global lock to prevent concurrent installations
var installLock sync.Mutex

func InstallArgoCD(logger *zap.SugaredLogger, clientset kubernetes.Interface, dynClient dynamic.Interface, version string) error {
	// Get ArgoCD configuration from environment variable
	argocdConfig := os.Getenv("ARGOCD_CONFIG")

	// Check if ArgoCD is already installed and ready
	installed, ready, err := isArgoCDInstalledAndReady(logger, clientset)
	if err != nil {
		return fmt.Errorf("failed to check if ArgoCD is installed and ready: %w", err)
	}

	if installed && ready {
		logger.Info("ArgoCD is already installed and ready")
		return nil
	}

	// Lock to prevent concurrent installations
	installLock.Lock()
	defer installLock.Unlock()

	// Check again if ArgoCD is still not ready after acquiring lock
	installed, ready, err = isArgoCDInstalledAndReady(logger, clientset)
	if err != nil {
		return fmt.Errorf("failed to check if ArgoCD is installed and ready after acquiring lock: %w", err)
	}

	if installed && ready {
		logger.Info("ArgoCD is already installed and ready after acquiring lock")
		return nil
	}

	// Install ArgoCD using Helm
	err = installArgoCDWithHelm(logger, clientset, argocdConfig, version)
	if err != nil {
		return fmt.Errorf("failed to install ArgoCD with Helm: %w", err)
	}

	logger.Info("ArgoCD installed successfully")
	return nil
}

func isArgoCDInstalledAndReady(logger *zap.SugaredLogger,clientset kubernetes.Interface) (bool, bool, error) {
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), "argocd", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}

	// Check for the presence and readiness of ArgoCD components
	deployments := []string{
		"argo-cd-argocd-applicationset-controller",
		"argo-cd-argocd-notifications-controller",
		"argo-cd-argocd-server",
		"argo-cd-argocd-repo-server",
		"argo-cd-argocd-redis",
		"argo-cd-argocd-dex-server",
	}

	for _, deployment := range deployments {
		deploy, err := clientset.AppsV1().Deployments("argocd").Get(context.TODO(), deployment, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("ArgoCD Components not found.installing...")
				return false, false, nil
			}
			return false, false, err
		}

		// Check if the number of ready replicas matches the desired replicas
		if deploy.Status.ReadyReplicas != *deploy.Spec.Replicas {
			return true, false, nil // Components are installed but not ready
		}
	}

	return true, true, nil // All components are installed and ready
}


func installArgoCDWithHelm(logger *zap.SugaredLogger, clientset kubernetes.Interface, argocdConfig, version string) error {

	settings := cli.New()
	actionConfig := new(action.Configuration)

	err := actionConfig.Init(settings.RESTClientGetter(), "argocd", "", logger.Infof)
	if err != nil {
		return fmt.Errorf("failed to initialize Helm action configuration: %w", err)
	}

	// Add the repository
	repoEntry := repo.Entry{
		Name: "argo-cd",
		URL:  "https://argoproj.github.io/argo-helm",
	}
	chartRepo, err := repo.NewChartRepository(&repoEntry, getter.All(settings))
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	_, err = chartRepo.DownloadIndexFile()
	if err != nil {
		return fmt.Errorf("failed to download index file: %w", err)
	}

	repoFile := &repo.File{}
	if _, err := os.Stat(settings.RepositoryConfig); err == nil {
		repoFile, err = repo.LoadFile(settings.RepositoryConfig)
		if err != nil {
			return fmt.Errorf("failed to load repository config: %w", err)
		}
	}

	if !repoFile.Has(repoEntry.Name) {
		repoFile.Update(&repoEntry)
		if err := repoFile.WriteFile(settings.RepositoryConfig, 0644); err != nil {
			return fmt.Errorf("failed to write repository config: %w", err)
		}
	}

	chartName := "argo-cd/argo-cd"
	install := action.NewInstall(actionConfig)
	chartPath, err := install.LocateChart(chartName, settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}

	chart, err := loader.Load(chartPath)
	if (err != nil) {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	valOpts := &values.Options{}
	defaultVals, err := valOpts.MergeValues(getter.All(settings))
	if err != nil {
		return fmt.Errorf("failed to get default values: %w", err)
	}

	var vals map[string]interface{}
	if argocdConfig != "" {
		// Parse the provided YAML values
		providedVals := map[string]interface{}{}
		err = yaml.Unmarshal([]byte(argocdConfig), &providedVals)
		if err != nil {
			return fmt.Errorf("failed to parse ArgoCD configuration: %w", err)
		}

		// Merge default values with provided values
		vals = deepMerge(defaultVals, providedVals)
	} else {
		// Use default values if no configuration is provided
		vals = defaultVals
	}

	// Check if another operation is in progress
	statusClient := action.NewStatus(actionConfig)
	release, err := statusClient.Run("argo-cd")
	if err == nil && release.Info.Status.IsPending() {
		logger.Warn("Another operation is in progress for release argo-cd, skipping new operation.")
		return nil // Return without starting a new operation
	}

	// Perform install or upgrade with exponential backoff retry mechanism
	err = wait.ExponentialBackoff(RetryBackoff(), func() (bool, error) {
		histClient := action.NewHistory(actionConfig)
		histClient.Max = 1
		_, err := histClient.Run("argo-cd")
		if err == nil {
			// If the release exists, perform an upgrade
			err = upgradeArgoCD(actionConfig, chart, vals, logger)
		} else {
			// If the release does not exist, perform a new installation
			err = installArgoCD(actionConfig, chart, vals, logger)
		}

		if err != nil {
			return false, err
		}

		return true, nil // Stop retrying if successful
	})

	if err != nil {
		return fmt.Errorf("failed to install/upgrade ArgoCD with Helm: %w", err)
	}

	return nil
}


func upgradeArgoCD(actionConfig *action.Configuration, chart *chart.Chart, vals map[string]interface{}, logger *zap.SugaredLogger) error {
	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = "argocd"
	upgrade.Wait = true
	upgrade.Timeout = 20 * time.Minute // Set timeout to 20 minutes
	upgrade.Atomic = true // Enable atomic option

	_, err := upgrade.Run("argo-cd", chart, vals)
	if err != nil {
		return fmt.Errorf("failed to upgrade ArgoCD with Helm: %w", err)
	}
	return nil
}

func installArgoCD(actionConfig *action.Configuration, chart *chart.Chart, vals map[string]interface{}, logger *zap.SugaredLogger) error {
	install := action.NewInstall(actionConfig)
	install.ReleaseName = "argo-cd"
	install.Namespace = "argocd"
	install.CreateNamespace = true
	install.Wait = true
	install.Timeout = 20 * time.Minute // Set timeout to 20 minutes
	install.Atomic = true // Enable atomic option

	_, err := install.Run(chart, vals)
	if err != nil {
		return fmt.Errorf("failed to install ArgoCD with Helm: %w", err)
	}
	return nil
}

func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	for key, srcValue := range src {
		if srcMap, ok := srcValue.(map[string]interface{}); ok {
			if dstValue, found := dst[key]; found {
				if dstMap, ok := dstValue.(map[string]interface{}); ok {
					dst[key] = deepMerge(dstMap, srcMap)
					continue
				}
			}
		}
		dst[key] = srcValue
	}
	return dst
}

func RetryBackoff() wait.Backoff {
	return wait.Backoff{
		Duration: 5 * time.Second, // Initial delay
		Factor:   2,               // Exponential factor
		Steps:    5,               // Number of retry attempts
	}
}
