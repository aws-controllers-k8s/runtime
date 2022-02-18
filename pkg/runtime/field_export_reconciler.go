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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

const (
	fieldExportFinalizerString = "finalizers.services.k8s.aws/FieldExport"
)

// fieldExportReconciler is responsible for reconciling the state of any field
// export CRs that target resources supported by the current controller.
// It implements the upstream controller-runtime `Reconciler` interface.
type fieldExportReconciler struct {
	reconciler
}

// BindControllerManager sets up the AWSResourceReconciler with an instance
// of an upstream controller-runtime.Manager
func (r *fieldExportReconciler) BindControllerManager(mgr ctrlrt.Manager) error {
	r.kc = mgr.GetClient()
	r.apiReader = mgr.GetAPIReader()
	return ctrlrt.NewControllerManagedBy(
		mgr,
	).For(
		// Read only field export objects
		&ackv1alpha1.FieldExport{},
	).WithEventFilter(
		predicate.GenerationChangedPredicate{},
	).Complete(r)
}

// Reconcile implements `controller-runtime.Reconciler` and handles reconciling
// a CR CRUD request
func (r *fieldExportReconciler) Reconcile(ctx context.Context, req ctrlrt.Request) (ctrlrt.Result, error) {
	return r.handleReconcileError(r.reconcile(ctx, req))
}

func (r *fieldExportReconciler) reconcile(ctx context.Context, req ctrlrt.Request) error {
	return nil
}

func (r *fieldExportReconciler) Sync(
	ctx context.Context,
	targetDescriptor acktypes.AWSResourceDescriptor,
	rm acktypes.AWSResourceManager,
	desired *ackv1alpha1.FieldExport,
) error {
	if err := r.markManaged(ctx, desired); err != nil {
		return r.onError(ctx, desired, err)
	}

	// Don't attempt to patch conditions again, directly return result of
	// 'r.onSuccess'
	return r.onSuccess(ctx, desired)
}

// cleanup removes the finalizer from AdoptedResource so that k8s object can
// be deleted.
func (r *fieldExportReconciler) cleanup(
	ctx context.Context,
	current *ackv1alpha1.FieldExport,
) error {
	if err := r.markUnmanaged(ctx, current); err != nil {
		return err
	}
	// Additional logic?
	return nil
}

// getAdoptedResource returns an AdoptedResource representing the requested Kubernetes
// namespaced object
func (r *fieldExportReconciler) getAdoptedResource(
	ctx context.Context,
	req ctrlrt.Request,
) (*ackv1alpha1.FieldExport, error) {
	ro := &ackv1alpha1.FieldExport{}
	// Here we use k8s APIReader to read the k8s object by making the
	// direct call to k8s apiserver instead of using k8sClient.
	// The reason is that k8sClient uses a cache and sometimes k8sClient can
	// return stale copy of object.
	// It is okay to make direct call to k8s apiserver because we are only
	// making single read call for complete reconciler loop.
	// See following issue for more details:
	// https://github.com/aws-controllers-k8s/community/issues/894
	if err := r.apiReader.Get(ctx, req.NamespacedName, ro); err != nil {
		return nil, err
	}
	return ro, nil
}

// onError will patch the adopted resource with the given error and return the
// same error back
func (r *fieldExportReconciler) onError(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
	err error,
) error {
	r.patchRecoverableCondition(ctx, res, err)
	return err
}

// onSuccess will patch the adopted resource with a adopted condition and
// return any errors that occurred while patching
func (r *fieldExportReconciler) onSuccess(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
) error {
	return nil
}

// patchRecoverableCondition updates the recoverable condition status of the
// field export CR. The resource passed in the parameter gets updated with the
// conditions
func (r *fieldExportReconciler) patchRecoverableCondition(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
	err error,
) error {
	base := res.DeepCopy()

	// Recoverable condition
	var recoverableCondition *ackv1alpha1.Condition = nil
	for _, condition := range res.Status.Conditions {
		if condition.Type == ackv1alpha1.ConditionTypeRecoverable {
			recoverableCondition = condition
			break
		}
	}

	if recoverableCondition == nil {
		recoverableCondition = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeRecoverable,
		}
		res.Status.Conditions = append(res.Status.Conditions, recoverableCondition)
	}

	var errMessage string
	if err != nil {
		errMessage = err.Error()
		recoverableCondition.Status = corev1.ConditionFalse
		recoverableCondition.Message = &errMessage
	} else {
		recoverableCondition.Message = nil
		recoverableCondition.Status = corev1.ConditionTrue
	}

	return r.patchStatus(ctx, res, base)
}

