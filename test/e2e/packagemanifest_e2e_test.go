package e2e

import (
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/controller/registry"
	packagev1 "github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/operators/v1"
	pmversioned "github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/client/clientset/versioned"
)

type packageManifestCheckFunc func(*packagev1.PackageManifest) bool

func packageManifestHasStatus(pm *packagev1.PackageManifest) bool {
	// as long as it has a package name we consider the status non-empty
	if pm == nil || pm.Status.PackageName == "" {
		return false
	}

	return true
}

func fetchPackageManifest(t *testing.T, pmc pmversioned.Interface, namespace, name string, check packageManifestCheckFunc) (*packagev1.PackageManifest, error) {
	var fetched *packagev1.PackageManifest
	var err error

	err = wait.Poll(pollInterval, pollDuration, func() (bool, error) {
		t.Logf("Polling...")
		fetched, err = pmc.OperatorsV1().PackageManifests(namespace).Get(name, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return true, err
		}
		return check(fetched), nil
	})

	return fetched, err
}

var Describe = ginkgo.Describe
var When = ginkgo.Context
var It = ginkgo.It
var By = ginkgo.By

func TestPackageManifest(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "PackageManifest Suite")
}

var _ = Describe("PackageManifest", func() {
	It("should be available as a cluster API", func() {

	})

	Describe("global CatalogSources", func() {

	})

	Describe("local CatalogSources", func() {

	})

	It("should return items from global CatalogSources", func() {
		When("fetching from global catalog namespace", func() {

		})

		When("fetching from a different namespace", func() {

		})

		When("fetching from all-namespaces", func() {

		})
	})

	It("should return items from local CatalogSource", func() {
		When("fetching from all-namespaces", func() {

		})

		When("fetching from local CatalogSource namespace", func() {

		})
	})
})

// TODO(alecmerdler): Rewrite using Ginkgo test framework
func TestPackageManifestLoading(t *testing.T) {
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
	catalogSourceName := genName("mock-ocs")
	namedStrategy := newNginxInstallStrategy(genName("dep-"), nil, nil)
	csv := newCSV(packageStable, testNamespace, "", semver.MustParse("0.1.0"), []apiextensions.CustomResourceDefinition{crd}, nil, namedStrategy)

	c := newKubeClient(t)
	crc := newCRClient(t)
	pmc := newPMClient(t)

	expectedStatus := packagev1.PackageManifestStatus{
		CatalogSource:          catalogSourceName,
		CatalogSourceNamespace: testNamespace,
		PackageName:            packageName,
		Channels: []packagev1.PackageChannel{
			{
				Name:           stableChannel,
				CurrentCSV:     packageStable,
				CurrentCSVDesc: packagev1.CreateCSVDescription(&csv),
			},
		},
		DefaultChannel: stableChannel,
	}

	// Wait for package-server to be ready
	err := wait.Poll(pollInterval, 1*time.Minute, func() (bool, error) {
		t.Logf("Polling package-server...")
		_, err := pmc.OperatorsV1().PackageManifests(testNamespace).List(metav1.ListOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	require.NoError(t, err, "package-server not available")

	_, cleanupCatalogSource := createInternalCatalogSource(t, c, crc, catalogSourceName, testNamespace, manifests, []apiextensions.CustomResourceDefinition{crd}, []v1alpha1.ClusterServiceVersion{csv})
	require.NoError(t, err)
	defer cleanupCatalogSource()

	_, err = fetchCatalogSource(t, crc, catalogSourceName, testNamespace, catalogSourceRegistryPodSynced)
	require.NoError(t, err)

	pm, err := fetchPackageManifest(t, pmc, testNamespace, packageName, packageManifestHasStatus)
	require.NoError(t, err, "error getting package manifest")
	require.NotNil(t, pm)
	require.Equal(t, packageName, pm.GetName())
	require.Equal(t, expectedStatus, pm.Status)

	// Get a PackageManifestList and ensure it has the correct items
	pmList, err := pmc.OperatorsV1().PackageManifests(testNamespace).List(metav1.ListOptions{})
	require.NoError(t, err, "could not access package manifests list meta")
	require.NotNil(t, pmList.ListMeta, "package manifest list metadata empty")
	require.NotNil(t, pmList.Items)
}
