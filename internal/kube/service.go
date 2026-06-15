// Package kube wraps controller-runtime and kstatus to provide Server-Side Apply,
// deletion, conditional waiting, and JQ-based value extraction for Kubernetes objects.
package kube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"strings"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"

	interrors "github.com/ipaqsa/kube2e/internal/errors"
	"github.com/ipaqsa/kube2e/internal/tools/filter"
	"github.com/ipaqsa/kube2e/internal/tools/patch"
	"github.com/ipaqsa/kube2e/internal/tools/safe"
)

const (
	managerField = "kube2e"

	namespaceDefault = "default"

	// waitInterval is the default kstatus poll cadence used for delete-wait.
	waitInterval = 2 * time.Second
)

// Service is the primary Kubernetes client for kube2e. It maintains a shared
// applied-resource cache used for automatic cleanup after each test case.
type Service struct {
	cli       client.Client
	clientset kubernetes.Interface
	restCfg   *rest.Config
	namespace string
	poller    *polling.StatusPoller
	disc      discovery.DiscoveryInterface
	applied   *safe.Store[client.Object]
	logger    *slog.Logger
	dryRun    bool
}

// Option configures a Service at construction time.
type Option func(*Service)

// WithNamespace sets the fallback namespace for namespaced objects that do not
// specify one explicitly. Ignored when namespace is empty.
func WithNamespace(namespace string) Option {
	return func(service *Service) {
		if namespace != "" {
			service.namespace = namespace
		}
	}
}

// WithDryRun enables dry-run mode for the service, which prevents actual resource
// creation and deletion.
func WithDryRun() Option {
	return func(service *Service) {
		service.dryRun = true
	}
}

// New builds a Service from cfg. It constructs a controller-runtime client with
// support for core types and CRDs, a kstatus poller for condition watching, and
// a discovery client for server version checks.
func New(cfg *rest.Config, logger *slog.Logger, opts ...Option) (*Service, error) {
	svc := &Service{
		namespace: namespaceDefault,
		applied:   safe.NewStore[client.Object](),
		logger:    logger.With("service", "kube"),
	}

	for _, opt := range opts {
		opt(svc)
	}

	if svc.dryRun {
		svc.logger.Debug("dry run enabled")
		return svc, nil
	}

	// build a scheme that knows about core types and CRDs (apiextensions.k8s.io/v1).
	sch := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("add core scheme: %w", err)
	}

	if err := apiextensionsv1.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("add apiextensions scheme: %w", err)
	}

	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build HTTP client: %w", err)
	}

	disc, err := discovery.NewDiscoveryClientForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("build discovery client: %w", err)
	}

	svc.disc = disc

	clientset, err := kubernetes.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}

	svc.clientset = clientset
	svc.restCfg = cfg

	cli, err := client.New(cfg, client.Options{Scheme: sch})
	if err != nil {
		return nil, fmt.Errorf("create runtime client: %w", err)
	}

	svc.cli = cli
	svc.poller = polling.NewStatusPoller(cli, cli.RESTMapper(), polling.Options{})

	svc.logger.Debug("service initialized")

	return svc, nil
}

// retryAlways is passed to retry.OnError to retry on any error.
var retryAlways = func(_ error) bool { return true }

// retryBackoff builds a constant-interval wait.Backoff for user-configured retry.
// attempts is the total number of calls; Steps must be >= 1 or retry.OnError
// never invokes the function and silently returns nil.
func retryBackoff(attempts int, backoff time.Duration) wait.Backoff {
	return wait.Backoff{
		Steps:    max(attempts, 1),
		Duration: backoff,
		Factor:   1.0,
	}
}

// EnsureOptions collects per-call overrides for Ensure.
type EnsureOptions struct {
	Annotations map[string]string
	ToCache     bool
	Retry       wait.Backoff
}

func newEnsureOptions() *EnsureOptions {
	return &EnsureOptions{
		Annotations: make(map[string]string),
		ToCache:     true,
		Retry:       retryBackoff(1, 0),
	}
}

// EnsureOptionFunc is a functional option for a single Ensure call.
type EnsureOptionFunc func(*EnsureOptions)

// WithEnsureAnnotations merges additional annotations onto the object for this
// call only. Used by the engine to inject tracing annotations.
func WithEnsureAnnotations(annotations map[string]string) EnsureOptionFunc {
	return func(o *EnsureOptions) {
		if len(annotations) == 0 {
			return
		}

		maps.Copy(o.Annotations, annotations)
	}
}

