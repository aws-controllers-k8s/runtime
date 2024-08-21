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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/arn"
	backoff "github.com/cenkalti/backoff/v4"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlrt "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlrtcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
	"github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackcondition "github.com/aws-controllers-k8s/runtime/pkg/condition"
	ackcfg "github.com/aws-controllers-k8s/runtime/pkg/config"
	ackerr "github.com/aws-controllers-k8s/runtime/pkg/errors"
	"github.com/aws-controllers-k8s/runtime/pkg/featuregate"
	ackmetrics "github.com/aws-controllers-k8s/runtime/pkg/metrics"
	"github.com/aws-controllers-k8s/runtime/pkg/requeue"
	ackrtcache "github.com/aws-controllers-k8s/runtime/pkg/runtime/cache"
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

const (
	backoffReadOneTimeout = 10 * time.Second
	// The default duration to trigger the sync for an ACK resource after
	// the successful reconciliation. This behavior for a resource can be
	// overriden by RequeueOnSuccessSeconds configuration for that resource.
	defaultResyncPeriod = 10 * time.Hour
	// roleARNNotAvailableRequeueDelay is the default duration to requeue the
	// resource if the CARM cache is not synced yet, or if the roleARN is not
	// available.
	roleARNNotAvailableRequeueDelay = 15 * time.Second
)

// reconciler describes a generic reconciler within ACK.
type reconciler struct {
	sc        acktypes.ServiceController
	kc        client.Client
	apiReader client.Reader
	log       logr.Logger
	cfg       ackcfg.Config
	cache     ackrtcache.Caches
	metrics   *ackmetrics.Metrics
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
	rmf          acktypes.AWSResourceManagerFactory
	rd           acktypes.AWSResourceDescriptor
	resyncPeriod time.Duration
}

// GroupVersionKind returns the string containing the API group, version and
// kind reconciled by this reconciler
func (r *resourceReconciler) GroupVersionKind() *schema.GroupVersionKind {
	if r.rd == nil {
		return nil
	}
	gvk := r.rd.GroupVersionKind()
	return &gvk
}

// BindControllerManager sets up the AWSResourceReconciler with an instance
// of an upstream controller-runtime.Manager
func (r *resourceReconciler) BindControllerManager(mgr ctrlrt.Manager) error {
	if r.rmf == nil {
		return ackerr.NilResourceManagerFactory
	}
	r.kc = mgr.GetClient()
	r.apiReader = mgr.GetAPIReader()
	rd := r.rmf.ResourceDescriptor()
	maxConcurrentReconciles := r.cfg.GetReconcileResourceMaxConcurrency(rd.GroupVersionKind().Kind)
	return ctrlrt.NewControllerManagedBy(
		mgr,
	).For(
		rd.EmptyRuntimeObject(),
	).WithEventFilter(
		predicate.GenerationChangedPredicate{},
	).WithOptions(
		ctrlrtcontroller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
		},
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
	// During the reconcile process, the resourceNamespace is stored in the context
	// and can be used to fetch the secret if the namespace is not provided in the
	// SecretKeyReference.
	//
	// NOTE(a-hilaly): When refactoring the runtime, we might want to consider passing
	// the ObjectMeta in the context.
	ctxResourceNamespace := ctx.Value("resourceNamespace")
	if namespace == "" && ctxResourceNamespace != nil {
		ctxNamespace, ok := ctxResourceNamespace.(string)
		if ok {
			namespace = ctxNamespace
		}
	}

	nsn := client.ObjectKey{
		Namespace: namespace,
		Name:      ref.Name,
	}
	var secret corev1.Secret
	if err := r.apiReader.Get(ctx, nsn, &secret); err != nil {
		return "", ackerr.SecretNotFound
	}

	// Currently we have only Opaque secrets in scope.
	if secret.Type != corev1.SecretTypeOpaque {
		return "", ackerr.SecretTypeNotSupported
	}

	if value, ok := secret.Data[ref.Key]; ok {
		valuestr := string(value)
		return valuestr, nil
	}

	return "", ackerr.SecretNotFound
}

