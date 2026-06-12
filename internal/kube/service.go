// Package kube wraps controller-runtime and kstatus to provide Server-Side Apply,
// deletion, conditional waiting, and JQ-based value extraction for Kubernetes objects.
package kube

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"

	interrors "github.com/ipaqsa/kube2e/internal/errors"
	"github.com/ipaqsa/kube2e/internal/tools/filter"
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
// attempts is the total number of calls; attempts < 2 disables retry (Steps = 0).
func retryBackoff(attempts int, backoff time.Duration) wait.Backoff {
	return wait.Backoff{
		Steps:    max(attempts-1, 0),
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

				tmp := obj.DeepCopyObject().(client.Object) //nolint:errcheck // it will be a client.Object anyway
				if err := s.cli.Get(ctx, client.ObjectKeyFromObject(obj), tmp); err != nil {
					if apierrors.IsNotFound(err) {
						// Cache only after a successful create so ClearApplied never
						// tries to delete a resource that was never applied.
						if err = s.cli.Apply(ctx, applyObj, client.ForceOwnership, client.FieldOwner(managerField)); err != nil {
							return err
						}

						if ensureOpts.ToCache {
							s.applied.Put(obj.GetName(), obj)
						}

						return nil
					}

					return fmt.Errorf("get object: %w", err)
				}

				return s.cli.Apply(ctx, applyObj, client.FieldOwner(managerField))
			})
		})
	})
}

// toApplyConfiguration converts an client.Object to an runtime.ApplyConfiguration.
func toApplyConfiguration(obj client.Object) (runtime.ApplyConfiguration, error) {
	raw, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("object is not unstructured: %T", obj)
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

	deleteOpts := new(DeleteOptions)
	for _, opt := range opts {
		opt(deleteOpts)
	}

	if err := retry.OnError(deleteOpts.Retry, retryAlways, func() error {
		return retry.OnError(retry.DefaultBackoff, apierrors.IsServerTimeout, func() error {
			return retry.OnError(retry.DefaultBackoff, apierrors.IsServiceUnavailable, func() error {
				s.applied.Delete(obj.GetName())

				if err := s.setNamespace(obj); err != nil {
					return fmt.Errorf("set namespace for the object '%s': %w", obj.GetName(), err)
				}

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
			// deletion mode: finish when kstatus says NotFound
			if waitOpts.onDeletion && ev.Resource.Status == status.NotFoundStatus {
				return nil
			}

			if ev.Resource == nil || ev.Resource.Identifier != id || ev.Resource.Resource == nil {
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

	checkOpts := new(CheckOptions)
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

// ClearApplied deletes every object that was added to the applied cache during
// the current test case and clears the cache entries as it goes.
func (s *Service) ClearApplied(ctx context.Context) error {
	s.logger.Debug("clear applied resources")

	if s.dryRun {
		s.logger.Debug("dry run enabled, skipping clear applied")
		return nil
	}

	for _, obj := range s.applied.Iter() {
		if err := s.Delete(ctx, obj); err != nil {
			return fmt.Errorf("delete the applied object '%s': %w", obj.GetName(), err)
		}
	}

	return nil
}