// WithEnsureToCache controls whether a newly created object is added to the
// applied-resource cache for automatic cleanup. Default is true.
func WithEnsureToCache(toCache bool) EnsureOptionFunc {
	return func(o *EnsureOptions) {
		o.ToCache = toCache
	}
}

// WithEnsureRetry sets the application-level retry for this Ensure call.
// attempts < 2 disables retry; backoff of 0 means no sleep between attempts.
func WithEnsureRetry(attempts int, backoff time.Duration) EnsureOptionFunc {
	return func(o *EnsureOptions) {
		o.Retry = retryBackoff(attempts, backoff)
	}
}

// Ensure creates or updates obj on the cluster using Server-Side Apply with
// field manager "kube2e". On first create the object is recorded in the
// applied cache so ClearApplied can delete it later. Retries on transient
// server-timeout and service-unavailable errors.
func (s *Service) Ensure(ctx context.Context, obj client.Object, opts ...EnsureOptionFunc) error {
	if obj == nil {
		return interrors.ErrNilObject
	}

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping ensure")
		return nil
	}

	ensureOpts := newEnsureOptions()
	for _, opt := range opts {
		opt(ensureOpts)
	}

	return retry.OnError(ensureOpts.Retry, retryAlways, func() error {
		return retry.OnError(retry.DefaultBackoff, apierrors.IsServerTimeout, func() error {
			return retry.OnError(retry.DefaultBackoff, apierrors.IsServiceUnavailable, func() error {
				if err := s.setNamespace(obj); err != nil {
					return fmt.Errorf("set namespace for the object '%s': %w", obj.GetName(), err)
				}

				// Merge engine-injected annotations onto the object.
				if len(ensureOpts.Annotations) > 0 {
					annotations := obj.GetAnnotations()
					if len(annotations) == 0 {
						annotations = make(map[string]string)
					}

					maps.Copy(annotations, ensureOpts.Annotations)
					obj.SetAnnotations(annotations)
				}

				s.logger.Debug("ensure object",
					"name", obj.GetName(),
					"gvk", obj.GetObjectKind().GroupVersionKind().String(),
					"namespace", obj.GetNamespace())

				applyObj, err := toApplyConfiguration(obj)
				if err != nil {
					return fmt.Errorf("to apply configuration: %w", err)
				}

				// Probe existence so we only cache newly-created objects (so
				// Cleanup never tries to delete a resource we did not create).
				tmp := obj.DeepCopyObject().(client.Object) //nolint:errcheck // it will be a client.Object anyway

				err = s.cli.Get(ctx, client.ObjectKeyFromObject(obj), tmp)
				switch {
				case err == nil, apierrors.IsNotFound(err):
				default:
					return fmt.Errorf("get object: %w", err)
				}

				created := apierrors.IsNotFound(err)

				// Always force ownership: SSA is idempotent for create and
				// update, and forcing keeps updates from failing on fields this
				// manager already owns.
				if err = s.cli.Apply(ctx, applyObj, client.ForceOwnership, client.FieldOwner(managerField)); err != nil {
					return err
				}

				if created && ensureOpts.ToCache {
					s.applied.Put(appliedKey(obj), obj)
				}

				return nil
			})
		})
	})
}

// PatchOptions collects per-call overrides for Patch.
type PatchOptions struct {
	Retry wait.Backoff
}

// PatchOptionFunc is a functional option for a single Patch call.
type PatchOptionFunc func(*PatchOptions)

// WithPatchRetry sets the application-level retry for this Patch call.
func WithPatchRetry(attempts int, backoff time.Duration) PatchOptionFunc {
	return func(o *PatchOptions) {
		o.Retry = retryBackoff(attempts, backoff)
	}
}

// Patch applies RFC 6902 patches to the live object on the cluster: it fetches
// the current object, applies the patches client-side (creating missing parents
// on add), and writes the result back with optimistic concurrency. Operating on
// the live object preserves fields set by earlier steps, unlike re-applying a
// freshly rendered template.
func (s *Service) Patch(ctx context.Context, obj client.Object, patches patch.Patches, opts ...PatchOptionFunc) error {
	if obj == nil {
		return interrors.ErrNilObject
	}

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping patch")
		return nil
	}

	if err := s.setNamespace(obj); err != nil {
		return fmt.Errorf("set namespace for the object '%s': %w", obj.GetName(), err)
	}

	patchOpts := &PatchOptions{Retry: retryBackoff(1, 0)}
	for _, opt := range opts {
		opt(patchOpts)
	}

	applyOpts := jsonpatch.NewApplyOptions()
	applyOpts.EnsurePathExistsOnAdd = true

	gvk := obj.GetObjectKind().GroupVersionKind()
	key := client.ObjectKeyFromObject(obj)

	return retry.OnError(patchOpts.Retry, retryAlways, func() error {
		// Re-read and re-apply on conflict so a concurrent update does not fail
		// the whole patch.
		return retry.OnError(retry.DefaultBackoff, apierrors.IsConflict, func() error {
			live := &unstructured.Unstructured{}
			live.SetGroupVersionKind(gvk)

			if err := s.cli.Get(ctx, key, live); err != nil {
				return fmt.Errorf("get object: %w", err)
			}

			patched, err := patches.Apply(live, applyOpts)
			if err != nil {
				return fmt.Errorf("apply patches to object '%s': %w", obj.GetName(), err)
			}

			s.logger.Debug("patch object", "name", obj.GetName(), "gvk", gvk.String(), "namespace", obj.GetNamespace())

			return s.cli.Update(ctx, patched)
		})
	})
}