// WriteToSecret writes a value to a Secret given the namespace, name,
// and key of the Secret
func (r *reconciler) WriteToSecret(
	ctx context.Context,
	sourceValue string,
	namespace string,
	name string,
	key string,
) error {

	// Get the initial secret
	nsn := types.NamespacedName{
		Name: name,
	}
	nsn.Namespace = namespace

	secret := &corev1.Secret{}
	err := r.apiReader.Get(ctx, nsn, secret)
	if err != nil {
		return ackerr.SecretNotFound
	}

	// Update the field
	patch := client.StrategicMergeFrom(secret.DeepCopy())
	if secret.Data == nil {
		secret.Data = make(map[string][]byte, 1)
	}
	secret.Data[key] = []byte(sourceValue)

	err = r.kc.Patch(ctx, secret, patch)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile implements `controller-runtime.Reconciler` and handles reconciling
// a CR CRUD request
func (r *resourceReconciler) Reconcile(ctx context.Context, req ctrlrt.Request) (ctrlrt.Result, error) {
	desired, err := r.getAWSResource(ctx, req)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// resource wasn't found. just ignore these.
			return ctrlrt.Result{}, nil
		}
		return ctrlrt.Result{}, err
	}

	rlog := ackrtlog.NewResourceLogger(
		r.log, desired,
		// All the fields for a resource that do not change during reconciliation
		// can be initialized during resourceLogger creation
		"kind", r.rd.GroupVersionKind().Kind,
		"namespace", req.Namespace,
		"name", req.Name,
	)
	// We're storing a logger pointer in the context, so that any changes to the logger
	// will be reflected in the context.
	ctx = context.WithValue(ctx, ackrtlog.ContextKey, rlog)
	ctx = context.WithValue(ctx, "resourceNamespace", req.Namespace)

	// If a user has specified a namespace that is annotated with the
	// an owner account ID, we need an appropriate role ARN to assume
	// in order to perform the reconciliation. The roles ARN are typically
	// stored in the `ack-role-account-map` ConfigMap in the ACK system namespace.
	// If the ConfigMap is not created, or not populated with an
	// accountID to roleARN mapping, we need to properly requeue with a
	// helpful message to the user.
	acctID, needCARMLookup := r.getOwnerAccountID(desired)

	var roleARN ackv1alpha1.AWSResourceName
	if teamID := r.getTeamID(desired); teamID != "" && r.cfg.FeatureGates.IsEnabled(featuregate.TeamLevelCARM) {
		// The user is specifying a namespace that is annotated with a team ID.
		// Requeue if the corresponding roleARN is not available in the Teams configmap.
		// Additionally, set the account ID to the role's account ID.
		roleARN, err = r.getRoleARN(string(teamID), ackrtcache.ACKRoleTeamMap)
		if err != nil {
			return r.handleCacheError(ctx, err, desired)
		}
		parsedARN, err := arn.Parse(string(roleARN))
		if err != nil {
			return ctrlrt.Result{}, fmt.Errorf("parsing role ARN %q from %q configmap: %v", roleARN, ackrtcache.ACKRoleTeamMap, err)
		}
		acctID = ackv1alpha1.AWSAccountID(parsedARN.AccountID)
	} else if needCARMLookup {
		// The user is specifying a namespace that is annotated with an owner account ID.
		// Requeue if the corresponding roleARN is not available in the Accounts configmap.
		roleARN, err = r.getRoleARN(string(acctID), ackrtcache.ACKRoleAccountMap)
		if err != nil {
			return r.handleCacheError(ctx, err, desired)
		}
	}

	region := r.getRegion(desired)
	endpointURL := r.getEndpointURL(desired)
	gvk := r.rd.GroupVersionKind()
	// New session will only pivot to the roleARN if it is not empty.
	sess, err := r.sc.NewSession(region, &endpointURL, roleARN, gvk)
	if err != nil {
		return ctrlrt.Result{}, err
	}

	rlog.WithValues(
		"account", acctID,
		"role", roleARN,
		"region", region,
	)

	rm, err := r.rmf.ManagerFor(
		r.cfg, r.log, r.metrics, r, sess, acctID, region, roleARN,
	)
	if err != nil {
		return ctrlrt.Result{}, err
	}
	latest, err := r.reconcile(ctx, rm, desired)
	return r.HandleReconcileError(ctx, desired, latest, err)
}

func (r *resourceReconciler) handleCacheError(
	ctx context.Context,
	err error,
	desired acktypes.AWSResource,
) (ctrlrt.Result, error) {
	// TODO(a-hilaly): Refactor all the reconcile function to make it
	// easier to understand and maintain.
	reason := err.Error()
	latest := desired.DeepCopy()
	// set ResourceSynced condition to false with proper error message
	condition.SetSynced(latest, corev1.ConditionFalse, &condition.UnavailableIAMRoleMessage, &reason)
	return r.HandleReconcileError(ctx, desired, latest, requeue.NeededAfter(err, roleARNNotAvailableRequeueDelay))
}

// reconcile either cleans up a deleted resource or ensures that the supplied
// AWSResource's backing API resource matches the supplied desired state.
//
// It returns a copy of the resource that represents the latest observed state.
func (r *resourceReconciler) reconcile(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	res acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	if res.IsBeingDeleted() {
		// Determine whether we should retain or delete the resource
		if r.getDeletionPolicy(res) == ackv1alpha1.DeletionPolicyDelete {
			// Resolve references before deleting the resource.
			// Ignore any errors while resolving the references
			resolved, _, _ := rm.ResolveReferences(ctx, r.apiReader, res)
			return r.deleteResource(ctx, rm, resolved)
		}

		rlog := ackrtlog.FromContext(ctx)
		rlog.Info("AWS resource will not be deleted - deletion policy set to retain")
		if err := r.setResourceUnmanaged(ctx, rm, res); err != nil {
			return res, err
		}
		return r.handleRequeues(ctx, res)
	}
	latest, err := r.Sync(ctx, rm, res)
	if err != nil {
		return latest, err
	}
	return r.handleRequeues(ctx, latest)
}

