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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// A resource that a selector maps to another account is reconciled before the
// IAMRoleSelector cache has synced. Matching against the empty cache would use
// the controller's own role, account and region, so the reconcile is requeued
// without any AWS call or status change, and the next one, after the sync,
// uses the selector's role.
func TestReconcile_IAMRoleSelectorCacheNotSynced(t *testing.T) {
	const (
		controllerAccount = "111111111111"
		teamAccount       = "222222222222"
		teamRoleARN       = "arn:aws:iam::222222222222:role/team-a"
	)
	logger := ctrlrtzap.New(ctrlrtzap.UseFlagOptions(&ctrlrtzap.Options{Development: true, Level: zapcore.InfoLevel}))

	// An IAMRoleSelector sends every resource in namespace team-a to the team
	// account. The API server answers the selector LIST only when released.
	selector := &ackv1alpha1.IAMRoleSelector{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a"},
		Spec: ackv1alpha1.IAMRoleSelectorSpec{
			ARN:               teamRoleARN,
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
	nsClient.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		<-release
		return false, nil, nil
	})

	irsCache := iamroleselector.NewCache(logger)
	stop := make(chan struct{})
	defer close(stop)
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
	var createdIn []string
	newRM := func(acct ackv1alpha1.AWSAccountID) *ackmocks.AWSResourceManager {
		rm := &ackmocks.AWSResourceManager{}
		rm.On("ResolveReferences", mock.Anything, mock.Anything, mock.Anything).Return(desired, false, nil)
		rm.On("EnsureTags", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		rm.On("ReadOne", mock.Anything, mock.Anything).Return(nil, ackerr.NotFound).Once()
		rm.On("Create", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
			createdIn = append(createdIn, string(acct))
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
	type call struct {
		account ackv1alpha1.AWSAccountID
		region  ackv1alpha1.AWSRegion
		role    ackv1alpha1.AWSResourceName
	}
	var calls []call
	rmf := &ackmocks.AWSResourceManagerFactory{}
	rmf.On("ResourceDescriptor").Return(rd)
	rmf.On("RequeueOnSuccessSeconds").Return(0)
	rmf.On("ManagerFor", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything,
	).Return(func(_ ackcfg.Config, _ aws.Config, _ logr.Logger, _ *ackmetrics.Metrics, _ acktypes.Reconciler,
		acct ackv1alpha1.AWSAccountID, region ackv1alpha1.AWSRegion, role ackv1alpha1.AWSResourceName,
	) (acktypes.AWSResourceManager, error) {
		calls = append(calls, call{acct, region, role})
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
				AccountID:       controllerAccount,
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
	req := ctrlrt.Request{NamespacedName: types.NamespacedName{Namespace: "team-a", Name: "mybook"}}

	// 1. The cache has not synced: requeue, no AWS call, no status patch.
	require.False(t, irsCache.HasSynced())
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ctrlrt.Result{RequeueAfter: roleARNNotAvailableRequeueDelay}, res)
	assert.Empty(t, calls, "no resource manager before the cache has synced")
	sc.AssertNotCalled(t, "NewAWSConfig", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	sw.AssertNotCalled(t, "Patch", mock.Anything, mock.Anything, mock.Anything)

	// 2. The LISTs complete and the cache syncs: the resource is reconciled
	// with the selector's role, in the team account and the namespace's
	// default region.
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.True(t, irsCache.WaitForCacheSync(ctx))
	_, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, call{teamAccount, "eu-central-1", teamRoleARN}, calls[0])
	assert.Equal(t, []string{teamAccount}, createdIn)
}