// appliedKey builds a cache key unique across namespace, kind, and name so two
// objects with the same name but different kind or namespace (e.g. a Service and
// a Deployment both named "app") do not overwrite each other in the applied
// cache and silently leak past Cleanup.
func appliedKey(obj client.Object) string {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return strings.Join([]string{obj.GetNamespace(), gvk.Kind, obj.GetName()}, "/")
}

// toApplyConfiguration converts a client.Object to a runtime.ApplyConfiguration.
// Unstructured objects are used directly; typed objects (e.g. the engine-built
// Namespace) are converted to unstructured first. Typed objects must carry their
// TypeMeta so the resulting apply keeps apiVersion/kind for Server-Side Apply.
func toApplyConfiguration(obj client.Object) (runtime.ApplyConfiguration, error) {
	raw, ok := obj.(*unstructured.Unstructured)
	if !ok {
		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, fmt.Errorf("convert %T to unstructured: %w", obj, err)
		}

		raw = &unstructured.Unstructured{Object: data}
	}

	return client.ApplyConfigurationFromUnstructured(raw), nil
}

// DeleteOptions collects per-call overrides for Delete.
type DeleteOptions struct {
	// Wait blocks until the object is gone from the cluster when true.
	Wait         bool
	WaitTimeout  time.Duration
	WaitInterval time.Duration
	Retry        wait.Backoff
}

// DeleteOptionFunc is a functional option for a single Delete call.
type DeleteOptionFunc func(*DeleteOptions)

// WithDeleteWait makes Delete block until the object disappears from the
// cluster. timeout sets the hard deadline for the watch; zero means no deadline.
func WithDeleteWait(wait bool, timeout time.Duration) DeleteOptionFunc {
	return func(o *DeleteOptions) {
		o.Wait = wait
		o.WaitTimeout = timeout
	}
}

// WithDeleteInterval sets the kstatus poll interval for the post-delete wait.
// Zero values are ignored and the service default is used.
func WithDeleteInterval(interval time.Duration) DeleteOptionFunc {
	return func(o *DeleteOptions) {
		o.WaitInterval = interval
	}
}

// WithDeleteRetry sets the application-level retry for this Delete call.
func WithDeleteRetry(attempts int, backoff time.Duration) DeleteOptionFunc {
	return func(o *DeleteOptions) {
		o.Retry = retryBackoff(attempts, backoff)
	}
}

// Delete removes obj from the cluster. Not-found responses are treated as
// success. The object is removed from the applied cache before deletion.
// Retries on transient server-timeout and service-unavailable errors.
func (s *Service) Delete(ctx context.Context, obj client.Object, opts ...DeleteOptionFunc) error {
	if obj == nil {
		return nil
	}

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping delete")
		return nil
	}

	deleteOpts := &DeleteOptions{Retry: retryBackoff(1, 0)}
	for _, opt := range opts {
		opt(deleteOpts)
	}

	if err := retry.OnError(deleteOpts.Retry, retryAlways, func() error {
		return retry.OnError(retry.DefaultBackoff, apierrors.IsServerTimeout, func() error {
			return retry.OnError(retry.DefaultBackoff, apierrors.IsServiceUnavailable, func() error {
				if err := s.setNamespace(obj); err != nil {
					return fmt.Errorf("set namespace for the object '%s': %w", obj.GetName(), err)
				}

				s.applied.Delete(appliedKey(obj))

				s.logger.Debug("delete object",
					"name", obj.GetName(),
					"gvk", obj.GetObjectKind().GroupVersionKind().String(),
					"namespace", obj.GetNamespace())

				if err := s.cli.Delete(ctx, obj); err != nil {
					if apierrors.IsNotFound(err) {
						s.logger.Warn("object not found",
							"name", obj.GetName(),
							"gvk", obj.GetObjectKind().GroupVersionKind().String(),
							"namespace", obj.GetNamespace())

						return nil
					}

					return err
				}

				return nil
			})
		})
	}); err != nil {
		return err
	}

	if !deleteOpts.Wait {
		return nil
	}

	interval := deleteOpts.WaitInterval
	if interval <= 0 {
		interval = waitInterval
	}

	return s.Wait(ctx, obj,
		WithOnDeletion(true),
		WithWaitTimeout(deleteOpts.WaitTimeout),
		WithWaitInterval(interval))
}