// Sync ensures that the supplied AWSResource's backing API resource
// matches the supplied desired state.
//
// It returns a copy of the resource that represents the latest observed state.
func (r *resourceReconciler) Sync(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	desired acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.Sync")
	defer func() {
		exit(err)
	}()

	var latest acktypes.AWSResource // the newly created or mutated resource

	r.resetConditions(ctx, desired)
	defer func() {
		r.ensureConditions(ctx, rm, latest, err)
	}()

	isAdopted := IsAdopted(desired)
	rlog.WithValues("is_adopted", isAdopted)

	rlog.Enter("rm.ResolveReferences")
	resolved, hasReferences, err := rm.ResolveReferences(ctx, r.apiReader, desired)
	rlog.Exit("rm.ResolveReferences", err)
	if err != nil {
		return ackcondition.WithReferencesResolvedCondition(desired, err), err
	}
	if hasReferences {
		resolved = ackcondition.WithReferencesResolvedCondition(resolved, err)
	}

	rlog.Enter("rm.EnsureTags")
	err = rm.EnsureTags(ctx, resolved, r.sc.GetMetadata())
	rlog.Exit("rm.EnsureTags", err)
	if err != nil {
		return resolved, err
	}

	rlog.Enter("rm.ReadOne")
	latest, err = rm.ReadOne(ctx, resolved)
	rlog.Exit("rm.ReadOne", err)
	if err != nil {
		if err != ackerr.NotFound {
			return latest, err
		}
		if isAdopted {
			return nil, ackerr.AdoptedResourceNotFound
		}
		if latest, err = r.createResource(ctx, rm, resolved); err != nil {
			return latest, err
		}
	} else {
		if latest, err = r.updateResource(ctx, rm, resolved, latest); err != nil {
			return latest, err
		}
	}
	// Attempt to late initialize the resource. If there are no fields to
	// late initialize, this operation will be a no-op.
	if latest, err = r.lateInitializeResource(ctx, rm, latest); err != nil {
		return latest, err
	}
	return latest, nil
}

// resetConditions strips the supplied resource of all objects in its
// Status.Conditions collection. We do this at the start of each reconciliation
// loop in order to ensure that the objects in the Status.Conditions collection
// represent the state transitions that occurred in the last reconciliation
// loop. In other words, Status.Conditions should refer to the latest observed
// state read.
func (r *resourceReconciler) resetConditions(
	ctx context.Context,
	res acktypes.AWSResource,
) {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.resetConditions")
	defer func() {
		exit(err)
	}()

	ackcondition.Clear(res)
}

// ensureConditions examines the supplied resource's collection of Condition
// objects and ensures that an ACK.ResourceSynced condition is present.
func (r *resourceReconciler) ensureConditions(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	res acktypes.AWSResource,
	reconcileErr error,
) {
	if ackcompare.IsNil(res) {
		return
	}

	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.ensureConditions")
	defer func() {
		exit(err)
	}()

	// If the ACK.ResourceSynced condition is not set using the custom hooks,
	// determine the Synced condition using "rm.IsSynced" method
	if ackcondition.Synced(res) == nil {
		condStatus := corev1.ConditionFalse
		synced := false
		condMessage := ackcondition.NotSyncedMessage
		var condReason string
		rlog.Enter("rm.IsSynced")
		if synced, err = rm.IsSynced(ctx, res); err == nil && synced {
			condStatus = corev1.ConditionTrue
			condMessage = ackcondition.SyncedMessage
		} else if err != nil {
			condReason = err.Error()
		}
		rlog.Exit("rm.IsSynced", err)

		if reconcileErr != nil {
			condReason = reconcileErr.Error()
			if reconcileErr == ackerr.Terminal {
				// A terminal condition is a stable state for a resource.
				// Terminal conditions indicate that without changes to the
				// desired state of a resource, the resource's desired state
				// will never match the latest observed state. Thus,
				// ACK.ResourceSynced must be False.
				condStatus = corev1.ConditionFalse
				condMessage = ackcondition.NotSyncedMessage
			} else {
				// For any other reconciler error, set synced condition to
				// unknown, since we don't know whether the resource's desired
				// state matches the resource's latest observed state.
				condStatus = corev1.ConditionUnknown
				condMessage = ackcondition.UnknownSyncedMessage
			}
		}
		ackcondition.SetSynced(res, condStatus, &condMessage, &condReason)
	}
}

