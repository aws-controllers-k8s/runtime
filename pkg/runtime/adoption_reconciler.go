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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	ctrlrt "sigs.k8s.io/controller-runtime"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	finalizerString = "finalizers.services.k8s.aws/AdoptedResource"
)

// adoptionReconciler is responsible for reconciling the state of any adopted resources
// of that match any Kubernetes custom resources (CRs) that support by a gievn
// AWS service.
// It implements the upstream controller-runtime `Reconciler` interface.
type adoptionReconciler struct {
	reconciler
}

// BindControllerManager sets up the AWSResourceReconciler with an instance
// of an upstream controller-runtime.Manager
func (r *adoptionReconciler) BindControllerManager(mgr ctrlrt.Manager) error {
	clusterConfig := mgr.GetConfig()
	clientset, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return err
	}
	r.kc = mgr.GetClient()
	r.cache = ackrtcache.New(clientset, r.log)
	r.cache.Run()
	return ctrlrt.NewControllerManagedBy(
		mgr,
	).For(
		// Read only adopted resource objects
		&ackv1alpha1.AdoptedResource{},
	).Complete(r)
}

// Reconcile implements `controller-runtime.Reconciler` and handles reconciling
// a CR CRUD request
func (r *adoptionReconciler) Reconcile(req ctrlrt.Request) (ctrlrt.Result, error) {
	return r.handleReconcileError(r.reconcile(req))
}

func (r *adoptionReconciler) reconcile(req ctrlrt.Request) error {
	ctx := context.Background()
	res, err := r.getAdoptedResource(ctx, req)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// resource wasn't found. just ignore these.
			return nil
		}
		return err
	}

	gk := r.getTargetResourceGroupKind(res)

	// Check if the target API group matches with the controller
	var controllerRMF acktypes.AWSResourceManagerFactory
	for _, v := range r.sc.GetResourceManagerFactories() {
		controllerRMF = v
		break
	}
	if gk.Group != controllerRMF.ResourceDescriptor().GroupKind().Group {
		ackrtlog.DebugAdoptedResource(r.log, res, "target resource API group is not of this service. no-op")
		return nil
	}

	// Look up the rmf for the given target resource GVK
	rmf, ok := (r.sc.GetResourceManagerFactories())[gk.String()]
	if !ok {
		// TODO(RedbackThomson): Is printing necessary? Some other controller
		// might be able to reconcile - printing would be verbose
		return ackerr.ResourceManagerFactoryNotFound
	}

	if !rmf.IsAdoptable() {
		// TODO(RedbackThomson): Place into terminal state + condition
		return ackerr.NotAdoptable
	}

	targetDescriptor := rmf.ResourceDescriptor()
	acctID := r.getOwnerAccountID(res)
	region := r.getRegion(res)
	roleARN := r.getRoleARN(acctID)

	sess, err := r.sc.NewSession(
		region, &r.cfg.EndpointURL, roleARN,
		targetDescriptor.EmptyRuntimeObject().GetObjectKind().GroupVersionKind(),
	)
	if err != nil {
		return err
	}

	ackrtlog.InfoAdoptedResource(r.log, res, "starting adoption reconciliation")

	rm, err := rmf.ManagerFor(
		r.cfg, r.log, r.metrics, r, sess, acctID, region,
	)
	if err != nil {
		return err
	}

	if res.DeletionTimestamp != nil {
		return r.cleanup(ctx, *res)
	}

	// Determine whether the reason is in a terminal state
	if r.isAdopted(ctx, res) {
		return nil
	}

	return r.sync(ctx, targetDescriptor, rm, res)
}

