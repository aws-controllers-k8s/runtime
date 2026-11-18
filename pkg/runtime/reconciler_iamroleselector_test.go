// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	ctrlrt "sigs.k8s.io/controller-runtime"
	ctrlrtzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ctrlrtclientmock "github.com/aws-controllers-k8s/runtime/mocks/controller-runtime/pkg/client"
	ackmocks "github.com/aws-controllers-k8s/runtime/mocks/pkg/types"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	"github.com/aws-controllers-k8s/runtime/pkg/runtime/iamroleselector"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

const (
	irsControllerAccount = "111111111111"
	irsTeamAccount       = "222222222222"
	irsTeamRoleARN       = "arn:aws:iam::222222222222:role/team-a"
)

// managerForCall records the account, region and role a resource manager was
// requested for.
type managerForCall struct {
	account ackv1alpha1.AWSAccountID
	region  ackv1alpha1.AWSRegion
	role    ackv1alpha1.AWSResourceName
}

// irsFixture is a reconciler with the IAMRoleSelector feature gate on, in
// account 111111111111 and region us-west-2, and a real IAMRoleSelector cache
// on fake clients. An IAMRoleSelector maps namespace team-a, whose default
// region is eu-central-1, to a role in account 222222222222. The fake API
// server answers the LISTs of IAMRoleSelectors and namespaces only once
// release is closed.
type irsFixture struct {
	r         *resourceReconciler
	req       ctrlrt.Request
	dyn       *dynamicfake.FakeDynamicClient
	irsCache  *iamroleselector.Cache
	release   chan struct{}
	sc        *ackmocks.ServiceController
	sw        *ctrlrtclientmock.SubResourceWriter
	calls     []managerForCall
	createdIn []string
}