// createResource marks the CR as managed by ACK, calls one or more AWS APIs to
// create the backend AWS resource and patches the CR's Metadata, Spec and
// Status back to the Kubernetes API.
//
// When the backend resource modification fails, we return an error along with
// the latest observed state of the CR, and the HandleReconcileError wrapper
// ensures that the CR's Status is patched back to the Kubernetes API. This is
// done in order to ensure things like Conditions are appropriately saved on
// the resource.
//
// The function returns a copy of the CR that has most recently been patched
// back to the Kubernetes API.
func (r *resourceReconciler) createResource(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	desired acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.createResource")
	defer func() {
		exit(err)
	}()

	var latest acktypes.AWSResource // the newly created resource

	// Before we create the backend AWS service resources, let's first mark
	// the CR as being managed by ACK. Internally, this means adding a
	// finalizer to the CR; a finalizer that is removed once ACK no longer
	// manages the resource OR if the backend AWS service resource is
	// properly deleted.
	if !r.rd.IsManaged(desired) {
		if err = r.setResourceManaged(ctx, rm, desired); err != nil {
			return nil, err
		}

		// Ensure tags again after adding the finalizer and patching the
		// resource. Patching desired resource omits the controller tags
		// because they are not persisted in etcd. So we again ensure
		// that tags are present before performing the create operation.
		rlog.Enter("rm.EnsureTags")
		err = rm.EnsureTags(ctx, desired, r.sc.GetMetadata())
		rlog.Exit("rm.EnsureTags", err)
		if err != nil {
			return desired, err
		}
	}

	rlog.Enter("rm.Create")
	latest, err = rm.Create(ctx, desired)
	rlog.Exit("rm.Create", err)
	if err != nil {
		return latest, err
	}

	rlog.Enter("rm.ReadOne")
	observed, err := rm.ReadOne(ctx, latest)
	rlog.Exit("rm.ReadOne", err)
	if err != nil {
		if err == ackerr.NotFound {
			// Some eventually-consistent APIs return a 404 from a
			// ReadOne operation immediately after a successful
			// Create operation. In these exceptional cases
			// we retry the ReadOne operation with a backoff
			// until we get the expected 200 from the ReadOne.
			rlog.Enter("rm.delayedReadOneAfterCreate")
			observed, err = r.delayedReadOneAfterCreate(ctx, rm, latest)
			rlog.Exit("rm.delayedReadOneAfterCreate", err)
			if err != nil {
				return latest, err
			}
		} else {
			return latest, err
		}
	}

	// Take the status from the latest ReadOne
	latest.SetStatus(observed)

	// Ensure that we are patching any changes to the annotations/metadata and
	// the Spec that may have been set by the resource manager's successful
	// Create call above.
	latest, err = r.patchResourceMetadataAndSpec(ctx, rm, desired, latest)
	if err != nil {
		return latest, err
	}
	rlog.Info("created new resource")

	return latest, nil
}

// delayedReadOneAfterCreate is a helper function called when a ReadOne call
// fails with a 404 error right after a Create call. It uses a backoff/retry
// mechanism to retrieve the observed state right after a readone call.
func (r *resourceReconciler) delayedReadOneAfterCreate(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	res acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.delayedReadOneAfterCreate")
	defer func() {
		exit(err)
	}()

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = backoffReadOneTimeout
	ticker := backoff.NewTicker(bo)
	attempts := 0

	var observed acktypes.AWSResource

	for range ticker.C {
		attempts++

		rlog.Enter(fmt.Sprintf("rm.ReadOne (attempt %d)", attempts))
		observed, err = rm.ReadOne(ctx, res)
		rlog.Exit(fmt.Sprintf("rm.ReadOne (attempt %d)", attempts), err)
		if err == nil || err != ackerr.NotFound {
			ticker.Stop()
			break
		}
	}
	if err != nil {
		return res, ackerr.NewReadOneFailAfterCreate(attempts)
	}
	return observed, nil
}

// updateResource calls one or more AWS APIs to modify the backend AWS resource
// and patches the CR's Metadata and Spec back to the Kubernetes API.
//
// When the backend resource creation fails, we return an error along with the
// latest observed state of the CR, and the HandleReconcileError wrapper
// ensures that the CR's Status is patched back to the Kubernetes API. This is
// done in order to ensure things like Conditions are appropriately saved on
// the resource.
//
// The function returns a copy of the CR that has most recently been patched
// back to the Kubernetes API.
func (r *resourceReconciler) updateResource(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.updateResource")
	defer func() {
		exit(err)
	}()

	// Ensure the resource is managed
	if err = r.failOnResourceUnmanaged(ctx, latest); err != nil {
		return latest, err
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
			return latest, err
		}
		// Ensure that we are patching any changes to the annotations/metadata and
		// the Spec that may have been set by the resource manager's successful
		// Update call above.
		latest, err = r.patchResourceMetadataAndSpec(ctx, rm, desired, latest)
		if err != nil {
			return latest, err
		}
		rlog.Info("updated resource")
	}
	return latest, nil
}

// lateInitializeResource calls AWSResourceManager.LateInitialize() method and
// returns the AWSResource with late initialized fields.
//
// When the late initialization is delayed for an AWSResource, an error is returned
// with specific requeue delay to attempt lateInitialization again.
//
// This method also adds an annotation to K8s CR, indicating the number of
// late initialization attempts to correctly calculate exponential backoff delay
//
// This method also adds Condition to CR's status indicating status of late initialization.
func (r *resourceReconciler) lateInitializeResource(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	latest acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.lateInitializeResource")
	defer func() {
		exit(err)
	}()

	rlog.Enter("rm.LateInitialize")
	lateInitializedLatest, err := rm.LateInitialize(ctx, latest)
	rlog.Exit("rm.LateInitialize", err)
	// Always patch after late initialize because some fields may have been initialized while
	// others require a retry after some delay.
	// This patching does not hurt because if there is no diff then 'patchResourceMetadataAndSpec'
	// acts as a no-op.
	if ackcompare.IsNotNil(lateInitializedLatest) {
		var patchErr error
		lateInitializedLatest, patchErr = r.patchResourceMetadataAndSpec(ctx, rm, latest, lateInitializedLatest)
		// Throw the patching error if reconciler is unable to patch the resource with late initializations
		if patchErr != nil {
			err = patchErr
		}
	}
	return lateInitializedLatest, err
}