func (s *Service) setNamespace(obj client.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return interrors.ErrObjectNoGVK
	}

	mapping, err := s.cli.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("REST mapping for the object '%s': %w", gvk.String(), err)
	}

	if mapping.Scope.Name() == meta.RESTScopeNameNamespace && obj.GetNamespace() == "" {
		obj.SetNamespace(s.namespace)
	}

	return nil
}

// WaitOptions controls polling behavior for Wait.
type WaitOptions struct {
	interval   time.Duration
	timeout    time.Duration
	conditions []string
	onDeletion bool
}

// WaitOptionFunc is a functional option for a single Wait call.
type WaitOptionFunc func(*WaitOptions)

// WithWaitInterval sets the kstatus poll interval. Zero values are ignored.
func WithWaitInterval(interval time.Duration) WaitOptionFunc {
	return func(o *WaitOptions) {
		if interval == 0 {
			return
		}

		o.interval = interval
	}
}

// WithWaitTimeout sets a hard deadline for the Wait call. Zero values are ignored.
func WithWaitTimeout(timeout time.Duration) WaitOptionFunc {
	return func(o *WaitOptions) {
		if timeout == 0 {
			return
		}

		o.timeout = timeout
	}
}

// WithWaitConditions sets the JQ expressions that must all return true before
// Wait returns successfully.
func WithWaitConditions(conditions []string) WaitOptionFunc {
	return func(o *WaitOptions) {
		o.conditions = conditions
	}
}

// WithOnDeletion makes Wait succeed once the object reaches NotFoundStatus,
// instead of waiting for it to satisfy conditions.
func WithOnDeletion(onDeletion bool) WaitOptionFunc {
	return func(o *WaitOptions) {
		o.onDeletion = onDeletion
	}
}

// Wait polls obj using kstatus until all JQ conditions pass, the object is
// deleted (when WithOnDeletion is set), or the context/timeout expires.
func (s *Service) Wait(ctx context.Context, obj client.Object, opts ...WaitOptionFunc) error { //nolint:gocyclo // not worth simplifying
	if obj == nil {
		return interrors.ErrNilObject
	}

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping wait")
		return nil
	}

	if err := s.setNamespace(obj); err != nil {
		return fmt.Errorf("set namespace for the object '%s': %w", obj.GetName(), err)
	}

	s.logger.Debug("wait for conditions",
		"name", obj.GetName(),
		"gvk", obj.GetObjectKind().GroupVersionKind().String(),
		"namespace", obj.GetNamespace())

	id := object.ObjMetadata{
		GroupKind: obj.GetObjectKind().GroupVersionKind().GroupKind(),
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	waitOpts := new(WaitOptions)
	for _, opt := range opts {
		opt(waitOpts)
	}

	// enforce timeout
	if waitOpts.timeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, waitOpts.timeout)

		defer cancel()
	}

	pollOpts := polling.PollOptions{PollInterval: waitOpts.interval}
	for ev := range s.poller.Poll(ctx, object.ObjMetadataSet{id}, pollOpts) {
		switch ev.Type {
		case event.ErrorEvent:
			return ev.Error

		case event.SyncEvent, event.ResourceUpdateEvent:
			// ev.Resource is nil for SyncEvent; guard before any deref.
			if ev.Resource == nil || ev.Resource.Identifier != id {
				continue
			}

			// deletion mode: finish when kstatus says NotFound (its manifest
			// Resource is nil, so this must run before the nil-manifest guard).
			if waitOpts.onDeletion && ev.Resource.Status == status.NotFoundStatus {
				return nil
			}

			if ev.Resource.Resource == nil {
				continue
			}

			if len(waitOpts.conditions) == 0 {
				return nil
			}

			allPassed := true

			for _, cond := range waitOpts.conditions {
				pass, err := filter.Pass(ctx, ev.Resource.Resource, cond)
				if err != nil {
					return fmt.Errorf("filter resource update by the condition '%s': %w", cond, err)
				}

				if !pass {
					allPassed = false
					break
				}
			}

			if allPassed {
				return nil
			}

		default:
			continue
		}
	}

	return ctx.Err()
}