func (r *adoptionReconciler) sync(
	ctx context.Context,
	targetDescriptor acktypes.AWSResourceDescriptor,
	rm acktypes.AWSResourceManager,
	desired *ackv1alpha1.AdoptedResource,
) error {
	// Create empty resource with spec/status fields set for ReadOne
	readableResource := targetDescriptor.ResourceFromRuntimeObject(targetDescriptor.EmptyRuntimeObject())
	if err := readableResource.SetIdentifiers(desired.Spec.AWS); err != nil {
		return r.onError(ctx, desired, err)
	}

	described, err := rm.ReadOne(ctx, readableResource)
	if err != nil {
		return r.onError(ctx, desired, err)
	}

	rmo := described.RuntimeMetaObject()

	// Use values from ReadOne output by default
	targetMeta := &metav1.ObjectMeta{
		Labels:          rmo.GetLabels(),
		Annotations:     rmo.GetAnnotations(),
		Finalizers:      rmo.GetFinalizers(),
		OwnerReferences: rmo.GetOwnerReferences(),
		GenerateName:    rmo.GetGenerateName(),
	}

	desiredMetadata := desired.Spec.Kubernetes.Metadata

	// Attempt to use metadata values from the adopted resource target metadata
	if desiredMetadata != nil {
		if desiredMetadata.Name != "" {
			targetMeta.SetName(desiredMetadata.Name)
		}

		if desiredMetadata.Namespace != "" {
			targetMeta.SetNamespace(desiredMetadata.Namespace)
		}

		if len(desiredMetadata.Annotations) > 0 {
			targetMeta.SetAnnotations(desiredMetadata.Annotations)
		}

		if len(desiredMetadata.Labels) > 0 {
			targetMeta.SetLabels(desiredMetadata.Labels)
		}

		if len(desiredMetadata.OwnerReferences) > 0 {
			targetMeta.SetOwnerReferences(desiredMetadata.OwnerReferences)
		}

		if desiredMetadata.GenerateName != "" {
			targetMeta.SetGenerateName(desiredMetadata.GenerateName)
		}
	}

	// If name and namespace not are specified, use the ones from the adopted
	// resource directly.
	if targetMeta.Name == "" {
		targetMeta.SetName(desired.ObjectMeta.Name)
	}

	if targetMeta.Namespace == "" {
		targetMeta.SetNamespace(desired.ObjectMeta.Namespace)
	}

	described.SetObjectMeta(*targetMeta)
	targetDescriptor.MarkAdopted(described)

	if err := r.kc.Create(ctx, described.RuntimeObject()); err != nil {
		return r.onError(ctx, desired, err)
	}

	if err := r.markManaged(ctx, *desired); err != nil {
		return r.onError(ctx, desired, err)
	}

	if err := r.onSuccess(ctx, desired); err != nil {
		// Don't attempt to patch conditions again, directly return err
		return err
	}

	return nil
}

// cleanup ensures that the supplied AWSResource's backing API resource is
// destroyed along with all child dependent resources
func (r *adoptionReconciler) cleanup(
	ctx context.Context,
	current ackv1alpha1.AdoptedResource,
) error {
	if err := r.markUnmanaged(ctx, current); err != nil {
		return err
	}
	// Additional logic?
	return nil
}

// getAdoptedResource returns an AdoptedResource representing the requested Kubernetes
// namespaced object
func (r *adoptionReconciler) getAdoptedResource(
	ctx context.Context,
	req ctrlrt.Request,
) (*ackv1alpha1.AdoptedResource, error) {
	ro := &ackv1alpha1.AdoptedResource{}
	if err := r.kc.Get(ctx, req.NamespacedName, ro); err != nil {
		return nil, err
	}
	return ro, nil
}

// onError will patch the adopted resource with the given error and return the
// same error back
func (r *adoptionReconciler) onError(
	ctx context.Context,
	res *ackv1alpha1.AdoptedResource,
	err error,
) error {
	r.patchAdoptedCondition(ctx, res, err)
	return err
}

// onSuccess will patch the adopted resource with a adopted condition and
// return any errors that occurred while patching
func (r *adoptionReconciler) onSuccess(
	ctx context.Context,
	res *ackv1alpha1.AdoptedResource,
) error {
	return r.patchAdoptedCondition(ctx, res, nil)
}

// patchAdoptedCondition updates the adopted condition status of the adopted resource
func (r *adoptionReconciler) patchAdoptedCondition(
	ctx context.Context,
	res *ackv1alpha1.AdoptedResource,
	err error,
) error {
	ko := res.DeepCopy()

	// Adopted condition
	var adoptedCondition *ackv1alpha1.Condition = nil
	for _, condition := range ko.Status.Conditions {
		if condition.Type == ackv1alpha1.ConditionTypeAdopted {
			adoptedCondition = condition
			break
		}
	}

	if adoptedCondition == nil {
		adoptedCondition = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeAdopted,
		}
		ko.Status.Conditions = append(ko.Status.Conditions, adoptedCondition)
	}

	var errMessage string
	if err != nil {
		errMessage = err.Error()
		adoptedCondition.Status = corev1.ConditionFalse
		adoptedCondition.Message = &errMessage
	} else {
		adoptedCondition.Message = nil
		adoptedCondition.Status = corev1.ConditionTrue
	}

	return r.kc.Status().Patch(
		ctx,
		ko.DeepCopyObject(),
		client.MergeFrom(res),
	)
}

