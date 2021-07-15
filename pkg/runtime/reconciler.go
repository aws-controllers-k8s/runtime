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
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

// reconciler describes a generic reconciler within ACK.
type reconciler struct {
	sc      acktypes.ServiceController
	kc      client.Client
	log     logr.Logger
	cfg     ackcfg.Config
	cache   ackrtcache.Caches
	metrics *ackmetrics.Metrics
}

// resourceReconciler is responsible for reconciling the state of a SINGLE KIND of
// Kubernetes custom resources (CRs) that represent AWS service API resources.
// It implements the upstream controller-runtime `Reconciler` interface.
//
// The upstream controller-runtime.Manager object ends up managing MULTIPLE
// controller-runtime.Controller objects (each containing a single resourceReconciler
// object)s and sharing watch and informer queues across those controllers.
type resourceReconciler struct {
	reconciler
	rmf acktypes.AWSResourceManagerFactory
	rd  acktypes.AWSResourceDescriptor
}

// GroupKind returns the string containing the API group and kind reconciled by
// this reconciler
func (r *resourceReconciler) GroupKind() *metav1.GroupKind {
	if r.rd == nil {
		return nil
	}
	return r.rd.GroupKind()
}

// BindControllerManager sets up the AWSResourceReconciler with an instance
// of an upstream controller-runtime.Manager
func (r *resourceReconciler) BindControllerManager(mgr ctrlrt.Manager) error {
	if r.rmf == nil {
		return ackerr.NilResourceManagerFactory
	}
	clusterConfig := mgr.GetConfig()
	clientset, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return err
	}
	r.kc = mgr.GetClient()
	r.cache = ackrtcache.New(clientset, r.log, r.cfg.WatchNamespace)
	r.cache.Run()
	rd := r.rmf.ResourceDescriptor()
	return ctrlrt.NewControllerManagedBy(
		mgr,
	).For(
		rd.EmptyRuntimeObject(),
	).Complete(r)
}

// SecretValueFromReference fetches the value of a Secret given a
// SecretKeyReference.
func (r *reconciler) SecretValueFromReference(
	ctx context.Context,
	ref *ackv1alpha1.SecretKeyReference,
) (string, error) {

	if ref == nil {
		return "", nil
	}

	namespace := ref.Namespace
	if namespace == "" {
		namespace = "default"
	}

	nsn := client.ObjectKey{
		Namespace: namespace,
		Name:      ref.Name,
	}
	var secret corev1.Secret
	if err := r.kc.Get(ctx, nsn, &secret); err != nil {
		return "", err
	}

	// Currently we have only Opaque secrets in scope.
	if secret.Type != corev1.SecretTypeOpaque {
		return "", ackerr.SecretTypeNotSupported
	}

	if value, ok := secret.Data[ref.Key]; ok {
		valuestr := string(value)
		return valuestr, nil
	}

	return "", ackerr.NotFound
}

// Reconcile implements `controller-runtime.Reconciler` and handles reconciling
// a CR CRUD request
func (r *resourceReconciler) Reconcile(req ctrlrt.Request) (ctrlrt.Result, error) {
	ctx := context.Background()
	res, err := r.getAWSResource(ctx, req)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// resource wasn't found. just ignore these.
			return ctrlrt.Result{}, nil
		}
		return ctrlrt.Result{}, err
	}

	acctID := r.getOwnerAccountID(res)
	region := r.getRegion(res)
	roleARN := r.getRoleARN(acctID)
	sess, err := r.sc.NewSession(
		region, &r.cfg.EndpointURL, roleARN,
		res.RuntimeObject().GetObjectKind().GroupVersionKind(),
	)
	if err != nil {
		return ctrlrt.Result{}, err
	}

	rlog := ackrtlog.NewResourceLogger(
		r.log, res,
		"account", acctID,
		"role", roleARN,
		"region", region,
	)
	ctx = context.WithValue(ctx, ackrtlog.ContextKey, rlog)

	rm, err := r.rmf.ManagerFor(
		r.cfg, r.log, r.metrics, r, sess, acctID, region,
	)
	if err != nil {
		return ctrlrt.Result{}, err
	}
	return r.handleReconcileError(ctx, r.reconcile(ctx, rm, res))
}

func (r *resourceReconciler) reconcile(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	res acktypes.AWSResource,
) error {
	if res.IsBeingDeleted() {
		return r.cleanup(ctx, rm, res)
	}

	return r.Sync(ctx, rm, res)
}