// CheckOptions collects per-call overrides for Check.
type CheckOptions struct {
	Retry wait.Backoff
}

// CheckOptionFunc is a functional option for a single Check call.
type CheckOptionFunc func(*CheckOptions)

// WithCheckRetry sets the application-level retry for this Check call.
func WithCheckRetry(attempts int, backoff time.Duration) CheckOptionFunc {
	return func(o *CheckOptions) {
		o.Retry = retryBackoff(attempts, backoff)
	}
}

// Check fetches the current state of obj and evaluates all JQ conditions against
// it. Returns nil when all conditions pass. An empty conditions slice succeeds as
// long as the object exists.
func (s *Service) Check(ctx context.Context, obj client.Object, conditions []string, opts ...CheckOptionFunc) error {
	if obj == nil {
		return interrors.ErrNilObject
	}

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping check")
		return nil
	}

	checkOpts := &CheckOptions{Retry: retryBackoff(1, 0)}
	for _, opt := range opts {
		opt(checkOpts)
	}

	return retry.OnError(checkOpts.Retry, retryAlways, func() error {
		if err := s.setNamespace(obj); err != nil {
			return fmt.Errorf("set namespace for object '%s': %w", obj.GetName(), err)
		}

		s.logger.Debug("check conditions",
			"name", obj.GetName(),
			"gvk", obj.GetObjectKind().GroupVersionKind().String(),
			"namespace", obj.GetNamespace())

		if err := s.cli.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return fmt.Errorf("get object '%s': %w", obj.GetName(), err)
		}

		parsed, err := toUnstructured(obj)
		if err != nil {
			return fmt.Errorf("convert object '%s' to unstructured: %w", obj.GetName(), err)
		}

		for _, cond := range conditions {
			pass, err := filter.Pass(ctx, parsed, cond)
			if err != nil {
				return fmt.Errorf("evaluate condition '%s': %w", cond, err)
			}

			if !pass {
				return fmt.Errorf("condition not satisfied: %s", cond)
			}
		}

		return nil
	})
}

// Filter fetches the latest version of obj from the API server, evaluates the
// JQ expression against it, and returns the first result as a string.
func (s *Service) Filter(ctx context.Context, obj client.Object, expression string) (string, error) {
	if obj == nil {
		return "", interrors.ErrNilObject
	}

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping filter")
		return "", nil
	}

	if err := s.setNamespace(obj); err != nil {
		return "", fmt.Errorf("set namespace for the object '%s': %w", obj.GetName(), err)
	}

	s.logger.Debug("filter object",
		"name", obj.GetName(),
		"gvk", obj.GetObjectKind().GroupVersionKind().String(),
		"namespace", obj.GetNamespace())

	if err := s.cli.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return "", fmt.Errorf("get the object '%s': %w", obj.GetName(), err)
	}

	parsed, err := toUnstructured(obj)
	if err != nil {
		return "", fmt.Errorf("convert the object '%s' to unstructured: %w", obj.GetName(), err)
	}

	value, err := filter.Filter(ctx, parsed, expression)
	if err != nil {
		return "", fmt.Errorf("filter object by the exp '%s': %w", expression, err)
	}

	return value, nil
}

func toUnstructured(obj client.Object) (*unstructured.Unstructured, error) {
	parsed, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{Object: parsed}
	u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())

	return u, nil
}

// defaultLogsTailLines is the number of log lines fetched from the tail on each
// poll tick. Keeps each fetch cheap while covering typical test output windows.
const defaultLogsTailLines int64 = 200

// LogsMatch controls how log contents are evaluated across pods.
type LogsMatch string

const (
	// LogsMatchAny succeeds when at least one pod's logs contain the string (default).
	LogsMatchAny LogsMatch = "any"
	// LogsMatchAll succeeds when every pod's logs contain the string.
	LogsMatchAll LogsMatch = "all"
	// LogsMatchNone succeeds when no pod's logs contain the string.
	LogsMatchNone LogsMatch = "none"
)

// LogsOptions controls polling behavior for LogsContains.
type LogsOptions struct {
	interval  time.Duration
	timeout   time.Duration
	container string
	match     LogsMatch
}

func newLogsOptions() *LogsOptions {
	return &LogsOptions{
		interval: waitInterval,
		timeout:  2 * time.Minute,
		match:    LogsMatchAny,
	}
}