// getPatchDocument returns a JSON string containing the object that will be
// patched in the Kubernetes API server.
//
// NOTE(jaypipes): Because the Kubernetes API server's server-side apply
// functionality introduces an enormous amount of verbose annotations in the
// resource metadata, and because those annotations are pretty unhelpful to
// ACK, we strip all that stuff out of the returned patch document.
func getPatchDocument(
	patch client.Patch,
	obj client.Object, // the diff of this will be represented in the patch
) string {
	js, _ := patch.Data(obj)
	var m map[string]interface{}
	_ = json.Unmarshal(js, &m)
	if md, ok := m["metadata"]; ok {
		// Strip out managedFields stuff, since it's super verbose and
		// doesn't offer any value to us (since we don't use server-side
		// apply
		if mv, ok := md.(map[string]interface{}); ok {
			if _, ok := mv["managedFields"]; ok {
				delete(mv, "managedFields")
			}
		}
	}
	js, _ = json.Marshal(m)
	return string(js)
}

// patchResourceMetadataAndSpec patches the custom resource in the Kubernetes
// API to match the supplied latest resource's metadata and spec.
//
// NOTE(redbackthomson): This method returns an updated version of the latest
// parameter. This version has an updated metadata.resourceVersion, which is
// incremented in the process of calling Patch. This is intentional, because
// without updating the resource's metadata.resourceVersion, the resource cannot
// be passed to Patch again later in the reconciliation if Patch is called with
// the Optimistic Locking option.
//
// See https://github.com/kubernetes-sigs/controller-runtime/blob/165a8c869c4388b861c7c91cb1e5330f6e07ee16/pkg/client/patch.go#L81-L84
// for more information.
func (r *resourceReconciler) patchResourceMetadataAndSpec(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.patchResourceMetadataAndSpec")
	defer func() {
		exit(err)
	}()

	// Remove resolved references from the objects before patching
	desiredCleaned := rm.ClearResolvedReferences(desired)
	latestCleaned := rm.ClearResolvedReferences(latest)

	equalMetadata, err := ackcompare.MetaV1ObjectEqual(desiredCleaned.MetaObject(), latestCleaned.MetaObject())
	if err != nil {
		return latest, err
	}
	if equalMetadata && !r.rd.Delta(desiredCleaned, latestCleaned).DifferentAt("Spec") {
		rlog.Debug("no difference found between metadata and spec for desired and latest object.")
		return latest, nil
	}

	rlog.Enter("kc.Patch (metadata + spec)")
	lorig := latestCleaned.DeepCopy()
	patch := client.MergeFrom(desiredCleaned.RuntimeObject())
	err = r.kc.Patch(ctx, latestCleaned.RuntimeObject(), patch)
	if err == nil {
		if rlog.IsDebugEnabled() {
			js := getPatchDocument(patch, lorig.RuntimeObject())
			rlog.Debug("patched resource metadata + spec", "json", js)
		}
	}
	// The call to Patch() above ends up setting the latest variable's Status
	// to the value of the desired variable's Status. We do not want this
	// behaviour; instead, we want to keep latest's original Status value.
	latestCleaned.SetStatus(lorig)
	rlog.Exit("kc.Patch (metadata + spec)", err)
	return latestCleaned, err
}

// patchResourceStatus patches the custom resource in the Kubernetes API to
// match the supplied latest resource.
//
// NOTE(jaypipes): We make a copy of both desired and latest parameters to
// avoid mutating either
func (r *resourceReconciler) patchResourceStatus(
	ctx context.Context,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
) error {
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.patchResourceStatus")
	defer func() {
		exit(err)
	}()

	rlog.Enter("kc.Patch (status)")
	dobj := desired.DeepCopy().RuntimeObject()
	lobj := latest.DeepCopy().RuntimeObject()
	patch := client.MergeFrom(dobj)
	err = r.kc.Status().Patch(ctx, lobj, patch)
	if err == nil {
		if rlog.IsDebugEnabled() {
			js := getPatchDocument(patch, lobj)
			rlog.Debug("patched resource status", "json", js)
		}
	} else if apierrors.IsNotFound(err) {
		// reset the NotFound error so it is not printed in controller logs
		// providing false positive error
		err = nil
	}
	rlog.Exit("kc.Patch (status)", err)
	return err
}

// deleteResource ensures that the supplied AWSResource's backing API resource
// is destroyed along with all child dependent resources.
//
// Returns a copy of the resource with the latest state either right before
// deletion OR after a failed attempted deletion.
func (r *resourceReconciler) deleteResource(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	current acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	// TODO(jaypipes): Handle all dependent resources. The AWSResource
	// interface needs to get some methods that return schema relationships,
	// first though
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.deleteResource")
	defer func() {
		exit(err)
	}()

	rlog.Enter("rm.ReadOne")
	observed, err := rm.ReadOne(ctx, current)
	rlog.Exit("rm.ReadOne", err)
	if err != nil {
		if err == ackerr.NotFound {
			// If the aws resource is not found, remove finalizer
			return current, r.setResourceUnmanaged(ctx, rm, current)
		}
		return current, err
	}
	rlog.Enter("rm.Delete")
	latest, err := rm.Delete(ctx, observed)
	rlog.Exit("rm.Delete", err)
	if ackcompare.IsNotNil(latest) {
		// The Delete operation may be asynchronous and the resource manager
		// may have set a Spec field or metadata on the CR during `rm.Delete`,
		// so we make sure to save any of those Spec/Metadata changes here.
		//
		// NOTE(jaypipes): The `HandleReconcilerError` wrapper *always* saves
		// any changes to Status fields that may have been made by the resource
		// manager if the returned `latest` resource is non-nil, so we don't
		// have to worry about saving status stuff here.
		latest, _ = r.patchResourceMetadataAndSpec(ctx, rm, current, latest)
	}
	if err != nil {
		// NOTE: Delete() implementations that have asynchronously-completing
		// deletions should return a RequeueNeededAfter.
		return latest, err
	}

	// Now that external AWS service resources have been appropriately cleaned
	// up, we remove the finalizer representing the CR is managed by ACK,
	// allowing the CR to be deleted by the Kubernetes API server
	if ackcompare.IsNotNil(latest) {
		err = r.setResourceUnmanaged(ctx, rm, latest)
	} else {
		err = r.setResourceUnmanaged(ctx, rm, current)
	}
	if err == nil {
		rlog.Info("deleted resource")
	}

	return latest, err
}

