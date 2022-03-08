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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	jq "github.com/itchyny/gojq"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	ackrtlog "github.com/aws-controllers-k8s/runtime/pkg/runtime/log"
	acktypes "github.com/aws-controllers-k8s/runtime/pkg/types"
)

const (
	fieldExportFinalizerString = "finalizers.services.k8s.aws/FieldExport"
)

var (
	pathDoesNotExistError = errors.New("path does not exist in this object")
)

// Remove the global to make it easier to mock
var (
	UnstructuredConverter runtime.UnstructuredConverter = runtime.DefaultUnstructuredConverter
)

// fieldExportReconciler is responsible for reconciling the state of any field
// export CRs that target resources supported by the current controller.
// It implements the upstream controller-runtime `Reconciler` interface.
type fieldExportReconciler struct {
	reconciler
	// resourceReconciler indicates that the reconciler should be reconciling
	// AWS resources
	resourceReconciler bool
	// rd is only used if binding to an ACK resource (not `FieldExport`)
	rd acktypes.AWSResourceDescriptor
}

// BindControllerManagerForFieldExport sets up the AWSResourceReconciler with an
// instance of an upstream controller-runtime.Manager, watching for changes in
// `FieldExport` CRs
func (r *fieldExportReconciler) BindControllerManagerForFieldExport(mgr ctrlrt.Manager) error {
	r.kc = mgr.GetClient()
	r.apiReader = mgr.GetAPIReader()

	if r.resourceReconciler {
		return errors.New("cannot bind to field export. reconciler marked for reconciling AWS resources")
	}

	return ctrlrt.NewControllerManagedBy(
		mgr,
	).For(
		// Read only field export objects
		&ackv1alpha1.FieldExport{},
	).WithEventFilter(
		predicate.GenerationChangedPredicate{},
	).Complete(r)
}

// BindControllerManagerForAWSResource sets up the AWSResourceReconciler with an
// instance of an upstream controller-runtime.Manager, watching for changes in
// AWS Resource CRs
func (r *fieldExportReconciler) BindControllerManagerForAWSResource(mgr ctrlrt.Manager) error {
	r.kc = mgr.GetClient()
	r.apiReader = mgr.GetAPIReader()

	if !r.resourceReconciler {
		return errors.New("cannot bind to AWS resource. reconciler marked for reconciling field exports")
	}

	return ctrlrt.NewControllerManagedBy(
		mgr,
	).For(
		r.rd.EmptyRuntimeObject(),
	).WithEventFilter(
		// Update on both status and spec changes
		predicate.ResourceVersionChangedPredicate{},
	).Complete(r)
}

// Reconcile implements `controller-runtime.Reconciler` and handles reconciling
// a CR CRUD request
func (r *fieldExportReconciler) Reconcile(ctx context.Context, req ctrlrt.Request) (ctrlrt.Result, error) {
	return r.handleReconcileError(r.reconcile(ctx, req))
}

func (r *fieldExportReconciler) reconcile(ctx context.Context, req ctrlrt.Request) error {
	// Determine if we are reconciling an ACK resource
	if r.resourceReconciler {
		return r.reconcileSourceResource(ctx, req)
	}

	// We are reconciling a field export CR
	return r.reconcileFieldExport(ctx, req)
}

// reconcileFieldExport handles updates to `FieldExport` resources
func (r *fieldExportReconciler) reconcileFieldExport(ctx context.Context, req ctrlrt.Request) error {
	feObject, err := r.getFieldExport(ctx, req)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	sourceGK := feObject.Spec.From.Resource.GroupKind
	sourceName := types.NamespacedName{
		Name: *feObject.Spec.From.Resource.Name,
		// We only support pulling from resources in
		// the same namespace
		Namespace: req.Namespace,
	}

	if feObject.DeletionTimestamp != nil {
		return r.cleanup(ctx, feObject)
	}

	// Check if the target API group matches with the controller
	var controllerRMF acktypes.AWSResourceManagerFactory
	for _, v := range r.sc.GetResourceManagerFactories() {
		controllerRMF = v
		break
	}
	if sourceGK.Group != controllerRMF.ResourceDescriptor().GroupKind().Group {
		ackrtlog.DebugFieldExport(r.log, feObject, "target resource API group is not of this service. no-op")
		return nil
	}

	if err := r.markManaged(ctx, feObject); err != nil {
		return r.onError(ctx, feObject, err)
	}

	// Look up the rmf for the given target resource GVK
	rmf, ok := (r.sc.GetResourceManagerFactories())[sourceGK.String()]
	if !ok {
		return ackerr.ResourceManagerFactoryNotFound
	}

	sourceObject, err := r.getSourceResource(ctx, rmf.ResourceDescriptor(), sourceName)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return r.onError(ctx, feObject, requeue.None(err))
		}
		return nil
	}

	// Attempt an initial export
	return r.Sync(ctx, sourceObject, *feObject)
}