// patchStatus patches the Status for FieldExport into k8s. The field export
// 'res' also gets updated with the content returned from apiserver.
// TODO(vijtrip2): Refactor this and use single 'patchStatus' method
// for all reconcilers
func (r *fieldExportReconciler) patchStatus(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
	base *ackv1alpha1.FieldExport,
) error {
	return r.kc.Status().Patch(
		ctx,
		res,
		client.MergeFrom(base),
	)
}

// markManaged places the supplied resource under the management of ACK.
// It adds the finalizer string, patches the object in etcd and updates
// the object 'res' in parameter with latest metadata.
func (r *fieldExportReconciler) markManaged(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
) error {
	base := res.DeepCopy()
	k8sctrlutil.AddFinalizer(res, fieldExportFinalizerString)
	return r.patchMetadataAndSpec(ctx, res, base)
}

// markUnmanaged removes the supplied resource from management by ACK.
// It removes the finalizer string, patches the object in etcd and updates
// the object 'res' in parameter with latest metadata.
func (r *fieldExportReconciler) markUnmanaged(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
) error {
	base := res.DeepCopy()
	k8sctrlutil.RemoveFinalizer(res, fieldExportFinalizerString)
	return r.patchMetadataAndSpec(ctx, res, base)
}

// patchMetadataAndSpec patches the Metadata and Spec for FieldExport into
// k8s. The field export 'res' also gets updated with content returned from
// apiserver.
// TODO(vijtrip2@): Refactor this and use single 'patchMetadataAndSpec' method
// for all reconcilers
func (r *fieldExportReconciler) patchMetadataAndSpec(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
	base *ackv1alpha1.FieldExport,
) error {
	// k8s Client Patch call updates the status of original object with the
	// content returned from apiserver.
	// Keep a copy of status field to reset the status of 'res' after patch call
	resStatusCopy := res.DeepCopy().Status
	err := r.kc.Patch(
		ctx,
		res,
		client.MergeFrom(base),
	)
	res.Status = resStatusCopy
	return err
}

// handleReconcileError will handle errors from reconcile handlers, which
// respects runtime errors.
func (r *fieldExportReconciler) handleReconcileError(err error) (ctrlrt.Result, error) {
	if err == nil || err == ackerr.Terminal {
		return ctrlrt.Result{}, nil
	}

	var requeueNeededAfter *requeue.RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		after := requeueNeededAfter.Duration()
		r.log.V(1).Info(
			"requeue needed after error",
			"error", requeueNeededAfter.Unwrap(),
			"after", after,
		)
		return ctrlrt.Result{RequeueAfter: after}, nil
	}

	var requeueNeeded *requeue.RequeueNeeded
	if errors.As(err, &requeueNeeded) {
		r.log.V(1).Info(
			"requeue needed error",
			"error", requeueNeeded.Unwrap(),
		)
		return ctrlrt.Result{Requeue: true}, nil
	}

	return ctrlrt.Result{}, err
}

// NewFieldExportReconciler returns a new FieldExportReconciler object
func NewFieldExportReconciler(
	sc acktypes.ServiceController,
	log logr.Logger,
	cfg ackcfg.Config,
	metrics *ackmetrics.Metrics,
	cache ackrtcache.Caches,
) acktypes.Reconciler {
	return NewFieldExportReconcilerWithClient(sc, log, cfg, metrics, cache, nil, nil)
}

// NewFieldExportReconcilerWithClient returns a new FieldExportReconciler object with
// specified k8s client and Reader. Currently this function is used for testing
// purpose only because "FieldExportReconciler" struct is not available outside
// 'runtime' package for dependency injection.
func NewFieldExportReconcilerWithClient(
	sc acktypes.ServiceController,
	log logr.Logger,
	cfg ackcfg.Config,
	metrics *ackmetrics.Metrics,
	cache ackrtcache.Caches,
	kc client.Client,
	apiReader client.Reader,
) acktypes.FieldExportReconciler {
	return &fieldExportReconciler{
		reconciler: reconciler{
			sc:        sc,
			log:       log.WithName("field-export-reconciler"),
			cfg:       cfg,
			metrics:   metrics,
			cache:     cache,
			kc:        kc,
			apiReader: apiReader,
		},
	}
}