// newIRSFixture builds an irsFixture. crdInstalled says whether the API
// server serves IAMRoleSelectors.
func newIRSFixture(t *testing.T, crdInstalled bool) *irsFixture {
	logger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&ctrlrtzap.Options{Development: true, Level: zapcore.InfoLevel}))

	// An IAMRoleSelector sends every resource in namespace team-a to the team
	// account. The API server answers the selector LIST only when released.
	selector := &ackv1alpha1.IAMRoleSelector{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a"},
		Spec: ackv1alpha1.IAMRoleSelectorSpec{
			ARN:               irsTeamRoleARN,
			NamespaceSelector: ackv1alpha1.NamespaceSelector{Names: []string{"team-a"}},
		},
	}
	obj, _ := k8sruntime.DefaultUnstructuredConverter.ToUnstructured(selector)
	u := &unstructured.Unstructured{Object: obj}
	u.SetAPIVersion("services.k8s.aws/v1alpha1")
	u.SetKind("IAMRoleSelector")
	gvr := schema.GroupVersionResource{Group: "services.k8s.aws", Version: "v1alpha1", Resource: "iamroleselectors"}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		k8sruntime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "IAMRoleSelectorList"}, u,
	)
	release := make(chan struct{})
	dyn.PrependReactor("list", "iamroleselectors", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		<-release // a slow or failing LIST
		return false, nil, nil
	})
	nsClient := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:        "team-a",
		Annotations: map[string]string{ackv1alpha1.AnnotationDefaultRegion: "eu-central-1"},
	}})
	if crdInstalled {
		nsClient.Resources = []*metav1.APIResourceList{{
			GroupVersion: gvr.GroupVersion().String(),
			APIResources: []metav1.APIResource{{Name: gvr.Resource, Kind: "IAMRoleSelector"}},
		}}
	} else {
		// Without the CRD, the API server answers 404 to IAMRoleSelectors.
		notFound := apierrors.NewNotFound(gvr.GroupResource(), "")
		dyn.PrependReactor("list", "iamroleselectors", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, notFound
		})
		dyn.PrependWatchReactor("iamroleselectors", func(k8stesting.Action) (bool, watch.Interface, error) {
			return true, nil, notFound
		})
	}
	nsClient.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		<-release
		return false, nil, nil
	})

	irsCache := iamroleselector.NewCache(logger)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	irsCache.Run(dyn, nsClient, stop)

	// The CR: a Book in namespace team-a. Its status gets the owner account
	// once it is created.
	cr := &unstructured.Unstructured{}
	cr.SetAPIVersion("bookstore.services.k8s.aws/v1alpha1")
	cr.SetKind("Book")
	cr.SetNamespace("team-a")
	cr.SetName("mybook")
	var ownerAccount *ackv1alpha1.AWSAccountID
	ids := &ackmocks.AWSResourceIdentifiers{}
	ids.On("Region").Return(nil)
	ids.On("OwnerAccountID").Return(func() *ackv1alpha1.AWSAccountID { return ownerAccount })
	ids.On("ARN").Return(nil)
	desired := &ackmocks.AWSResource{}
	desired.On("MetaObject").Return(cr)
	desired.On("RuntimeObject").Return(cr)
	desired.On("DeepCopy").Return(desired)
	desired.On("Identifiers").Return(ids)
	desired.On("IsBeingDeleted").Return(false)
	desired.On("Conditions").Return([]*ackv1alpha1.Condition{})
	desired.On("ReplaceConditions", mock.Anything).Return()
	desired.On("SetStatus", mock.Anything).Return()

	rd := &ackmocks.AWSResourceDescriptor{}
	rd.On("GroupVersionKind").Return(cr.GroupVersionKind())
	rd.On("EmptyRuntimeObject").Return(&fakeBook{})
	rd.On("ResourceFromRuntimeObject", mock.Anything).Return(desired)
	rd.On("IsManaged", mock.Anything).Return(true)
	rd.On("Delta", mock.Anything, mock.Anything).Return(ackcompare.NewDelta())

	// The resource manager the reconciler gets is bound to an account and a
	// role. Create records where the resource lands.
	f := &irsFixture{dyn: dyn, irsCache: irsCache, release: release}
	newRM := func(acct ackv1alpha1.AWSAccountID) *ackmocks.AWSResourceManager {
		rm := &ackmocks.AWSResourceManager{}
		rm.On("ResolveReferences", mock.Anything, mock.Anything, mock.Anything).Return(desired, false, nil)
		rm.On("EnsureTags", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		rm.On("ReadOne", mock.Anything, mock.Anything).Return(nil, ackerr.NotFound).Once()
		rm.On("Create", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
			f.createdIn = append(f.createdIn, string(acct))
			a := acct
			ownerAccount = &a // the generated manager sets status.ackResourceMetadata.ownerAccountID
		}).Return(desired, nil)
		rm.On("ReadOne", mock.Anything, mock.Anything).Return(desired, nil)
		rm.On("ClearResolvedReferences", mock.Anything).Return(func(r acktypes.AWSResource) acktypes.AWSResource { return r })
		rm.On("LateInitialize", mock.Anything, mock.Anything).Return(desired, nil)
		rm.On("IsSynced", mock.Anything, mock.Anything).Return(true, nil)
		rm.On("FilterSystemTags", mock.Anything, mock.Anything)
		return rm
	}
	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)
	rmf.On("ManagerFor", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything,
	).Return(func(_ ackcfg.Config, _ aws.Config, _ logr.Logger, _ *ackmetrics.Metrics, _ acktypes.Reconciler,
		acct ackv1alpha1.AWSAccountID, region ackv1alpha1.AWSRegion, role ackv1alpha1.AWSResourceName,
	) (acktypes.AWSResourceManager, error) {
		f.calls = append(f.calls, managerForCall{acct, region, role})
		return newRM(acct), nil
	})

	sc := &ackmocks.ServiceController{}
	sc.On("GetMetadata").Return(acktypes.ServiceControllerMetadata{})
	sc.On("NewAWSConfig", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(aws.Config{}, nil)
	kc := &ctrlrtclientmock.Client{}
	kc.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	sw := &ctrlrtclientmock.SubResourceWriter{}
	kc.On("Status").Return(sw)
	sw.On("Patch", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	apiReader := &ctrlrtclientmock.Reader{}
	apiReader.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	r := &resourceReconciler{
		reconciler: reconciler{
			kc: kc, apiReader: apiReader, sc: sc, log: logger,
			cfg: ackcfg.Config{
				AccountID:       irsControllerAccount,
				Region:          "us-west-2",
				ResourceTagKeys: []string{},
				FeatureGates:    featuregate.FeatureGates{featuregate.IAMRoleSelector: {Enabled: true}},
			},
			carmCache: ackrtcache.New(logger, ackrtcache.Config{}, featuregate.FeatureGates{}),
			irsCache:  irsCache,
			metrics:   ackmetrics.NewMetrics("bookstore"),
		},
		rmf: rmf,
		rd:  rd,
	}
	f.r, f.sc, f.sw = r, sc, sw
	f.req = ctrlrt.Request{NamespacedName: types.NamespacedName{Namespace: "team-a", Name: "mybook"}}
	return f

}

// sync releases the LISTs and waits for the IAMRoleSelector cache to sync.
func (f *irsFixture) sync(t *testing.T) {
	close(f.release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.True(t, f.irsCache.WaitForCacheSync(ctx))
}

// A resource that a selector maps to another account is reconciled before the
// IAMRoleSelector cache has synced. Matching against the empty cache would use
// the controller's own role, account and region, so the reconcile is requeued
// without any AWS call or status change, and the next one, after the sync,
// uses the selector's role.
func TestReconcile_IAMRoleSelectorCacheNotSynced(t *testing.T) {
	f := newIRSFixture(t, true)

	// 1. The cache has not synced: requeue, no AWS call, no status patch.
	require.False(t, f.irsCache.HasSynced())
	res, err := f.r.Reconcile(context.Background(), f.req)
	require.NoError(t, err)
	assert.Equal(t, ctrlrt.Result{RequeueAfter: roleARNNotAvailableRequeueDelay}, res)
	assert.Empty(t, f.calls, "no resource manager before the cache has synced")
	f.sc.AssertNotCalled(t, "NewAWSConfig", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.sw.AssertNotCalled(t, "Patch", mock.Anything, mock.Anything, mock.Anything)

	// 2. The LISTs complete and the cache syncs: the resource is reconciled
	// with the selector's role, in the team account and the namespace's
	// default region.
	f.sync(t)
	_, err = f.r.Reconcile(context.Background(), f.req)
	require.NoError(t, err)
	require.Len(t, f.calls, 1)
	assert.Equal(t, managerForCall{irsTeamAccount, "eu-central-1", irsTeamRoleARN}, f.calls[0])
	assert.Equal(t, []string{irsTeamAccount}, f.createdIn)
}

// The feature gate is on but the IAMRoleSelector CRD is not installed, as after
// a `helm upgrade` from a chart that did not have it. There can be no
// selectors: the cache syncs with the namespaces alone, although the LIST of
// IAMRoleSelectors fails, and the resource is reconciled with the controller's
// role, in the namespace's default region, as before.
func TestReconcile_IAMRoleSelectorCRDNotInstalled(t *testing.T) {
	f := newIRSFixture(t, false)
	f.sync(t)

	_, err := f.r.Reconcile(context.Background(), f.req)
	require.NoError(t, err)
	require.Len(t, f.calls, 1)
	assert.Equal(t, managerForCall{irsControllerAccount, "eu-central-1", ""}, f.calls[0])
	assert.Equal(t, []string{irsControllerAccount}, f.createdIn)
}