// setResourceManaged marks the underlying CR in the supplied AWSResource with
// a finalizer that indicates the object is under ACK management and will not
// be deleted until that finalizer is removed (in setResourceUnmanaged())
func (r *resourceReconciler) setResourceManaged(
	ctx context.Context,
	rm acktypes.AWSResourceManager,
	res acktypes.AWSResource,
) error {
	if r.rd.IsManaged(res) {
		return nil
	}
	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.setResourceManaged")
	defer func() {
		exit(err)
	}()

	orig := res.DeepCopy().RuntimeObject()
	r.rd.MarkManaged(res)
	res, err = r.patchResourceMetadataAndSpec(ctx, rm, r.rd.ResourceFromRuntimeObject(orig), res)
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
	rm acktypes.AWSResourceManager,
	res acktypes.AWSResource,
) error {
	if !r.rd.IsManaged(res) {
		return nil
	}

	var err error
	rlog := ackrtlog.FromContext(ctx)
	exit := rlog.Trace("r.setResourceUnmanaged")
	defer func() {
		exit(err)
	}()

	orig := res.DeepCopy().RuntimeObject()
	r.rd.MarkUnmanaged(res)
	res, err = r.patchResourceMetadataAndSpec(ctx, rm, r.rd.ResourceFromRuntimeObject(orig), res)
	if err != nil {
		return err
	}
	rlog.Debug("removed resource from management")
	return nil
}

// failOnResourceUnmanaged ensures that the underlying CR in the supplied
// AWSResource has a finalizer. If it does not, it will set a Terminal condition
// and return with an error
func (r *resourceReconciler) failOnResourceUnmanaged(
	ctx context.Context,
	res acktypes.AWSResource,
) error {
	if r.rd.IsManaged(res) {
		return nil
	}

	condition.SetTerminal(res, corev1.ConditionTrue, &condition.NotManagedMessage, &condition.NotManagedReason)
	return ackerr.Terminal
}

// getAWSResource returns an AWSResource representing the requested Kubernetes
// namespaced object
// NOTE: this method makes direct call to k8s apiserver. Currently this method
// is only invoked once per reconciler loop. For future use, Take care of k8s
// apiserver rate limit if calling this method more than once per reconciler
// loop.
func (r *resourceReconciler) getAWSResource(
	ctx context.Context,
	req ctrlrt.Request,
) (acktypes.AWSResource, error) {
	ro := r.rd.EmptyRuntimeObject()
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
	return r.rd.ResourceFromRuntimeObject(ro), nil
}

// handleRequeues examines the supplied latest observed resource state and
// triggers a requeue for reconciling the resource when certain events occur
// (or when nothing occurs and the resource manager for that kind of resource
// indicates the resource should be repeatedly reconciled)
func (r *resourceReconciler) handleRequeues(
	ctx context.Context,
	latest acktypes.AWSResource,
) (acktypes.AWSResource, error) {
	if ackcompare.IsNotNil(latest) {
		rlog := ackrtlog.FromContext(ctx)
		for _, condition := range latest.Conditions() {
			if condition.Type != ackv1alpha1.ConditionTypeResourceSynced {
				continue
			}
			// The code below only executes for "ConditionTypeResourceSynced"
			if condition.Status == corev1.ConditionTrue {
				rlog.Debug("requeuing", "after", r.resyncPeriod)
				return latest, requeue.NeededAfter(nil, r.resyncPeriod)
			} else {
				rlog.Debug(
					"requeueing resource after finding resource synced condition false",
				)
				return latest, requeue.NeededAfter(
					ackerr.TemporaryOutOfSync, requeue.DefaultRequeueAfterDuration)
			}
		}
	}
	return latest, nil
}