// reconcileSourceResource handles updates to any other (not `FieldExport`) ACK
// resources
func (r *fieldExportReconciler) reconcileSourceResource(ctx context.Context, req ctrlrt.Request) error {
	res, err := r.getSourceResource(ctx, r.rd, req.NamespacedName)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			// Can't attach this error to any particular FieldExport object, so
			// it will only be displayed in the controller logs.
			return requeue.None(err)
		}
		return nil
	}

	// Get each of the exports referencing this AWS resource
	exports, err := r.FilterAllExports(ctx,
		*r.rd.GroupKind(),
		types.NamespacedName{
			Namespace: res.MetaObject().GetNamespace(),
			Name:      res.MetaObject().GetName(),
		},
	)
	if err != nil {
		return err
	}

	// Iterate through each export and sync it
	for _, export := range exports {
		if err = r.Sync(ctx, res, export); err != nil {
			return err
		}
	}

	return nil
}

// Sync will attempt to take the exported field value from the source ACK
// resource and write it into the destination field export output type.
func (r *fieldExportReconciler) Sync(
	ctx context.Context,
	from acktypes.AWSResource,
	desired ackv1alpha1.FieldExport,
) error {
	r.clearConditions(ctx, &desired)

	// Get the field from the resource
	value, err := r.getSourcePathFromResource(from, *desired.Spec.From.Path)
	if err != nil {
		return r.onError(ctx, &desired, err)
	} else if value == nil {
		return r.onError(ctx, &desired, requeue.None(pathDoesNotExistError))
	}

	switch *desired.Spec.To.Kind {
	case ackv1alpha1.FieldExportOutputTypeConfigMap:
		if err = r.writeToConfigMap(ctx, *value, &desired); err != nil {
			return r.onError(ctx, &desired, err)
		}
	case ackv1alpha1.FieldExportOutputTypeSecret:
		if err = r.writeToSecret(ctx, *value, &desired); err != nil {
			return r.onError(ctx, &desired, err)
		}
	}

	// Don't attempt to patch conditions again, directly return result of
	// 'r.onSuccess'
	return r.onSuccess(ctx, &desired)
}

// cleanup removes the finalizer from FieldExport so that k8s object can
// be deleted.
func (r *fieldExportReconciler) cleanup(
	ctx context.Context,
	current *ackv1alpha1.FieldExport,
) error {
	if err := r.markUnmanaged(ctx, current); err != nil {
		return err
	}

	return nil
}