// isAdopted returns true if the AdoptedResource is in a terminal adoption state
func (r *adoptionReconciler) isAdopted(
	ctx context.Context,
	res *ackv1alpha1.AdoptedResource,
) bool {
	for _, condition := range res.Status.Conditions {
		if condition.Type == ackv1alpha1.ConditionTypeAdopted {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// getTargetResourceGroupKind returns the GroupKind as specified in the spec of
// the AdoptedResource object.
func (r *adoptionReconciler) getTargetResourceGroupKind(
	res *ackv1alpha1.AdoptedResource,
) schema.GroupKind {
	return schema.GroupKind{
		Group: *res.Spec.Kubernetes.Group,
		Kind:  *res.Spec.Kubernetes.Kind,
	}
}

// markManaged places the supplied resource under the management of ACK.
func (r *adoptionReconciler) markManaged(
	ctx context.Context,
	res ackv1alpha1.AdoptedResource,
) error {
	orig := res.DeepCopyObject()
	k8sctrlutil.AddFinalizer(&res.ObjectMeta, finalizerString)
	err := r.kc.Patch(
		ctx,
		res.DeepCopyObject(),
		client.MergeFrom(orig),
	)
	if err != nil {
		return err
	}
	return nil
}

// markUnmanaged removes the supplied resource from management by ACK.
func (r *adoptionReconciler) markUnmanaged(
	ctx context.Context,
	res ackv1alpha1.AdoptedResource,
) error {
	orig := res.DeepCopyObject()
	k8sctrlutil.RemoveFinalizer(&res.ObjectMeta, finalizerString)
	err := r.kc.Patch(
		ctx,
		res.DeepCopyObject(),
		client.MergeFrom(orig),
	)
	if err != nil {
		return err
	}
	return nil
}

// handleReconcileError will handle errors from reconcile handlers, which
// respects runtime errors.
func (r *adoptionReconciler) handleReconcileError(err error) (ctrlrt.Result, error) {
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

// getOwnerAccountID returns the AWS account that owns the supplied resource.
// The function looks to the common `Status.ACKResourceState` object, followed
// by the default AWS account ID associated with the Kubernetes Namespace in
// which the CR was created, followed by the AWS Account in which the IAM Role
// that the service controller is in.
func (r *adoptionReconciler) getOwnerAccountID(
	res *ackv1alpha1.AdoptedResource,
) ackv1alpha1.AWSAccountID {
	// look for owner account id in the namespace annotations
	namespace := res.GetNamespace()
	accID, ok := r.cache.Namespaces.GetOwnerAccountID(namespace)
	if ok {
		return ackv1alpha1.AWSAccountID(accID)
	}

	// use controller configuration
	return ackv1alpha1.AWSAccountID(r.cfg.AccountID)
}

// getRoleARN return the Role ARN that should be assumed in order to manage
// the resources.
func (r *adoptionReconciler) getRoleARN(
	acctID ackv1alpha1.AWSAccountID,
) ackv1alpha1.AWSResourceName {
	roleARN, _ := r.cache.Accounts.GetAccountRoleARN(string(acctID))
	return ackv1alpha1.AWSResourceName(roleARN)
}

// getRegion returns the AWS region that the given resource is in or should be
// created in. If the CR have a region associated with it, it is used. Otherwise
// we look for the namespace associated region, if that is set we use it. Finally
// if none of these annotations are set we use the use the region specified in the
// configuration is used
func (r *adoptionReconciler) getRegion(
	res *ackv1alpha1.AdoptedResource,
) ackv1alpha1.AWSRegion {
	// look for region in CR metadata annotations
	resAnnotations := res.GetAnnotations()
	region, ok := resAnnotations[ackv1alpha1.AnnotationRegion]
	if ok {
		return ackv1alpha1.AWSRegion(region)
	}

	// look for default region in namespace metadata annotations
	ns := res.GetNamespace()
	defaultRegion, ok := r.cache.Namespaces.GetDefaultRegion(ns)
	if ok {
		return ackv1alpha1.AWSRegion(defaultRegion)
	}

	// use controller configuration region
	return ackv1alpha1.AWSRegion(r.cfg.Region)
}

// NewAdoptionReconciler returns a new adoptionReconciler object
func NewAdoptionReconciler(
	sc acktypes.ServiceController,
	log logr.Logger,
	cfg ackcfg.Config,
	metrics *ackmetrics.Metrics,
) acktypes.Reconciler {
	return &adoptionReconciler{
		reconciler: reconciler{
			sc:      sc,
			log:     log.WithName("adopted-reconciler"),
			cfg:     cfg,
			metrics: metrics,
		},
	}
}