// LogsOptionFunc is a functional option for a single LogsContains call.
type LogsOptionFunc func(*LogsOptions)

// WithLogsInterval sets the poll interval for log checks. Zero values are ignored.
func WithLogsInterval(interval time.Duration) LogsOptionFunc {
	return func(o *LogsOptions) {
		if interval > 0 {
			o.interval = interval
		}
	}
}

// WithLogsTimeout sets a hard deadline for log polling. Zero values are ignored.
func WithLogsTimeout(timeout time.Duration) LogsOptionFunc {
	return func(o *LogsOptions) {
		if timeout > 0 {
			o.timeout = timeout
		}
	}
}

// WithLogsContainer restricts log streaming to the named container. When empty,
// Kubernetes picks the first (or only) container in the pod.
func WithLogsContainer(container string) LogsOptionFunc {
	return func(o *LogsOptions) {
		o.container = container
	}
}

// WithLogsMatch sets the pod match policy. Empty values are ignored and the
// default (any) is preserved.
func WithLogsMatch(match LogsMatch) LogsOptionFunc {
	return func(o *LogsOptions) {
		if match != "" {
			o.match = match
		}
	}
}

// workloadKinds are the workload types Exec and Logs resolve to pods via their
// spec.selector.matchLabels.
var workloadKinds = map[string]bool{"Deployment": true, "ReplicaSet": true, "StatefulSet": true}

// PodTarget identifies the pods an Exec or Logs action operates on. Exactly one
// of Object or (Kind + LabelSelector) is used: Object resolves a Pod or workload
// by name; Kind + LabelSelector selects existing objects by label.
type PodTarget struct {
	// Object is a Pod or workload (Deployment/ReplicaSet/StatefulSet) to resolve
	// pods from. When nil, Kind and LabelSelector are used instead.
	Object client.Object
	// Kind is the object kind to select when Object is nil.
	Kind string
	// LabelSelector filters objects of Kind, e.g. "app=nginx".
	LabelSelector string
}

// validate reports whether the target identifies pods one way or the other.
func (t PodTarget) validate() error {
	if t.Object == nil && (t.Kind == "" || t.LabelSelector == "") {
		return interrors.ErrNilObject
	}

	return nil
}

// describe returns a short human-readable identifier for logs and errors.
func (t PodTarget) describe() string {
	if t.Object != nil {
		return fmt.Sprintf("%s %q", t.Object.GetObjectKind().GroupVersionKind().Kind, t.Object.GetName())
	}

	return fmt.Sprintf("%s [%s]", t.Kind, t.LabelSelector)
}

// podSelector resolves target to a namespace and either a concrete pod name (a
// named Pod) or a pod label selector. Exactly one of podName or selector is
// non-empty.
func (s *Service) podSelector(ctx context.Context, target PodTarget) (ns, podName, selector string, err error) {
	if target.Object != nil {
		gvk := target.Object.GetObjectKind().GroupVersionKind()

		ns = target.Object.GetNamespace()
		if ns == "" {
			ns = s.namespace
		}

		if gvk.Kind == "Pod" {
			return ns, target.Object.GetName(), "", nil
		}

		if !workloadKinds[gvk.Kind] {
			return "", "", "", fmt.Errorf("unsupported kind %q; use Pod, Deployment, ReplicaSet, or StatefulSet", gvk.Kind)
		}

		live := &unstructured.Unstructured{}
		live.SetGroupVersionKind(gvk)

		// Use the resolved namespace (the rendered object may not carry one)
		// so the workload is fetched from the case namespace, not "".
		key := client.ObjectKey{Namespace: ns, Name: target.Object.GetName()}
		if err = s.cli.Get(ctx, key, live); err != nil {
			return "", "", "", fmt.Errorf("get %s: %w", target.describe(), err)
		}

		selector, err = matchLabelsSelector(live)
		if err != nil {
			return "", "", "", fmt.Errorf("%s: %w", target.describe(), err)
		}

		return ns, "", selector, nil
	}

	ns = s.namespace

	if target.Kind == "Pod" {
		return ns, "", target.LabelSelector, nil
	}

	if !workloadKinds[target.Kind] {
		return "", "", "", fmt.Errorf("unsupported kind %q; use Pod, Deployment, ReplicaSet, or StatefulSet", target.Kind)
	}

	selector, err = s.workloadPodSelector(ctx, target.Kind, target.LabelSelector, ns)
	if err != nil {
		return "", "", "", err
	}

	return ns, "", selector, nil
}

