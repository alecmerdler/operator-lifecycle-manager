//  +build !bare

package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/controller/registry"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/operatorclient"
)

func TestCatalogLoadingBetweenRestarts(t *testing.T) {
	defer cleaner.NotifyTestComplete(t, true)

	// create a simple catalogsource
	packageName := genName("nginx")
	stableChannel := "stable"
	packageStable := packageName + "-stable"
	manifests := []registry.PackageManifest{
		{
			PackageName: packageName,
			Channels: []registry.PackageChannel{
				{Name: stableChannel, CurrentCSVName: packageStable},
			},
			DefaultChannelName: stableChannel,
		},
	}

	crdPlural := genName("ins")
	crd := newCRD(crdPlural)
	namedStrategy := newNginxInstallStrategy(genName("dep-"), nil, nil)
	csv := newCSV(packageStable, testNamespace, "", *semver.New("0.1.0"), []apiextensions.CustomResourceDefinition{crd}, nil, namedStrategy)

	c := newKubeClient(t)
	crc := newCRClient(t)

	catalogSourceName := genName("mock-ocs-")
	_, cleanupCatalogSource := createInternalCatalogSource(t, c, crc, catalogSourceName, operatorNamespace, manifests, []apiextensions.CustomResourceDefinition{crd}, []v1alpha1.ClusterServiceVersion{csv})
	defer cleanupCatalogSource()

	// ensure the mock catalog exists and has been synced by the catalog operator
	catalogSource, err := fetchCatalogSource(t, crc, catalogSourceName, operatorNamespace, catalogSourceRegistryPodSynced)
	require.NoError(t, err)

	// get catalog operator deployment
	deployment, err := getOperatorDeployment(c, operatorNamespace, labels.Set{"app": "catalog-operator"})
	require.NoError(t, err)
	require.NotNil(t, deployment, "Could not find catalog operator deployment")

	// rescale catalog operator
	t.Log("Rescaling catalog operator...")
	err = rescaleDeployment(c, deployment)
	require.NoError(t, err, "Could not rescale catalog operator")
	t.Log("Catalog operator rescaled")

	// check for last synced update to catalogsource
	t.Log("Checking for catalogsource lastSync updates")
	_, err = fetchCatalogSource(t, crc, catalogSourceName, operatorNamespace, func(cs *v1alpha1.CatalogSource) bool {
		if cs.Status.LastSync.After(catalogSource.Status.LastSync.Time) {
			t.Logf("lastSync updated: %s -> %s", catalogSource.Status.LastSync, cs.Status.LastSync)
			return true
		}
		return false
	})
	require.NoError(t, err, "Catalog source changed after rescale")
	t.Logf("Catalog source sucessfully loaded after rescale")
}

func TestDefaultCatalogLoading(t *testing.T) {
	defer cleaner.NotifyTestComplete(t, true)
	c := newKubeClient(t)
	crc := newCRClient(t)

	catalogSource, err := fetchCatalogSource(t, crc, "olm-operators", operatorNamespace, catalogSourceRegistryPodSynced)
	require.NoError(t, err)
	requirement, err := labels.NewRequirement("olm.catalogSource", selection.Equals, []string{catalogSource.GetName()})
	require.NoError(t, err)
	selector := labels.NewSelector().Add(*requirement).String()
	pods, err := c.KubernetesInterface().CoreV1().Pods(operatorNamespace).List(metav1.ListOptions{LabelSelector: selector})
	require.NoError(t, err)
	for _, p := range pods.Items {
		for _, s := range p.Status.ContainerStatuses {
			require.True(t, s.Ready)
			require.Zero(t, s.RestartCount)
		}
	}
}