// getFieldExport returns a FieldExport representing the requested Kubernetes
// namespaced object
func (r *fieldExportReconciler) getFieldExport(
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

// getSourceResource returns an ACK resource given a resource descriptor
// and its respective namespaced name.
func (r *fieldExportReconciler) getSourceResource(
	ctx context.Context,
	rd acktypes.AWSResourceDescriptor,
	name types.NamespacedName,
) (acktypes.AWSResource, error) {
	obj := rd.EmptyRuntimeObject()
	if err := r.apiReader.Get(ctx, name, obj); err != nil {
		// Don't throw an error if the source object can't be found
		return nil, err
	}
	res := rd.ResourceFromRuntimeObject(obj)

	// Ensure our current object is synced
	// if synced := ackcondition.Synced(res); synced == nil || synced.Status != corev1.ConditionTrue {
	// 	return nil, fmt.Errorf(
	// 		"resource does not have the %s condition set to %s",
	// 		ackv1alpha1.ConditionTypeResourceSynced,
	// 		corev1.ConditionTrue,
	// 	)
	// }

	return res, nil
}

// getSourcePathFromResource returns the value from the resource as referenced
// by the given path. This method currently only supports a single field, and
// will return the first one it finds if multiple are selected. This method only
// supports primitives of type `int`, `bool` and `string`. Returns value as a
// string or `nil` if the value could not be found or converted to a string.
func (r *fieldExportReconciler) getSourcePathFromResource(
	from acktypes.AWSResource,
	path string,
) (*string, error) {
	obj, err := UnstructuredConverter.ToUnstructured(from.RuntimeObject())
	if err != nil {
		return nil, err
	}

	query, err := jq.Parse(path)
	if err != nil {
		return nil, &terminalError{err: errors.Wrap(err, "unable to parse path")}
	}

	iter := query.Run(obj)

	// Currently we only support exporting a single selection
	result, ok := iter.Next()
	if !ok {
		return nil, nil
	}

	// Handle query errors
	if err, ok := result.(error); ok {
		if err != nil {
			return nil, &terminalError{err: errors.Wrap(err, "unable to execute query")}
		}
	}

	// If it's already a string
	var stringResult string
	stringResult, ok = result.(string)
	if ok {
		return &stringResult, nil
	}
	boolResult, ok := result.(bool)
	if ok {
		stringResult = fmt.Sprintf("%t", boolResult)
		return &stringResult, nil
	}
	intResult, ok := result.(int)
	if ok {
		stringResult = fmt.Sprintf("%d", intResult)
		return &stringResult, nil
	}

	return nil, nil
}

// writeToConfigMap will patch an existing config map to add an exported field
// value. By default the key will be "<namespace>.<name>" using values from the
// exporter that created it.
func (r *fieldExportReconciler) writeToConfigMap(
	ctx context.Context,
	sourceValue string,
	desired *ackv1alpha1.FieldExport,
) error {
	// Construct the data key
	key := fmt.Sprintf("%s.%s", desired.Namespace, desired.Name)

	// Get the initial configmap
	nsn := types.NamespacedName{
		Name: *desired.Spec.To.Name,
	}
	if desired.Spec.To.Namespace != nil {
		nsn.Namespace = *desired.Spec.To.Namespace
	} else {
		nsn.Namespace = desired.Namespace
	}

	cm := &corev1.ConfigMap{}
	err := r.apiReader.Get(ctx, nsn, cm)
	if err != nil {
		return errors.Wrap(err, "unable to get existing config map")
	}

	// Update the field
	patch := client.StrategicMergeFrom(cm.DeepCopy())
	if cm.Data == nil {
		cm.Data = make(map[string]string, 1)
	}
	cm.Data[key] = sourceValue

	ackrtlog.DebugFieldExport(r.log, desired, "patching target config map")
	err = r.kc.Patch(ctx, cm, patch)
	if err != nil {
		return err
	}
	ackrtlog.InfoFieldExport(r.log, desired, "patched target config map")

	return nil
}

// writeToSecret will patch an existing secret to add an exported field value.
// By default the key will be "<namespace>.<name>" using values from the
// exporter that created it.
func (r *fieldExportReconciler) writeToSecret(
	ctx context.Context,
	sourceValue string,
	desired *ackv1alpha1.FieldExport,
) error {
	// Construct the data key
	key := fmt.Sprintf("%s.%s", desired.Namespace, desired.Name)

	// Get the initial secret
	nsn := types.NamespacedName{
		Name: *desired.Spec.To.Name,
	}
	if desired.Spec.To.Namespace != nil {
		nsn.Namespace = *desired.Spec.To.Namespace
	} else {
		nsn.Namespace = desired.Namespace
	}

	secret := &corev1.Secret{}
	err := r.apiReader.Get(ctx, nsn, secret)
	if err != nil {
		return errors.Wrap(err, "unable to get existing secret")
	}

	// Update the field
	patch := client.StrategicMergeFrom(secret.DeepCopy())
	if secret.Data == nil {
		secret.Data = make(map[string][]byte, 1)
	}
	secret.Data[key] = []byte(sourceValue)

	ackrtlog.DebugFieldExport(r.log, desired, "patching target secret")
	err = r.kc.Patch(ctx, secret, patch)
	if err != nil {
		return err
	}
	ackrtlog.InfoFieldExport(r.log, desired, "patched target secret")

	return nil
}

func (r *fieldExportReconciler) FilterAllExports(
	ctx context.Context,
	gk metav1.GroupKind,
	nsn types.NamespacedName,
) ([]ackv1alpha1.FieldExport, error) {
	listed := &ackv1alpha1.FieldExportList{}
	opts := []client.ListOption{
		client.InNamespace(nsn.Namespace),
	}
	if err := r.apiReader.List(ctx, listed, opts...); err != nil {
		return []ackv1alpha1.FieldExport{}, err
	}

	exports := []ackv1alpha1.FieldExport{}
	for _, export := range listed.Items {
		// Ensure we are working with managed exports
		if !k8sctrlutil.ContainsFinalizer(&export, fieldExportFinalizerString) {
			continue
		}

		// Check the reference matches our source resource
		if !strings.EqualFold(export.Spec.From.Resource.Kind, gk.Kind) ||
			!strings.EqualFold(*export.Spec.From.Resource.Name, nsn.Name) {
			continue
		}

		exports = append(exports, export)
	}

	return exports, nil
}

// onError will patch the FieldExport with the given error and return the
// same error back
func (r *fieldExportReconciler) onError(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
	err error,
) error {
	var terminal *terminalError
	if errors.As(err, &terminal) {
		r.patchTerminalCondition(ctx, res, err)
	} else {
		r.patchRecoverableCondition(ctx, res, err)
	}
	return err
}

// onSuccess will patch the FieldExport with a synced condition and
// return any errors that occurred while patching
func (r *fieldExportReconciler) onSuccess(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
) error {
	return nil
}

// clearConditions removes all conditions from the field export CR.
func (r *fieldExportReconciler) clearConditions(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
) error {
	base := res.DeepCopy()
	res.Status.Conditions = []*ackv1alpha1.Condition{}
	return r.patchStatus(ctx, res, base)
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
		recoverableCondition.Status = corev1.ConditionTrue
		recoverableCondition.Message = &errMessage
	}

	return r.patchStatus(ctx, res, base)
}