// Sync ensures that the supplied AWSResource's backing API resource
// matches the supplied desired state
func (r *resourceReconciler) Sync(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	desired acktypes.AWSResource,
) error {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.Sync")
	defer exit(err)

	var latest acktypes.AWSResource // the newly created or mutated resource

	isAdopted := IsAdopted(desired)
	rlog.WithValues("is_adopted", isAdopted)

	// TODO(jaypipes): Validate all dependent resources. The AWSResource
	// interface needs to get some methods that return schema relationships,
	// first though

	rlog.Enter("rm.ReadOne")
	latest, err = rm.ReadOne(ctx, desired)
	rlog.Exit("rm.ReadOne", err)
	if err != nil {
		if err != ackerr.NotFound {
			if latest != nil {
				// this indicates, that even though ReadOne failed
				// there is some changes available in the latest.RuntimeObject()
				// (example: ko.Status.Conditions) which have been
				// updated in the resource
				// Thus, patchResourceStatus() call should be made here
				_ = r.patchResourceStatus(ctx, desired, latest)
			}
			return err
		}
		if isAdopted {
			return ackerr.AdoptedResourceNotFound
		}
		// Before we create the backend AWS service resources, let's first mark
		// the CR as being managed by ACK. Internally, this means adding a
		// finalizer to the CR; a finalizer that is removed once ACK no longer
		// manages the resource OR if the backend AWS service resource is
		// properly deleted.
		if err = r.setResourceManaged(ctx, desired); err != nil {
			return err
		}

		rlog.Enter("rm.Create")
		latest, err = rm.Create(ctx, desired)
		rlog.Exit("rm.Create", err)
		if err != nil {
			if latest != nil {
				// this indicates, that even though Create failed
				// there is some changes available in the latest.RuntimeObject()
				// (example: ko.Status.Conditions) which have been
				// updated in the resource
				// Thus, patchResourceStatus() call should be made here
				_ = r.patchResourceStatus(ctx, desired, latest)
			}
			return err
		}
		rlog.Info("created new resource")
	} else {
		// Ensure the resource is always managed (adopted resources apply)
		if err = r.setResourceManaged(ctx, desired); err != nil {
			return err
		}

		// Check to see if the latest observed state already matches the
		// desired state and if not, update the resource
		delta := r.rd.Delta(desired, latest)
		if delta.DifferentAt("Spec") {
			rlog.Info(
				"desired resource state has changed",
				"diff", delta.Differences,
			)
			rlog.Enter("rm.Update")
			latest, err = rm.Update(ctx, desired, latest, delta)
			rlog.Exit("rm.Update", err, "latest", latest)
			if err != nil {
				if latest != nil {
					// this indicates, that even though update failed
					// there is some changes available in the latest.RuntimeObject()
					// (example: ko.Status.Conditions) which have been
					// updated in the resource
					// Thus, patchResourceStatus() call should be made here
					_ = r.patchResourceStatus(ctx, desired, latest)
				}
				return err
			}
			rlog.Info("updated resource")
		}
	}
	if ackcompare.IsNotNil(latest) {
		err = r.patchResource(ctx, desired, latest)
		if err != nil {
			return err
		}
		for _, condition := range latest.Conditions() {
			if condition.Type != ackv1alpha1.ConditionTypeResourceSynced {
				continue
			}
			// The code below only executes for "ConditionTypeResourceSynced"
			if condition.Status == corev1.ConditionTrue {
				if duration := r.rmf.RequeueOnSuccessSeconds(); duration > 0 {
					rlog.Debug(
						"requeueing resource after resource synced condition true",
					)
					return requeue.NeededAfter(nil, time.Duration(duration)*time.Second)
				}
			} else {
				rlog.Debug(
					"requeueing resource after finding resource synced condition false",
				)
				return requeue.NeededAfter(
					ackerr.TemporaryOutOfSync, requeue.DefaultRequeueAfterDuration)
			}
		}
	}
	return nil
}

// patchResource patches the custom resource in the Kubernetes API to match the
// supplied latest resource's "metadata", "spec" and "status"
// To only patch the metadata and spec, use "patchResourceMetadataAndSpec".
// To only patch the status, use "patchResourceStatus"
func (r *resourceReconciler) patchResource(
	ctx context.Context,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) error {
	err := r.patchResourceMetadataAndSpec(ctx, desired, latest)
	if err != nil {
		return err
	}

	err = r.patchResourceStatus(ctx, desired, latest)
	if err != nil {
		return err
	}
	return nil
}