func TestConfigMapUpdateTriggersRegistryPodRollout(t *testing.T) {
	defer cleaner.NotifyTestComplete(t, true)

	mainPackageName := genName("nginx-")
	dependentPackageName := genName("nginxdep-")

	mainPackageStable := fmt.Sprintf("%s-stable", mainPackageName)
	dependentPackageStable := fmt.Sprintf("%s-stable", dependentPackageName)

	stableChannel := "stable"

	mainNamedStrategy := newNginxInstallStrategy(genName("dep-"), nil, nil)
	dependentNamedStrategy := newNginxInstallStrategy(genName("dep-"), nil, nil)

	crdPlural := genName("ins-")

	dependentCRD := newCRD(crdPlural)
	mainCSV := newCSV(mainPackageStable, testNamespace, "", *semver.New("0.1.0"), nil, []apiextensions.CustomResourceDefinition{dependentCRD}, mainNamedStrategy)
	dependentCSV := newCSV(dependentPackageStable, testNamespace, "", *semver.New("0.1.0"), []apiextensions.CustomResourceDefinition{dependentCRD}, nil, dependentNamedStrategy)

	c := newKubeClient(t)
	crc := newCRClient(t)

	mainCatalogName := genName("mock-ocs-main-")

	// Create separate manifests for each CatalogSource
	mainManifests := []registry.PackageManifest{
		{
			PackageName: mainPackageName,
			Channels: []registry.PackageChannel{
				{Name: stableChannel, CurrentCSVName: mainPackageStable},
			},
			DefaultChannelName: stableChannel,
		},
	}

	dependentManifests := []registry.PackageManifest{
		{
			PackageName: dependentPackageName,
			Channels: []registry.PackageChannel{
				{Name: stableChannel, CurrentCSVName: dependentPackageStable},
			},
			DefaultChannelName: stableChannel,
		},
	}

	// Create the initial catalogsource
	createInternalCatalogSource(t, c, crc, mainCatalogName, testNamespace, mainManifests, nil, []v1alpha1.ClusterServiceVersion{mainCSV})

	// Attempt to get the catalog source before creating install plan
	fetchedInitialCatalog, err := fetchCatalogSource(t, crc, mainCatalogName, testNamespace, catalogSourceRegistryPodSynced)
	require.NoError(t, err)

	// Get initial configmap
	configMap, err := c.KubernetesInterface().CoreV1().ConfigMaps(testNamespace).Get(fetchedInitialCatalog.Spec.ConfigMap, metav1.GetOptions{})
	require.NoError(t, err)

	// Check pod created
	initialPods, err := c.KubernetesInterface().CoreV1().Pods(testNamespace).List(metav1.ListOptions{LabelSelector: "olm.configMapResourceVersion=" + configMap.ResourceVersion})
	require.NoError(t, err)
	require.Equal(t, 1, len(initialPods.Items))

	// Update raw manifests
	manifestsRaw, err := yaml.Marshal(append(mainManifests, dependentManifests...))
	require.NoError(t, err)
	configMap.Data[registry.ConfigMapPackageName] = string(manifestsRaw)

	// Update raw CRDs
	var crdsRaw []byte
	crdStrings := []string{}
	for _, crd := range []apiextensions.CustomResourceDefinition{dependentCRD} {
		crdStrings = append(crdStrings, serializeCRD(t, crd))
	}
	crdsRaw, err = yaml.Marshal(crdStrings)
	require.NoError(t, err)
	configMap.Data[registry.ConfigMapCRDName] = strings.Replace(string(crdsRaw), "- |\n  ", "- ", -1)

	// Update raw CSVs
	csvsRaw, err := yaml.Marshal([]v1alpha1.ClusterServiceVersion{mainCSV, dependentCSV})
	require.NoError(t, err)
	configMap.Data[registry.ConfigMapCSVName] = string(csvsRaw)

	// Update configmap
	updatedConfigMap, err := c.KubernetesInterface().CoreV1().ConfigMaps(testNamespace).Update(configMap)
	require.NoError(t, err)

	fetchedUpdatedCatalog, err := fetchCatalogSource(t, crc, mainCatalogName, testNamespace, func(catalog *v1alpha1.CatalogSource) bool {
		if catalog.Status.LastSync != fetchedInitialCatalog.Status.LastSync {
			fmt.Println("catalog updated")
			return true
		}
		fmt.Println("waiting for catalog pod to be available")
		return false
	})
	require.NoError(t, err)

	require.NotEqual(t, updatedConfigMap.ResourceVersion, configMap.ResourceVersion)
	require.NotEqual(t, fetchedUpdatedCatalog.Status.ConfigMapResource.ResourceVersion, fetchedInitialCatalog.Status.ConfigMapResource.ResourceVersion)
	require.Equal(t, updatedConfigMap.GetResourceVersion(), fetchedUpdatedCatalog.Status.ConfigMapResource.ResourceVersion)

	// Create Subscription
	subscriptionName := genName("sub-")
	createSubscriptionForCatalog(t, crc, testNamespace, subscriptionName, fetchedUpdatedCatalog.GetName(), mainPackageName, stableChannel, v1alpha1.ApprovalAutomatic)

	subscription, err := fetchSubscription(t, crc, testNamespace, subscriptionName, subscriptionStateAtLatestChecker)
	require.NoError(t, err)
	require.NotNil(t, subscription)
	_, err = fetchCSV(t, crc, subscription.Status.CurrentCSV, testNamespace, buildCSVConditionChecker(v1alpha1.CSVPhaseSucceeded))
	require.NoError(t, err)

	ipList, err := crc.OperatorsV1alpha1().InstallPlans(testNamespace).List(metav1.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(ipList.Items))
}