// patchTerminalCondition updates the terminal condition status of the
// field export CR. The resource passed in the parameter gets updated with the
// conditions
func (r *fieldExportReconciler) patchTerminalCondition(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
	err error,
) error {
	base := res.DeepCopy()

	// Terminal condition
	var terminalCondition *ackv1alpha1.Condition = nil
	for _, condition := range res.Status.Conditions {
		if condition.Type == ackv1alpha1.ConditionTypeTerminal {
			terminalCondition = condition
			break
		}
	}

	if terminalCondition == nil {
		terminalCondition = &ackv1alpha1.Condition{
			Type: ackv1alpha1.ConditionTypeTerminal,
		}
		res.Status.Conditions = append(res.Status.Conditions, terminalCondition)
	}

	var errMessage string
	if err != nil {
		errMessage = err.Error()
		terminalCondition.Status = corev1.ConditionTrue
		terminalCondition.Message = &errMessage
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
	if !k8sctrlutil.ContainsFinalizer(res, fieldExportFinalizerString) {
		base := res.DeepCopy()
		k8sctrlutil.AddFinalizer(res, fieldExportFinalizerString)
		return r.patchMetadataAndSpec(ctx, res, base)
	}
	return nil
}

// markUnmanaged removes the supplied resource from management by ACK.
// It removes the finalizer string, patches the object in etcd and updates
// the object 'res' in parameter with latest metadata.
func (r *fieldExportReconciler) markUnmanaged(
	ctx context.Context,
	res *ackv1alpha1.FieldExport,
) error {
	if k8sctrlutil.ContainsFinalizer(res, fieldExportFinalizerString) {
		base := res.DeepCopy()
		k8sctrlutil.RemoveFinalizer(res, fieldExportFinalizerString)
		return r.patchMetadataAndSpec(ctx, res, base)
	}
	return nil
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

	var noRequeue *requeue.NoRequeue
	if errors.As(err, &noRequeue) {
		r.log.V(1).Info(
			"error did not need requeue",
			"error", noRequeue.Unwrap(),
		)
		return ctrlrt.Result{}, nil
	}

	var term *terminalError
	if errors.As(err, &term) {
		return ctrlrt.Result{}, nil
	}

	return ctrlrt.Result{}, err
}

// terminalError defines an error that should be considered terminal, and placed
// onto an ACK.Terminal condition
type terminalError struct {
	err error
}

func (e *terminalError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *terminalError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

var _ error = &terminalError{}

// NewFieldExportReconcilerForFieldExport returns a new FieldExportReconciler object
func NewFieldExportReconcilerForFieldExport(
	sc acktypes.ServiceController,
	log logr.Logger,
	cfg ackcfg.Config,
	metrics *ackmetrics.Metrics,
	cache ackrtcache.Caches,
) acktypes.FieldExportReconciler {
	return NewFieldExportReconcilerWithClient(sc, log, cfg, metrics, cache, nil, nil, nil, false)
}

// NewFieldExportReconcilerForAWSResource returns a new FieldExportReconciler object
func NewFieldExportReconcilerForAWSResource(
	sc acktypes.ServiceController,
	log logr.Logger,
	cfg ackcfg.Config,
	metrics *ackmetrics.Metrics,
	cache ackrtcache.Caches,
	rd acktypes.AWSResourceDescriptor,
	resourceReconciler bool,
) acktypes.FieldExportReconciler {
	return NewFieldExportReconcilerWithClient(sc, log, cfg, metrics, cache, nil, nil, rd, true)
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
	rd acktypes.AWSResourceDescriptor,
	resourceReconciler bool,
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
		rd:                 rd,
		resourceReconciler: resourceReconciler,
	}
}