// HandleReconcileError will handle errors from reconcile handlers, which
// respects runtime errors.
//
// If the `latest` parameter is not nil, this function will ALWAYS patch the
// latest Status fields back to the Kubernetes API.
func (r *resourceReconciler) HandleReconcileError(
	ctx context.Context,
	desired acktypes.AWSResource,
	latest acktypes.AWSResource,
	err error,
) (ctrlrt.Result, error) {
	if ackcompare.IsNotNil(latest) {
		// The reconciliation loop may have returned an error, but if latest is
		// not nil, there may be some changes available in the CR's Status
		// struct (example: Conditions), and we want to make sure we save those
		// changes before proceeding
		//
		// PatchStatus even when resource is unmanaged. This helps in setting
		// conditions when resolving resource-reference fails, which happens
		// before resource is marked as managed.
		// It is okay to patch status when resource is not present due to deletion
		// because a NotFound error is thrown which will be ignored.
		//
		// TODO(jaypipes): We ignore error handling here but I don't know if
		// there is a more robust way to handle failures in the patch operation
		_ = r.patchResourceStatus(ctx, desired, latest)
	}
	if err == nil || err == ackerr.Terminal {
		return ctrlrt.Result{}, nil
	}
	rlog := ackrtlog.FromContext(ctx)

	var requeueNeededAfter *requeue.RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		after := requeueNeededAfter.Duration()
		if wrappedError := requeueNeededAfter.Unwrap(); wrappedError != nil {
			rlog.Debug(
				"requeue needed after error",
				"error", wrappedError,
				"after", after,
			)
		} else {
			rlog.Debug("requeueing", "after", after)
		}
		return ctrlrt.Result{RequeueAfter: after}, nil
	}

	var requeueNeeded *requeue.RequeueNeeded
	if errors.As(err, &requeueNeeded) {
		if wrappedError := requeueNeeded.Unwrap(); wrappedError != nil {
			rlog.Debug(
				"requeue needed after error",
				"error", wrappedError,
			)
		} else {
			rlog.Debug("requeueing immediately")
		}
		return ctrlrt.Result{Requeue: true}, nil
	}

	return ctrlrt.Result{}, err
}

// getOwnerAccountID returns the AWS account that owns the supplied resource.
// The function looks first to the default AWS account ID associated with the
// Kubernetes Namespace in which the CR was created, followed by the common
// `status.ackResourceMetadata` object, and finally the AWS Account in which the
// IAM Role that the service controller is in.
//
// This function is also returning a boolean stating whether the account ID
// is retrieved from the namespace annotations. This information is used to
// determine whether a role ARN should be assumed to manage the resource,
// which is typically found in the CARM ConfigMap.
//
// If the returned boolean is true, it means that the resource is owned by
// a different account than the controller's default account ID, and the
// controller should lookup the CARM ConfigMap.
func (r *resourceReconciler) getOwnerAccountID(
	res acktypes.AWSResource,
) (ackv1alpha1.AWSAccountID, bool) {
	// look for owner account id in the namespace annotations
	namespace := res.MetaObject().GetNamespace()
	accID, ok := r.cache.Namespaces.GetOwnerAccountID(namespace)
	if ok {
		return ackv1alpha1.AWSAccountID(accID), true
	}

	controllerAccountID := ackv1alpha1.AWSAccountID(r.cfg.AccountID)
	// look for owner account id in the resource status
	acctID := res.Identifiers().OwnerAccountID()
	if acctID != nil {
		return *acctID, *acctID != controllerAccountID
	}

	// use controller configuration
	return controllerAccountID, false
}

// getTeamID gets the team-id from the namespace annotation.
func (r *resourceReconciler) getTeamID(
	res acktypes.AWSResource,
) ackv1alpha1.TeamID {
	// look for team ID in the namespace annotations
	namespace := res.MetaObject().GetNamespace()
	namespacedTeamID, ok := r.cache.Namespaces.GetTeamID(namespace)
	if ok {
		return ackv1alpha1.TeamID(namespacedTeamID)
	}
	return ackv1alpha1.TeamID("")
}

// getRoleARN returns the Role ARN that should be assumed for the given accountID or teamID,
// from the appropriate configmap, in order to manage the resources.
func (r *resourceReconciler) getRoleARN(id string, cacheName string) (ackv1alpha1.AWSResourceName, error) {
	var cache *ackrtcache.CARMMap
	switch cacheName {
	case ackrtcache.ACKRoleTeamMap:
		cache = r.cache.Teams
	case ackrtcache.ACKRoleAccountMap:
		cache = r.cache.Accounts
	default:
		return "", fmt.Errorf("invalid cache name: %s", cacheName)
	}

	if r.cfg.FeatureGates.IsEnabled(featuregate.ServiceLevelCARM) {
		// use service level roleARN if present
		serviceID := r.sc.GetMetadata().ServiceAlias + "." + id
		if roleARN, err := cache.GetValue(serviceID); err == nil {
			return ackv1alpha1.AWSResourceName(roleARN), nil
		}
	}

	// otherwise use account/team level roleARN
	roleARN, err := cache.GetValue(id)
	if err != nil {
		return "", fmt.Errorf("retrieving role ARN for account/team ID %q from %q configmap: %v", id, cacheName, err)
	}
	return ackv1alpha1.AWSResourceName(roleARN), nil
}

