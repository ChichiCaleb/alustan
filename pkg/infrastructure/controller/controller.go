package controller

import (
	"context"
	"sync"
	"fmt"
	"strings"
	"time"
	"bytes"
	"io"
    "encoding/json"
    "net/http"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"  
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	

	"k8s.io/client-go/util/workqueue"
	"k8s.io/client-go/dynamic/dynamicinformer"
	apiclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	clusterpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/cluster"

	"github.com/alustan/alustan/pkg/infrastructure/registry"
	"github.com/alustan/alustan/api/infrastructure/v1alpha1"
	"github.com/alustan/alustan/pkg/infrastructure/terraform"
	"github.com/alustan/alustan/pkg/util"
	"github.com/alustan/alustan/pkg/infrastructure/listers"
	Kubernetespkg "github.com/alustan/alustan/pkg/infrastructure/kubernetes"
	"github.com/alustan/alustan/pkg/checkargo"
)

var (
    token     string
    tokenLock sync.Mutex
)

type Controller struct {
	Clientset        kubernetes.Interface
	dynClient        dynamic.Interface
	syncInterval     time.Duration
	lastSyncTime     time.Time
	workqueue        workqueue.RateLimitingInterface
	terraformLister  listers.TerraformLister
	informerFactory  dynamicinformer.DynamicSharedInformerFactory // Shared informer factory for Terraform resources
	informer         cache.SharedIndexInformer                    // Informer for Terraform resources
	logger           *zap.SugaredLogger
	mu               sync.Mutex
	numWorkers       int
	maxWorkers       int
	workerStopCh  chan struct{}
    managerStopCh chan struct{}
	argoClient   apiclient.Client
	clusterClient  clusterpkg.ClusterServiceClient
	
}


// NewController initializes a new controller
func NewController(clientset kubernetes.Interface, dynClient dynamic.Interface, syncInterval time.Duration, logger *zap.SugaredLogger) *Controller {
	ctrl := &Controller{
		Clientset:       clientset,
		dynClient:       dynClient,
		syncInterval:    syncInterval,
		lastSyncTime:    time.Now().Add(-syncInterval), // Initialize to allow immediate first run
		workqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "terraforms"),
		informerFactory: dynamicinformer.NewDynamicSharedInformerFactory(dynClient, syncInterval),
		logger:          logger,
		numWorkers:      0,
		maxWorkers:      5,
		workerStopCh:    make(chan struct{}),
		managerStopCh:    make(chan struct{}),
	}
	

	// Initialize informer
	ctrl.initInformer()

	return ctrl
}

// NewInClusterController initializes a new controller for in-cluster execution
func NewInClusterController(syncInterval time.Duration, logger *zap.SugaredLogger) *Controller {
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Fatalf("Error creating in-cluster config: %v", err)
	}

	config.QPS = 100.0
	config.Burst = 200

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Error creating Kubernetes clientset: %v", err)
	}

	// Check if ArgoCD is installed and ready
	installed, ready, err := checkargo.IsArgoCDInstalledAndReady(logger, clientset)
	if err != nil {
		logger.Fatalf("Error checking ArgoCD installation status: %v", err)
	}

	if !installed {
		logger.Fatalf("ArgoCD is not installed in the cluster")
	}

	if !ready {
		logger.Fatalf("ArgoCD components are installed but not ready")
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Error creating dynamic Kubernetes client: %v", err)
	}

	return NewController(clientset, dynClient, syncInterval, logger)
}

func (c *Controller) initInformer() {
	// Define the GroupVersionResource for the custom resource
	gvr := schema.GroupVersionResource{
		Group:    "alustan.io",
		Version:  "v1alpha1",
		Resource: "terraforms",
	}

	// Get the informer and error returned by ForResource
	informer := c.informerFactory.ForResource(gvr)
	c.informer = informer.Informer()

	// Set the lister for the custom resource
	c.terraformLister = listers.NewTerraformLister(c.informer.GetIndexer())

	// Add event handlers to the informer
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleAddTerraform,
		UpdateFunc: c.handleUpdateTerraform,
		DeleteFunc: c.handleDeleteTerraform,
	})
}

func (c *Controller) setupInformer(stopCh <-chan struct{}) {
	if c.informer == nil {
		c.logger.Fatal("informer is nil, ensure initInformer is called before setupInformer")
	}

	// Start the informer
	go c.informer.Run(stopCh)

	// Wait for the informer's cache to sync
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		c.logger.Error("timed out waiting for caches to sync")
		return
	}
}

func (c *Controller) handleAddTerraform(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		c.logger.Errorf("couldn't get key for object %+v: %v", obj, err)
		return
	}
	c.enqueue(key)
}