func TestConfigMapReplaceTriggersRegistryPodRollout(t *testing.T) {
	defer cleaner.NotifyTestComplete(t, true)

	mainPackageName := genName("nginx-")
	dependentPackageName := genName("nginxdep-")

	mainPackageStable := fmt.Sprintf("%s-stable", mainPackageName)
	dependentPackageStable := fmt.Sprintf("%s-stable", dependentPackageName)

	stableChannel := "stable"

	mainNamedStrategy := newNginxInstallStrategy(genName("dep-"), nil, nil)
	dependentNamedStrategy := newNginxInstallStrategy(genName("dep-"), nil, nil)

	crdPlural := genName("ins-")

	dependentCRD := newCRD(crdPlural)
	mainCSV := newCSV(mainPackageStable, testNamespace, "", *semver.New("0.1.0"), nil, []apiextensions.CustomResourceDefinition{dependentCRD}, mainNamedStrategy)
	dependentCSV := newCSV(dependentPackageStable, testNamespace, "", *semver.New("0.1.0"), []apiextensions.CustomResourceDefinition{dependentCRD}, nil, dependentNamedStrategy)

	c := newKubeClient(t)
	crc := newCRClient(t)

	mainCatalogName := genName("mock-ocs-main-")

	// Create separate manifests for each CatalogSource
	mainManifests := []registry.PackageManifest{
		{
			PackageName: mainPackageName,
			Channels: []registry.PackageChannel{
				{Name: stableChannel, CurrentCSVName: mainPackageStable},
			},
			DefaultChannelName: stableChannel,
		},
	}

	dependentManifests := []registry.PackageManifest{
		{
			PackageName: dependentPackageName,
			Channels: []registry.PackageChannel{
				{Name: stableChannel, CurrentCSVName: dependentPackageStable},
			},
			DefaultChannelName: stableChannel,
		},
	}

	// Create the initial catalogsource
	_, cleanupCatalog := createInternalCatalogSource(t, c, crc, mainCatalogName, testNamespace, mainManifests, nil, []v1alpha1.ClusterServiceVersion{mainCSV})

	// Attempt to get the catalog source before creating install plan
	fetchedInitialCatalog, err := fetchCatalogSource(t, crc, mainCatalogName, testNamespace, catalogSourceRegistryPodSynced)
	require.NoError(t, err)
	// Get initial configmap
	configMap, err := c.KubernetesInterface().CoreV1().ConfigMaps(testNamespace).Get(fetchedInitialCatalog.Spec.ConfigMap, metav1.GetOptions{})
	require.NoError(t, err)

	// Check pod created
	initialPods, err := c.KubernetesInterface().CoreV1().Pods(testNamespace).List(metav1.ListOptions{LabelSelector: "olm.configMapResourceVersion=" + configMap.ResourceVersion})
	require.NoError(t, err)
	require.Equal(t, 1, len(initialPods.Items))

	// delete the first catalog
	cleanupCatalog()

	// create a catalog with the same name
	createInternalCatalogSource(t, c, crc, mainCatalogName, testNamespace, append(mainManifests, dependentManifests...), []apiextensions.CustomResourceDefinition{dependentCRD}, []v1alpha1.ClusterServiceVersion{mainCSV, dependentCSV})

	// Create Subscription
	subscriptionName := genName("sub-")
	createSubscriptionForCatalog(t, crc, testNamespace, subscriptionName, mainCatalogName, mainPackageName, stableChannel, v1alpha1.ApprovalAutomatic)

	subscription, err := fetchSubscription(t, crc, testNamespace, subscriptionName, subscriptionStateAtLatestChecker)
	require.NoError(t, err)
	require.NotNil(t, subscription)
	_, err = fetchCSV(t, crc, subscription.Status.CurrentCSV, testNamespace, buildCSVConditionChecker(v1alpha1.CSVPhaseSucceeded))
	require.NoError(t, err)

}

func getOperatorDeployment(c operatorclient.ClientInterface, namespace string, operatorLabels labels.Set) (*appsv1.Deployment, error) {
	deployments, err := c.ListDeploymentsWithLabels(namespace, operatorLabels)
	if err != nil || deployments == nil || len(deployments.Items) != 1 {
		return nil, fmt.Errorf("Error getting single operator deployment for label: %v", operatorLabels)
	}
	return &deployments.Items[0], nil
}

func rescaleDeployment(c operatorclient.ClientInterface, deployment *appsv1.Deployment) error {
	// scale down
	var replicas int32 = 0
	deployment.Spec.Replicas = &replicas
	deployment, updated, err := c.UpdateDeployment(deployment)
	if err != nil || updated == false || deployment == nil {
		return fmt.Errorf("Failed to scale down deployment")
	}

	waitForScaleup := func() (bool, error) {
		fetchedDeployment, err := c.GetDeployment(deployment.GetNamespace(), deployment.GetName())
		if err != nil {
			return true, err
		}
		if fetchedDeployment.Status.Replicas == replicas {
			return true, nil
		}

		return false, nil
	}

	// wait for deployment to scale down
	err = wait.Poll(pollInterval, pollDuration, waitForScaleup)
	if err != nil {
		return err
	}

	// scale up
	replicas = 1
	deployment.Spec.Replicas = &replicas
	deployment, updated, err = c.UpdateDeployment(deployment)
	if err != nil || updated == false || deployment == nil {
		return fmt.Errorf("Failed to scale up deployment")
	}

	// wait for deployment to scale up
	err = wait.Poll(pollInterval, pollDuration, waitForScaleup)

	return err
}