// getRegion returns the region the resource exists in, or if the resource
// has yet to be created, the region the resource *should* be created in.
//
// If the resource has not yet been created, we look for the AWS region
// in the following order of precedence:
//   - The resource's `services.k8s.aws/region` annotation, if present
//   - The resource's Namespace's `services.k8s.aws/region` annotation, if present
//   - The controller's `--aws-region` CLI flag
func (r *resourceReconciler) getRegion(
	res acktypes.AWSResource,
) ackv1alpha1.AWSRegion {
	// first try to get the region from the status.resourceMetadata
	metadataRegion := res.Identifiers().Region()
	if metadataRegion != nil {
		return *metadataRegion
	}

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

// getDeletionPolicy returns the resource's deletion policy based on the default
// behaviour or any other overriding annotations.
//
// We look for the deletion policy in the annotations based on the following
// precedence:
//   - The resource's `services.k8s.aws/deletion-policy` annotation, if present
//   - The resource's Namespace's `{service}.services.k8s.aws/deletion-policy` annotation, if present
//   - The controller's `--deletion-policy` CLI flag
func (r *resourceReconciler) getDeletionPolicy(
	res acktypes.AWSResource,
) ackv1alpha1.DeletionPolicy {
	// look for deletion policy in CR metadata annotations
	resAnnotations := res.MetaObject().GetAnnotations()
	deletionPolicy, ok := resAnnotations[ackv1alpha1.AnnotationDeletionPolicy]
	if ok {
		return ackv1alpha1.DeletionPolicy(deletionPolicy)
	}

	// look for default deletion policy in namespace metadata annotations
	ns := res.MetaObject().GetNamespace()
	deletionPolicy, ok = r.cache.Namespaces.GetDeletionPolicy(ns, r.sc.GetMetadata().ServiceAlias)
	if ok {
		return ackv1alpha1.DeletionPolicy(deletionPolicy)
	}

	// use controller configuration policy
	return r.cfg.DeletionPolicy
}

// getEndpointURL returns the AWS account that owns the supplied resource.
// We look for the namespace associated endpoint url, if that is set we use it.
// Otherwise if none of these annotations are set we use the endpoint url specified
// in the configuration
func (r *resourceReconciler) getEndpointURL(
	res acktypes.AWSResource,
) string {

	// look for endpoint url in the namespace annotations
	namespace := res.MetaObject().GetNamespace()
	endpointURL, ok := r.cache.Namespaces.GetEndpointURL(namespace)
	if ok {
		return endpointURL
	}

	// use controller configuration EndpointURL
	return r.cfg.EndpointURL
}

// getResyncPeriod returns the period of the recurring reconciler process which ensures the desired
// state of custom resources is maintained.
// It attempts to retrieve the duration from the following sources, in this order:
//  1. A resource-specific reconciliation resync period specified in the reconciliation resync
//     configuration map (--reconcile-default-resync-seconds).
//  2. A resource-specific requeue on success period specified by the resource manager factory.
//     The resource manager factory is controller-specific, and thus this period is to specified
//     by controller authors (using ack-generate).
//  3. The default reconciliation resync period period specified in the controller binary flags.
//     (--reconcile-resource-resync-seconds)
//  4. The default resync period defined in the ACK runtime package. Defined in defaultResyncPeriod
//     within the same file
//
// Each reconciler has a unique value to use. This function should only be called during the
// instantiation of an AWSResourceReconciler and should not be called during the reconciliation
// function r.Sync
func getResyncPeriod(rmf acktypes.AWSResourceManagerFactory, cfg ackcfg.Config) time.Duration {
	// The reconciliation resync period configuration has already been validated as
	// a clean map. Therefore, we can safely ignore any errors that may occur while
	// parsing it and avoid changing the signature of NewReconcilerWithClient.
	drc, _ := cfg.ParseReconcileResourceResyncSeconds()

	// First, try to use a resource-specific resync period if provided in the resource
	// resync period configuration.
	resourceKind := rmf.ResourceDescriptor().GroupVersionKind().Kind
	if duration, ok := drc[strings.ToLower(resourceKind)]; ok && duration > 0 {
		return time.Duration(duration) * time.Second
	}

	// Second, try to use a resource-specific requeue on success period specified by the
	// resource manager factory. This value is set during the code generation of the
	// controller and takes precedence over the default resync period period because
	// it allows existing controllers that rely on this value to maintain their intended
	// behavior.
	if duration := rmf.RequeueOnSuccessSeconds(); duration > 0 {
		return time.Duration(duration) * time.Second
	}

	// Third, try to use the default resync period resync period specified during controller
	// start-up.
	if cfg.ReconcileDefaultResyncSeconds > 0 {
		return time.Duration(cfg.ReconcileDefaultResyncSeconds) * time.Second
	}

	// If none of the above values are present or valid, use the default resync period
	// defined in the ACK runtime package. Defined in `defaultResyncPeriod` within the
	// same file
	return defaultResyncPeriod
}

// NewReconciler returns a new reconciler object
func NewReconciler(
	sc acktypes.ServiceController,
	rmf acktypes.AWSResourceManagerFactory,
	log logr.Logger,
	cfg ackcfg.Config,
	metrics *ackmetrics.Metrics,
	cache ackrtcache.Caches,
) acktypes.AWSResourceReconciler {
	return NewReconcilerWithClient(sc, nil, rmf, log, cfg, metrics, cache)
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
	cache ackrtcache.Caches,
) acktypes.AWSResourceReconciler {
	rtLog := log.WithName("ackrt")
	resyncPeriod := getResyncPeriod(rmf, cfg)
	rtLog.V(1).Info("Initiating reconciler",
		"reconciler kind", rmf.ResourceDescriptor().GroupVersionKind().Kind,
		"resync period seconds", resyncPeriod.Seconds(),
	)
	return &resourceReconciler{
		reconciler: reconciler{
			sc:      sc,
			kc:      kc,
			log:     rtLog,
			cfg:     cfg,
			metrics: metrics,
			cache:   cache,
		},
		rmf:          rmf,
		rd:           rmf.ResourceDescriptor(),
		resyncPeriod: resyncPeriod,
	}
}
