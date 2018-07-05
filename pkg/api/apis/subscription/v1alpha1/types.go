package v1alpha1

import (
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/installplan/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	GroupVersion              = "v1alpha1" // version used in the Scheme for subscriptions
	SubscriptionKind          = "Subscription-v1"
	SubscriptionListKind      = "SubscriptionList-v1"
	SubscriptionCRDName       = "subscription-v1s"
	SubscriptionCRDAPIVersion = "app.coreos.com/v1alpha1" // API version w/ CRD support
)

// SubscriptionState tracks when updates are available, installing, or service is up to date
type SubscriptionState string

const (
	SubscriptionStateNone             = ""
	SubscriptionStateUpgradeAvailable = "UpgradeAvailable"
	SubscriptionStateUpgradePending   = "UpgradePending"
	SubscriptionStateAtLatest         = "AtLatestKnown"
)

// SubscriptionSpec defines an Application that can be installed
type SubscriptionSpec struct {
	CatalogSource       string            `json:"source"`
	Package             string            `json:"name"`
	Channel             string            `json:"channel,omitempty"`
	StartingCSV         string            `json:"startingCSV,omitempty"`
	InstallPlanApproval v1alpha1.Approval `json:"installPlanApproval,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
type Subscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   *SubscriptionSpec  `json:"spec"`
	Status SubscriptionStatus `json:"status"`
}

type SubscriptionStatus struct {
	// FIXME(alecmerdler): This field is too ambiguous about the state of the CSV
	CurrentCSV string                `json:"currentCSV,omitempty"`
	Install    *InstallPlanReference `json:"installplan,omitempty"`

	State       SubscriptionState `json:"state,omitempty"`
	LastUpdated metav1.Time       `json:"lastUpdated"`
}

type InstallPlanReference struct {
	APIVersion string    `json:"apiVersion"`
	Kind       string    `json:"kind"`
	Name       string    `json:"name"`
	UID        types.UID `json:"uuid"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Subscription `json:"items"`
}

// GetInstallPlanApproval gets the configured install plan approval or the default
func (s *Subscription) GetInstallPlanApproval() v1alpha1.Approval {
	if s.Spec.InstallPlanApproval == v1alpha1.ApprovalManual {
		return v1alpha1.ApprovalManual
	}
	return v1alpha1.ApprovalAutomatic
}