// patchResourceMetadataAndSpec patches the custom resource in the Kubernetes API to match the
// supplied latest resource's metadata and spec.
func (r *resourceReconciler) patchResourceMetadataAndSpec(
	ctx context.Context,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) error {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.patchResourceMetadataAndSpec")
	defer exit(err)

	equalMetadata, err := ackcompare.MetaV1ObjectEqual(desired.MetaObject(), latest.MetaObject())
	if err != nil {
		return err
	}
	if equalMetadata && !r.rd.Delta(desired, latest).DifferentAt("Spec") {
		rlog.Debug("no difference found between metadata and spec for desired and latest object.")
		return nil
	}

	rlog.Enter("kc.Patch (metadata + spec)")
	err = r.kc.Patch(
		ctx,
		latest.RuntimeObject(),
		client.MergeFrom(desired.RuntimeObject()),
	)
	rlog.Exit("kc.Patch (metadata + spec)", err)
	if err != nil {
		return err
	}
	rlog.Debug("patched resource metadata and spec", "latest", latest)
	return nil
}

// patchResourceStatus patches the custom resource in the Kubernetes API to match the
// supplied latest resource.
func (r *resourceReconciler) patchResourceStatus(
	ctx context.Context,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) error {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.patchResourceStatus")
	defer exit(err)

	changedStatus, err := r.rd.UpdateCRStatus(latest)
	if err != nil {
		return err
	}
	if !changedStatus {
		return nil
	}
	rlog.Enter("kc.Patch (status)")
	err = r.kc.Status().Patch(
		ctx,
		latest.RuntimeObject().DeepCopyObject(),
		client.MergeFrom(desired.RuntimeObject()),
	)
	rlog.Exit("kc.Patch (status)", err)
	if err != nil {
		return err
	}
	rlog.Debug("patched resource status", "latest", latest)
	return nil
}

// cleanup ensures that the supplied AWSResource's backing API resource is
// destroyed along with all child dependent resources
func (r *resourceReconciler) cleanup(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	current acktypes.AWSResource,
) error {
	// TODO(jaypipes): Handle all dependent resources. The AWSResource
	// interface needs to get some methods that return schema relationships,
	// first though
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.cleanup")
	defer exit(err)

	rlog.Enter("rm.ReadOne")
	observed, err := rm.ReadOne(ctx, current)
	rlog.Exit("rm.ReadOne", err)
	if err != nil {
		if err == ackerr.NotFound {
			// If the aws resource is not found, remove finalizer
			return r.setResourceUnmanaged(ctx, current)
		}
		return err
	}
	rlog.Enter("rm.Delete")
	latest, err := rm.Delete(ctx, observed)
	rlog.Exit("rm.Delete", err)
	if ackcompare.IsNotNil(latest) {
		// The Delete operation is likely asynchronous and has likely set a Status
		// field on the returned CR to something like `deleting`. Here, we patchResource()
		// in order to save these Status field modifications.
		_ = r.patchResource(ctx, current, latest)
	}
	if err != nil {
		// NOTE: Delete() implementations that have asynchronously-completing
		// deletions should return a RequeueNeededAfter.
		return err
	}

	// Now that external AWS service resources have been appropriately cleaned
	// up, we remove the finalizer representing the CR is managed by ACK,
	// allowing the CR to be deleted by the Kubernetes API server
	if err = r.setResourceUnmanaged(ctx, current); err != nil {
		return err
	}
	rlog.Info("deleted resource")
	return nil
}

// setResourceManaged marks the underlying CR in the supplied AWSResource with
// a finalizer that indicates the object is under ACK management and will not
// be deleted until that finalizer is removed (in setResourceUnmanaged())
func (r *resourceReconciler) setResourceManaged(
	ctx context.Context,
	res acktypes.AWSResource,
) error {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.setResourceManaged")
	defer exit(err)

	if r.rd.IsManaged(res) {
		return nil
	}
	orig := res.RuntimeObject().DeepCopyObject()
	r.rd.MarkManaged(res)
	rlog.Enter("kc.Patch (metadata + spec)")
	err = r.kc.Patch(
		ctx,
		res.RuntimeObject(),
		client.MergeFrom(orig),
	)
	rlog.Exit("kc.Patch (metadata + spec)", err)
	if err != nil {
		return err
	}
	rlog.Debug("marked resource as managed")
	return nil
}