// matchLabelsSelector reads spec.selector.matchLabels from a workload object and
// returns it as a label selector string.
func matchLabelsSelector(obj *unstructured.Unstructured) (string, error) {
	matchLabels, found, err := unstructured.NestedStringMap(obj.Object, "spec", "selector", "matchLabels")
	if err != nil {
		return "", fmt.Errorf("read spec.selector.matchLabels: %w", err)
	}

	if !found || len(matchLabels) == 0 {
		return "", errors.New("no spec.selector.matchLabels")
	}

	return labels.Set(matchLabels).String(), nil
}

// workloadPodSelector lists workloads of kind matching labelSelector in ns and
// returns the pod selector of the first match.
func (s *Service) workloadPodSelector(ctx context.Context, kind, labelSelector, ns string) (string, error) {
	sel, err := labels.Parse(labelSelector)
	if err != nil {
		return "", fmt.Errorf("parse label selector '%s': %w", labelSelector, err)
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind + "List"})

	if err = s.cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return "", fmt.Errorf("list %s by '%s': %w", kind, labelSelector, err)
	}

	if len(list.Items) == 0 {
		return "", fmt.Errorf("no %s matching '%s' in namespace '%s'", kind, labelSelector, ns)
	}

	return matchLabelsSelector(&list.Items[0])
}

// LogsContains polls the logs of target until they contain expected, the context
// is canceled, or the timeout expires. The target is a Pod or workload object, or
// a kind plus label selector — both resolve to their pods. Returns
// ErrLogsNotContain on timeout.
func (s *Service) LogsContains(ctx context.Context, target PodTarget, expected string, opts ...LogsOptionFunc) error {
	if err := target.validate(); err != nil {
		return err
	}

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping logs check")

		return nil
	}

	logsOpts := newLogsOptions()
	for _, opt := range opts {
		opt(logsOpts)
	}

	desc := target.describe()
	s.logger.Debug("polling logs", "target", desc, "contains", expected)

	tail := defaultLogsTailLines
	podLogOpts := &corev1.PodLogOptions{Container: logsOpts.container, TailLines: &tail}

	pollErr := wait.PollUntilContextTimeout(ctx, logsOpts.interval, logsOpts.timeout, true,
		func(ctx context.Context) (bool, error) {
			reqs, err := s.resolvePodsForLogs(ctx, target, podLogOpts)
			if err != nil {
				s.logger.Debug("could not resolve pods for logs", "target", desc, "error", err)

				return false, nil
			}

			matched, checked := 0, 0

			for _, req := range reqs {
				stream, streamErr := req.Stream(ctx)
				if streamErr != nil {
					s.logger.Debug("pod logs not yet available", "target", desc, "error", streamErr)

					continue
				}

				data, readErr := io.ReadAll(stream)

				if err := stream.Close(); err != nil {
					s.logger.Debug("close pod log stream error", "target", desc, "error", err)
				}

				if readErr != nil {
					s.logger.Debug("read pod logs error", "target", desc, "error", readErr)

					continue
				}

				checked++

				if strings.Contains(string(data), expected) {
					matched++
				}
			}

			if checked == 0 {
				return false, nil
			}

			switch logsOpts.match {
			case LogsMatchAll:
				return matched == checked, nil
			case LogsMatchNone:
				return matched == 0, nil
			default:
				return matched > 0, nil
			}
		})
	if pollErr != nil {
		return fmt.Errorf("%w: %q not found in logs of %s", interrors.ErrLogsNotContain, expected, desc)
	}

	return nil
}

// resolvePodsForLogs builds a log request for every available pod of target.
// Failed and Unknown pods are excluded.
func (s *Service) resolvePodsForLogs(ctx context.Context, target PodTarget, logOpts *corev1.PodLogOptions) ([]*rest.Request, error) {
	ns, podName, selector, err := s.podSelector(ctx, target)
	if err != nil {
		return nil, err
	}

	if podName != "" {
		return []*rest.Request{s.clientset.CoreV1().Pods(ns).GetLogs(podName, logOpts)}, nil
	}

	pods, err := s.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list pods by '%s': %w", selector, err)
	}

	reqs := make([]*rest.Request, 0, len(pods.Items))

	for i := range pods.Items {
		pod := &pods.Items[i]

		switch pod.Status.Phase {
		case corev1.PodFailed, corev1.PodUnknown:
			// skip degraded pods
		default:
			reqs = append(reqs, s.clientset.CoreV1().Pods(ns).GetLogs(pod.Name, logOpts))
		}
	}

	if len(reqs) == 0 {
		return nil, fmt.Errorf("no available pods found for selector '%s' in namespace '%s'", selector, ns)
	}

	return reqs, nil
}