func (c *Controller) handleUpdateTerraform(old, new interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(new)
	if err != nil {
		c.logger.Errorf("couldn't get key for object %+v: %v", new, err)
		return
	}
	c.enqueue(key)
}

func (c *Controller) handleDeleteTerraform(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		c.logger.Errorf("couldn't get key for object %+v: %v", obj, err)
		return
	}
	c.enqueue(key)
}

func (c *Controller) enqueue(key string) {
	c.workqueue.AddRateLimited(key)
}

func (c *Controller) RunLeader(stopCh <-chan struct{}) {
	defer c.logger.Sync()

	c.logger.Info("Starting Terraform controller")

	// Setup informers and listers
	c.setupInformer(stopCh)

	// Leader election configuration
	id := util.GetUniqueID()
	rl, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		"alustan",
		"terraform-controller-lock",
		c.Clientset.CoreV1(),
		c.Clientset.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id,
		},
	)
	if err != nil {
		c.logger.Fatalf("Failed to create resource lock: %v", err)
	}

	leaderelection.RunOrDie(context.TODO(), leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: 30 * time.Second,
        RenewDeadline: 20 * time.Second,
        RetryPeriod:   5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				c.logger.Info("got leadership")

				 // Authenticate and create ArgoCD client
				 password, err := getAdminPassword(c.Clientset)
				 if err != nil {
					 c.logger.Fatalf("Failed to get admin password: %v", err)
				 }

				 authToken, err := GetAuthToken(c, password)
				 if err != nil {
					 c.logger.Fatalf("Failed to get auth token: %v", err)
				 }

				 err = refreshClients(c, authToken)
				 if err != nil {
					 c.logger.Fatalf("Failed to refresh clients: %v", err)
				 }

				 c.logger.Info("Successfully created ArgoCD client")
				 c.logger.Infof("Successfully created Cluster client")
	             c.logger.Info("Terraform controller succesfuly instantiated!!!")
				// Start processing items
				go c.manageWorkers()
			},
			OnStoppedLeading: func() {
				c.logger.Infof("Pod %s lost leadership", id)
				// Stop processing items
				close(c.workerStopCh)  // Stop all individual runWorker functions
				close(c.managerStopCh) // Stop the manageWorkers function
				c.workqueue.ShutDown()
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					c.logger.Infof("Pod %s is still the leader", id)
				} else {
					c.logger.Infof("New leader elected: %s", identity)
				}
			},
		},
		ReleaseOnCancel: true,
	})
}


func (c *Controller) manageWorkers() {
	for {
		select {
		case <-c.managerStopCh:
		  return // Stops the manageWorkers loop
        default:
			queueLength := c.workqueue.Len()

			c.mu.Lock()
			currentWorkers := c.numWorkers
			c.mu.Unlock()

			// Calculate the desired number of workers based on the queue length
			// spawn a new runworker routine for every queue length greater than 20 vice versa
			desiredWorkers := (queueLength / 20) + 1
			if desiredWorkers > c.maxWorkers {
				desiredWorkers = c.maxWorkers
			}

			// Ensure at least one worker is running
			if desiredWorkers < 1 {
				desiredWorkers = 1
			}

			// Increase workers if needed
			if currentWorkers < desiredWorkers {
				for i := currentWorkers; i < desiredWorkers; i++ {
					go c.runWorker()
					c.mu.Lock()
					c.numWorkers++
					c.mu.Unlock()
				}
			}

			// Decrease workers if needed, but ensure at least one worker is running
			if currentWorkers > desiredWorkers {
				for i := currentWorkers; i > desiredWorkers && i > 1; i-- {
					c.mu.Lock()
					c.numWorkers--
					c.mu.Unlock()
					c.workerStopCh <- struct{}{} // Signal a worker to stop

				}
			}

			time.Sleep(1 * time.Minute)
		}
	}
}


func (c *Controller) runWorker() {
    for {
        select {
        case <-c.workerStopCh:
            return // Stops the individual worker
        default:
            if !c.processNextWorkItem() {
                return
            }
        }
    }
}