// setResourceUnmanaged removes a finalizer from the underlying CR in the
// supplied AWSResource that indicates the object is under ACK management. This
// allows the CR to be deleted by the Kubernetes API server.
func (r *resourceReconciler) setResourceUnmanaged(
	ctx context.Context,
	res acktypes.AWSResource,
) error {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.setResourceUnmanaged")
	defer exit(err)

	if !r.rd.IsManaged(res) {
		return nil
	}
	orig := res.RuntimeObject().DeepCopyObject()
	r.rd.MarkUnmanaged(res)
	rlog.Enter("kc.Patch (metadata + spec)")
	err = r.kc.Patch(
		ctx,
		res.RuntimeObject(),
		client.MergeFrom(orig),
	)
	rlog.Exit("kc.Patch (metadata + spec)", err)
	if err != nil {
		return err
	}
	rlog.Debug("removed resource from management")
	return nil
}

// getAWSResource returns an AWSResource representing the requested Kubernetes
// namespaced object
func (r *resourceReconciler) getAWSResource(
	ctx context.Context,
	req ctrlrt.Request,
) (acktypes.AWSResource, error) {
	ro := r.rd.EmptyRuntimeObject()
	if err := r.kc.Get(ctx, req.NamespacedName, ro); err != nil {
		return nil, err
	}
	return r.rd.ResourceFromRuntimeObject(ro), nil
}

// handleReconcileError will handle errors from reconcile handlers, which
// respects runtime errors.
func (r *resourceReconciler) handleReconcileError(
	ctx context.Context,
	err error,
) (ctrlrt.Result, error) {
	if err == nil || err == ackerr.Terminal {
		return ctrlrt.Result{}, nil
	}
	rlog := ackrtlog.FromContext(ctx)

	var requeueNeededAfter *requeue.RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		after := requeueNeededAfter.Duration()
		rlog.Debug(
			"requeue needed after error",
			"error", requeueNeededAfter.Unwrap(),
			"after", after,
		)
		return ctrlrt.Result{RequeueAfter: after}, nil
	}

	var requeueNeeded *requeue.RequeueNeeded
	if errors.As(err, &requeueNeeded) {
		rlog.Debug(
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
func (r *resourceReconciler) getOwnerAccountID(
	res acktypes.AWSResource,
) ackv1alpha1.AWSAccountID {
	acctID := res.Identifiers().OwnerAccountID()
	if acctID != nil {
		return *acctID
	}

	// look for owner account id in the namespace annotations
	namespace := res.MetaObject().GetNamespace()
	accID, ok := r.cache.Namespaces.GetOwnerAccountID(namespace)
	if ok {
		return ackv1alpha1.AWSAccountID(accID)
	}

	// use controller configuration
	return ackv1alpha1.AWSAccountID(r.cfg.AccountID)
}

// getRoleARN return the Role ARN that should be assumed in order to manage
// the resources.
func (r *resourceReconciler) getRoleARN(
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
func (r *resourceReconciler) getRegion(
	res acktypes.AWSResource,
) ackv1alpha1.AWSRegion {
	// look for region in CR metadata annotations
	resAnnotations := res.MetaObject().GetAnnotations()
	region, ok := resAnnotations[ackv1alpha1.AnnotationRegion]
	if ok {
		return ackv1alpha1.AWSRegion(region)
	}

	// look for default region in namespace metadata annotations
	ns := res.MetaObject().GetNamespace()
	defaultRegion, ok := r.cache.Namespaces.GetDefaultRegion(ns)
	if ok {
		return ackv1alpha1.AWSRegion(defaultRegion)
	}

	// use controller configuration region
	return ackv1alpha1.AWSRegion(r.cfg.Region)
}

// NewReconciler returns a new reconciler object
func NewReconciler(
	sc acktypes.ServiceController,
	rmf acktypes.AWSResourceManagerFactory,
	log logr.Logger,
	cfg ackcfg.Config,
	metrics *ackmetrics.Metrics,
) acktypes.AWSResourceReconciler {
	return NewReconcilerWithClient(sc, nil, rmf, log, cfg, metrics)
}

// NewReconcilerWithClient returns a new reconciler object
// with Client(controller-runtime/pkg/client) already set.
func NewReconcilerWithClient(
	sc acktypes.ServiceController,
	kc client.Client,
	rmf acktypes.AWSResourceManagerFactory,
	log logr.Logger,
	cfg ackcfg.Config,
	metrics *ackmetrics.Metrics,
) acktypes.AWSResourceReconciler {
	return &resourceReconciler{
		reconciler: reconciler{
			sc:      sc,
			kc:      kc,
			log:     log.WithName("ackrt"),
			cfg:     cfg,
			metrics: metrics,
		},
		rmf: rmf,
		rd:  rmf.ResourceDescriptor(),
	}
}