// ExecOptions controls how Exec runs a command inside a pod.
type ExecOptions struct {
	container string
	retry     wait.Backoff
	timeout   time.Duration
}

// ExecOptionFunc is a functional option for a single Exec call.
type ExecOptionFunc func(*ExecOptions)

// WithExecContainer restricts exec to the named container. When empty,
// Kubernetes picks the first (or only) container in the pod.
func WithExecContainer(container string) ExecOptionFunc {
	return func(o *ExecOptions) {
		o.container = container
	}
}

// WithExecRetry sets the application-level retry for this Exec call.
func WithExecRetry(attempts int, backoff time.Duration) ExecOptionFunc {
	return func(o *ExecOptions) {
		o.retry = retryBackoff(attempts, backoff)
	}
}

// WithExecTimeout sets a hard deadline for the exec call. Zero values are ignored.
func WithExecTimeout(timeout time.Duration) ExecOptionFunc {
	return func(o *ExecOptions) {
		if timeout > 0 {
			o.timeout = timeout
		}
	}
}

// Exec runs command inside the resolved pod and succeeds when the command
// exits with code zero. obj may be a Pod, Deployment, ReplicaSet, or
// StatefulSet — workload types resolve to a single Running pod.
func (s *Service) Exec(ctx context.Context, target PodTarget, command []string, opts ...ExecOptionFunc) error {
	if err := target.validate(); err != nil {
		return err
	}

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping exec")

		return nil
	}

	execOpts := &ExecOptions{retry: retryBackoff(1, 0)}

	for _, opt := range opts {
		opt(execOpts)
	}

	if execOpts.timeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, execOpts.timeout)

		defer cancel()
	}

	return retry.OnError(execOpts.retry, retryAlways, func() error {
		ns, podName, err := s.resolvePodForExec(ctx, target)
		if err != nil {
			return fmt.Errorf("resolve pod for exec: %w", err)
		}

		req := s.clientset.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(podName).
			Namespace(ns).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Command:   command,
				Container: execOpts.container,
				Stdout:    true,
				Stderr:    true,
			}, clientgoscheme.ParameterCodec)

		executor, err := remotecommand.NewWebSocketExecutor(s.restCfg, "POST", req.URL().String())
		if err != nil {
			return fmt.Errorf("create exec executor: %w", err)
		}

		var stdout, stderr bytes.Buffer

		if streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: &stdout,
			Stderr: &stderr,
		}); streamErr != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return fmt.Errorf("exec %v in pod %q: %w: %s", command, podName, streamErr, msg)
			}

			return fmt.Errorf("exec %v in pod %q: %w", command, podName, streamErr)
		}

		return nil
	})
}

// resolvePodForExec resolves target to the namespace and name of a single
// Running pod. A named Pod is used directly; workloads and label selectors
// resolve to the first Running pod.
func (s *Service) resolvePodForExec(ctx context.Context, target PodTarget) (string, string, error) {
	ns, podName, selector, err := s.podSelector(ctx, target)
	if err != nil {
		return "", "", err
	}

	if podName != "" {
		return ns, podName, nil
	}

	pods, err := s.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", "", fmt.Errorf("list pods by '%s': %w", selector, err)
	}

	for i := range pods.Items {
		if pods.Items[i].Status.Phase == corev1.PodRunning {
			return ns, pods.Items[i].Name, nil
		}
	}

	return "", "", fmt.Errorf("no running pod found for selector '%s' in namespace '%s'", selector, ns)
}

// GetVersion returns the Kubernetes API server GitVersion string (e.g., "v1.33.2").
// It uses the discovery client built from the same rest.Config and HTTP client as the controller-runtime client.
func (s *Service) GetVersion(_ context.Context) (string, error) {
	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping get version")
		return "dev", nil
	}

	info, err := s.disc.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("get server version: %w", err)
	}

	return info.GitVersion, nil
}

// Cleanup deletes every object that was added to the applied cache during
// the current test case and clears the cache entries as it goes.
func (s *Service) Cleanup(ctx context.Context) error {
	s.logger.Debug("clear applied resources")

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping clear applied")
		return nil
	}

	// Best-effort: attempt every object and aggregate failures so one failed
	// delete does not leave the remaining resources leaked in the cluster.
	var errs []error

	for _, obj := range s.applied.Iter() {
		if err := s.Delete(ctx, obj); err != nil {
			errs = append(errs, fmt.Errorf("delete the applied object '%s': %w", obj.GetName(), err))
		}
	}

	return errors.Join(errs...)
}