func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		key, ok := obj.(string)
		if !ok {
			c.workqueue.Forget(obj)
			c.logger.Errorf("expected string in workqueue but got %T", obj)
			return nil
		}

		namespace, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			c.workqueue.Forget(obj)
			c.logger.Errorf("invalid resource key: %s", key)
			return nil
		}

		// Get the actual resource using the lister
		terraformObject, err := c.terraformLister.Terraform(namespace).Get(name)
		if err != nil {
		    // Check if the error message contains "not found"
			if strings.Contains(err.Error(), "not found") {
				c.workqueue.Forget(obj)
				c.logger.Infof("resource %s/%s no longer exists", namespace, name)
				return nil
			}

			// For other errors, decide whether to requeue or not
			if errors.IsInternalError(err) || errors.IsServerTimeout(err) {
				// These are considered transient errors, requeue the item
				c.workqueue.AddRateLimited(key)
				c.logger.Errorf("transient error fetching resource. requeing!! %s: %v", key, err)
				return fmt.Errorf("transient error fetching resource. requeing!! %s: %v", key, err)
			} else {
				// Non-transient errors, do not requeue the item
				c.workqueue.Forget(obj)
				c.logger.Errorf("non-transient error fetching resource %s: %v", key, err)
				return fmt.Errorf("non-transient error fetching resource %s: %v", key, err)
			}
		}

		terraformObj := terraformObject.DeepCopyObject()

		// Convert to *v1alpha1.Terraform
		unstructuredObj, ok := terraformObj.(*unstructured.Unstructured)
		if !ok {
			c.workqueue.Forget(obj)
			c.logger.Errorf("expected *unstructured.Unstructured but got %T", terraformObj)
			return nil
		}
		terraform := &v1alpha1.Terraform{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, terraform)
		if err != nil {
			c.workqueue.Forget(obj)
			c.logger.Errorf("error converting unstructured object to *v1alpha1.Terraform: %v", err)
			return nil
		}

		// Retrieve generation information from status
		generation := terraform.GetGeneration()
		observedGeneration := terraform.Status.ObservedGeneration

		// Convert generation to int if necessary
		gen := int(generation)

		if gen > observedGeneration {
			// Perform synchronization and update observed generation
			finalStatus, err := c.handleSyncRequest(terraform)
			if finalStatus.Message == "Destroy completed successfully" {
               return nil
			}
			if err != nil {
				finalStatus.State = "Error"
				finalStatus.Message = err.Error()
				c.workqueue.AddRateLimited(key)
				c.logger.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
				return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
			}

			finalStatus.ObservedGeneration = gen
			updateErr := c.updateStatus(terraform, finalStatus)
			if updateErr != nil {
				c.logger.Infof("Failed to update status for %s: %v", key, updateErr)
				c.workqueue.AddRateLimited(key)
				return updateErr
			}
		}

		c.workqueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		c.logger.Error(err)
		return true
	}

	return true
}

func (c *Controller) handleSyncRequest(observed *v1alpha1.Terraform) (v1alpha1.TerraformStatus, error) {
     
   envVars := util.ExtractEnvVars(observed.Spec.Variables)
    secretName := fmt.Sprintf("%s-container-secret", observed.ObjectMeta.Name)
    c.logger.Infof("Observed Parent Spec: %+v", observed.Spec)

	commonStatus := v1alpha1.TerraformStatus{
        State:   "Progressing",
        Message: "Starting processing",
    }
	// Add finalizer if not already present
	err := Kubernetespkg.AddFinalizer(c.logger, c.dynClient, observed.ObjectMeta.Name, observed.ObjectMeta.Namespace)
	if err != nil {
		c.logger.Errorf("Failed to add finalizer for %s/%s: %v", observed.ObjectMeta.Namespace, observed.ObjectMeta.Name, err)
		commonStatus.State = "Error"
		commonStatus.Message = fmt.Sprintf("Failed to add finalizer: %v", err)
		return commonStatus, err
	}
	 finalizing := false
    // Check if the resource is being deleted
    if observed.ObjectMeta.DeletionTimestamp != nil {
		
        finalizing = true
    }

    // Handle script content
    scriptContent, scriptContentStatus := terraform.GetScriptContent(c.logger,observed, finalizing)
    commonStatus = mergeStatuses(commonStatus, scriptContentStatus)
    if scriptContentStatus.State == "Error" {
        return commonStatus, fmt.Errorf("error getting script content")
    }

    // Handle tagged image name
    taggedImageName, taggedImageStatus := registry.GetTaggedImageName(c.logger,observed, scriptContent, c.Clientset, finalizing)
    commonStatus = mergeStatuses(commonStatus, taggedImageStatus)
    if taggedImageStatus.State == "Error" {
        return commonStatus, fmt.Errorf("error getting tagged image name")
    }

    c.logger.Infof("taggedImageName: %v", taggedImageName)

    // Handle ExecuteTerraform
    execTerraformStatus := terraform.ExecuteTerraform(c.logger,c.Clientset, c.dynClient, c.clusterClient, observed, scriptContent, taggedImageName, secretName, envVars, finalizing)
    commonStatus = mergeStatuses(commonStatus, execTerraformStatus)

    if execTerraformStatus.State == "Error" {
        return commonStatus, fmt.Errorf("error executing terraform")
    }
  
   return commonStatus, nil
}

