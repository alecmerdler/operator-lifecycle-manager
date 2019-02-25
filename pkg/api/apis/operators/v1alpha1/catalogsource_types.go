package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators"
)

const (
	CatalogSourceCRDAPIVersion = operators.GroupName + "/" + GroupVersion
	CatalogSourceKind          = "CatalogSource"
)

// SourceType indicates the type of backing store for a CatalogSource
type SourceType string

const (
	// SourceTypeInternal (deprecated) specifies a CatalogSource of type SourceTypeConfigmap
	SourceTypeInternal SourceType = "internal"

	// SourceTypeConfigmap specifies a CatalogSource that generates a configmap-server registry
	SourceTypeConfigmap SourceType = "configmap"

	// SourceTypeGrpc specifies a CatalogSource that can use an operator registry image to generate a
	// registry-server or connect to a pre-existing registry at an address.
	SourceTypeGrpc SourceType = "grpc"
)

type CatalogSourceSpec struct {
	// SourceType is the type of source
	SourceType SourceType `json:"sourceType"`

	// ConfigMap is the name of the ConfigMap to be used to back a configmap-server registry.
	// Only used when SourceType = SourceTypeConfigmap or SourceTypeInternal.
	// +Optional
	ConfigMap string `json:"configMap,omitempty"`

	// Address is a host that OLM can use to connect to a pre-existing registry.
	// Format: <registry-host or ip>:<port>
	// Only used when SourceType = SourceTypeGrpc.
	// Ignored when the Image field is set.
	// +Optional
	Address string `json:"address,omitempty"`

	// Image is an operator-registry container image to instantiate a registry-server with.
	// Only used when SourceType = SourceTypeGrpc.
	// If present, the address field is ignored.
	// +Optional
	Image string `json:"image,omitempty"`

	// Secrets represent set of secrets that can be used to access the contents of the catalog.
	// It is best to keep this list small, since each will need to be tried for every catalog entry.
	// +Optional
	Secrets []string `json:"secrets,omitempty"`

	// Metadata
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	Icon        Icon   `json:"icon,omitempty"`
}

type RegistryServiceStatus struct {
	Protocol         string      `json:"protocol,omitempty"`
	ServiceName      string      `json:"serviceName,omitempty"`
	ServiceNamespace string      `json:"serviceNamespace,omitempty"`
	Port             string      `json:"port,omitempty"`
	CreatedAt        metav1.Time `json:"createdAt,omitempty"`
}

func (s *RegistryServiceStatus) Address() string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:%s", s.ServiceName, s.ServiceNamespace, s.Port)
}

type CatalogSourceConditionType string

const (
	CatalogSourceAvailable CatalogSourceConditionType = "Available"
)

type CatalogSourceConditionReason string

const (
	CatalogSourceReasonInvalidType       CatalogSourceConditionReason = "InvalidSourceType"
	CatalogSourceReasonConfigMapNotFound CatalogSourceConditionReason = "ConfigMapNotFound"
	CatalogSourceReasonRegistryHealthy   CatalogSourceConditionReason = "RegistryHealthy"
)

type CatalogSourceCondition struct {
	Type               CatalogSourceConditionType   `json:"type,omitempty"`
	Status             corev1.ConditionStatus       `json:"status,omitempty"`
	LastUpdateTime     metav1.Time                  `json:"lastUpdateTime,omitempty"`
	LastTransitionTime metav1.Time                  `json:"lastTransitionTime,omitempty"`
	Reason             CatalogSourceConditionReason `json:"reason,omitempty"`
	Message            string                       `json:"message,omitempty"`
}

// SetCatalogSourceCondition adds or updates a condition, using `Type` as merge key
func (s *CatalogSourceStatus) SetCondition(cond CatalogSourceCondition) CatalogSourceCondition {
	updated := now()
	cond.LastUpdateTime = updated
	cond.LastTransitionTime = updated

	for i, existing := range s.Conditions {
		if existing.Type != cond.Type {
			continue
		}
		if existing.Status == cond.Status {
			cond.LastTransitionTime = existing.LastTransitionTime
		}
		s.Conditions[i] = cond
		return cond
	}
	s.Conditions = append(s.Conditions, cond)
	return cond
}

func CatalogSourceConditionFailed(cond CatalogSourceConditionType, reason CatalogSourceConditionReason, err error) CatalogSourceCondition {
	return CatalogSourceCondition{
		Type:    cond,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: err.Error(),
	}
}

func CatalogSourceConditionMet(cond CatalogSourceConditionType) CatalogSourceCondition {
	return CatalogSourceCondition{
		Type:   cond,
		Status: corev1.ConditionTrue,
	}
}

type CatalogSourceStatus struct {
	ConfigMapResource     *ConfigMapResourceReference `json:"configMapReference,omitempty"`
	RegistryServiceStatus *RegistryServiceStatus      `json:"registryService,omitempty"`
	Conditions            []CatalogSourceCondition    `json:"conditions,omitempty"`
	LastSync              metav1.Time                 `json:"lastSync,omitempty"`
}

type ConfigMapResourceReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`

	UID             types.UID `json:"uid,omitempty"`
	ResourceVersion string    `json:"resourceVersion,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
type CatalogSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   CatalogSourceSpec   `json:"spec"`
	Status CatalogSourceStatus `json:"status"`
}

func (c *CatalogSource) Address() string {
	if c.Spec.Address != "" {
		return c.Spec.Address
	}
	return c.Status.RegistryServiceStatus.Address()
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CatalogSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []CatalogSource `json:"items"`
}