func mergeStatuses(baseStatus, newStatus v1alpha1.TerraformStatus) v1alpha1.TerraformStatus {
    if newStatus.State != "" {
        baseStatus.State = newStatus.State
    }
    if newStatus.Message != "" {
        baseStatus.Message = newStatus.Message
    }
   
    if newStatus.PostDeployOutput != nil {
        baseStatus.PostDeployOutput = newStatus.PostDeployOutput
    }
   
   
    return baseStatus
}


func (c *Controller) updateStatus(observed *v1alpha1.Terraform, status v1alpha1.TerraformStatus) error {
	err := Kubernetespkg.UpdateStatus(c.logger, c.dynClient, observed.ObjectMeta.Name, observed.ObjectMeta.Namespace, status)
	
	if err != nil {
		c.logger.Errorf("Failed to update status for %s/%s: %v", observed.ObjectMeta.Namespace, observed.ObjectMeta.Name, err)
		return err
	}
	return nil

}


func getAdminPassword(clientset kubernetes.Interface) (string, error) {
    namespace := "argocd"
    secretName := "argocd-initial-admin-secret"

    // Retrieve the secret from Kubernetes
    secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
    if err != nil {
        return "", fmt.Errorf("failed to get secret: %v", err)
    }

    // Ensure the password field exists
    passwordBytes, exists := secret.Data["password"]
    if !exists {
        return "", fmt.Errorf("password not found in secret")
    }

    // Print the raw password for debugging
    password := string(passwordBytes)
   


    return password, nil
}


func GenerateAuthToken(password string) (string, error) {
    argoURL := "http://argo-cd-argocd-server.argocd.svc.cluster.local/api/v1/session"
    payload := map[string]string{
        "username": "admin",
        "password": password,
    }
    payloadBytes, _ := json.Marshal(payload)


    client := &http.Client{
        Timeout: 30 * time.Second,
    }

    req, err := http.NewRequest("POST", argoURL, bytes.NewBuffer(payloadBytes))
    if err != nil {
        return "", fmt.Errorf("failed to create request: %v", err)
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("failed to send request: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
      return "", fmt.Errorf("authentication failed: %v, response: %s", resp.Status, string(bodyBytes))
    }

    var response map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return "", fmt.Errorf("failed to decode response: %v", err)
    }

    token, ok := response["token"].(string)
    if !ok {
       
      return "", fmt.Errorf("token not found in response")
    }


    return token, nil
}




// GetAuthToken retrieves the current token, generating a new one if necessary
func GetAuthToken(c *Controller, password string) (string, error) {
    tokenLock.Lock()
    defer tokenLock.Unlock()

    if token == "" {
        var err error
        token, err = GenerateAuthToken(password)
        if err != nil {
            return "", fmt.Errorf("failed to generate auth token: %v", err)
        }
        go scheduleTokenRefresh(c, password)
    }

    return token, nil
}


// scheduleTokenRefresh sets up a routine to refresh the token before it expires and update clients
func scheduleTokenRefresh(c *Controller, password string) {
    for {
        // Schedule the token refresh before the actual expiry time
        time.Sleep(23 * time.Hour) // Assuming a 24-hour token expiry, adjust as necessary
        tokenLock.Lock()

        newToken, err := GenerateAuthToken(password)
        if err != nil {
            fmt.Printf("failed to refresh token: %v\n", err)
            tokenLock.Unlock()
            continue
        }

        token = newToken
        err = refreshClients(c, newToken) // Pass controller reference
        if err != nil {
            fmt.Printf("failed to refresh clients: %v\n", err)
        }

        tokenLock.Unlock()
    }
}


// CreateArgoCDClient creates and returns an Argo CD client
func CreateArgoCDClient(authToken string) (apiclient.Client, error) {
   
    argoURL := "argo-cd-argocd-server.argocd.svc.cluster.local"

    // Create Argo CD client options with the token
    argoClientOpts := apiclient.ClientOptions{
        ServerAddr: argoURL,
        AuthToken:  authToken,
		PlainText: true,
    }

    // Create the Argo CD client
    argoClient, err := apiclient.NewClient(&argoClientOpts)
    if err != nil {
        return nil, fmt.Errorf("failed to create Argo CD client: %v", err)
    }

    return argoClient, nil
}

func refreshClients(c *Controller, newToken string) error {
    // Update the Argo CD client with the new token
    newArgoClient, err := CreateArgoCDClient(newToken)
    if err != nil {
        return fmt.Errorf("failed to create Argo CD client: %v", err)
    }

    // Update the clients stored in the controller struct
    c.argoClient = newArgoClient

    _, newClusterClient, err := newArgoClient.NewClusterClient()
    if err != nil {
        return fmt.Errorf("failed to create ArgoCD cluster client: %v", err)
    }
    c.clusterClient = newClusterClient

   return nil
}


