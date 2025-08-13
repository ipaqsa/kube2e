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
)

// Service is the primary Kubernetes client for kube2e. It maintains a shared
// applied-resource cache used for automatic cleanup after each test case.
type Service struct {
	cli         client.Client
	namespace   string
	poller      *polling.StatusPoller
	disc        discovery.DiscoveryInterface
	applied     *safe.Store[client.Object]
	labels      map[string]string
	annotations map[string]string
	logger      *slog.Logger
}

// Option configures a Service at construction time.
type Option func(*Service)

// WithLabels adds default labels that are merged onto every object passed to Ensure.
func WithLabels(labels map[string]string) Option {
	return func(service *Service) {
		service.labels = labels
	}
}

// WithAnnotations adds default annotations that are merged onto every object passed to Ensure.
func WithAnnotations(annotations map[string]string) Option {
	return func(service *Service) {
		service.annotations = annotations
	}
}

// WithNamespace sets the fallback namespace for namespaced objects that do not
// specify one explicitly. Ignored when namespace is empty.
func WithNamespace(namespace string) Option {
	return func(service *Service) {
		if namespace != "" {
			service.namespace = namespace
		}
	}
}

// New builds a Service from cfg. It constructs a controller-runtime client with
// support for core types and CRDs, a kstatus poller for condition watching, and
// a discovery client for server version checks.
func New(cfg *rest.Config, logger *slog.Logger, opts ...Option) (*Service, error) {
	if cfg == nil {
		return nil, interrors.ErrNilRestConfig
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

	cli, err := client.New(cfg, client.Options{Scheme: sch})
	if err != nil {
		return nil, fmt.Errorf("create runtime client: %w", err)
	}

	svc := &Service{
		cli:         cli,
		namespace:   namespaceDefault,
		poller:      polling.NewStatusPoller(cli, cli.RESTMapper(), polling.Options{}),
		disc:        disc,
		applied:     safe.NewStore[client.Object](),
		annotations: make(map[string]string),
		labels:      make(map[string]string),
		logger:      logger.With("service", "kube"),
	}

	for _, opt := range opts {
		opt(svc)
	}

	svc.logger.Debug("service initialized")

	return svc, nil
}

// EnsureOptions collects per-call overrides for Ensure.
type EnsureOptions struct {
	Labels      map[string]string
	Annotations map[string]string
	ToCache     bool
}

// EnsureOptionFunc is a functional option for a single Ensure call.
type EnsureOptionFunc func(*EnsureOptions)

// WithEnsureLabels merges additional labels onto the object for this call only.
func WithEnsureLabels(labels map[string]string) EnsureOptionFunc {
	return func(o *EnsureOptions) {
		if len(labels) == 0 {
			return
		}

		for k, v := range labels {
			o.Labels[k] = v
		}
	}
}

// WithEnsureAnnotations merges additional annotations onto the object for this call only.
func WithEnsureAnnotations(annotations map[string]string) EnsureOptionFunc {
	return func(o *EnsureOptions) {
		if len(annotations) == 0 {
			return
		}

		for k, v := range annotations {
			o.Annotations[k] = v
		}
	}
}

// WithEnsureToCache controls whether a newly created object is added to the
// applied-resource cache for automatic cleanup. Default is true.
func WithEnsureToCache(toCache bool) EnsureOptionFunc {
	return func(o *EnsureOptions) {
		o.ToCache = toCache
	}
}

// Ensure creates or updates obj on the cluster using Server-Side Apply with
// field manager "kube2e". On first create the object is recorded in the
// applied cache so ClearApplied can delete it later. Retries on transient
// server-timeout and service-unavailable errors.
func (s *Service) Ensure(ctx context.Context, obj client.Object, opts ...EnsureOptionFunc) error {
	return retry.OnError(retry.DefaultBackoff, apierrors.IsServerTimeout, func() error {
		return retry.OnError(retry.DefaultBackoff, apierrors.IsServiceUnavailable, func() error {
			if obj == nil {
				return interrors.ErrNilObject
			}

			ensureOpts := &EnsureOptions{
				Labels:      s.labels,
				Annotations: s.annotations,
				ToCache:     true,
			}

			for _, opt := range opts {
				opt(ensureOpts)
			}

			labels := obj.GetLabels()
			if len(labels) == 0 {
				labels = make(map[string]string)
			}

			maps.Copy(labels, ensureOpts.Labels)

			obj.SetLabels(labels)

			annotations := obj.GetAnnotations()
			if len(annotations) == 0 {
				annotations = make(map[string]string)
			}

			maps.Copy(annotations, ensureOpts.Annotations)

			obj.SetAnnotations(annotations)

			if err := s.setNamespace(obj); err != nil {
				return fmt.Errorf("set namespace for the object '%s': %w", obj.GetName(), err)
			}

			s.logger.Debug("ensure object",
				"name", obj.GetName(),
				"gvk", obj.GetObjectKind().GroupVersionKind().String(),
				"namespace", obj.GetNamespace())

			tmp := obj.DeepCopyObject().(client.Object) //nolint:errcheck // it will be a client.Object anyway
			if err := s.cli.Get(ctx, client.ObjectKeyFromObject(obj), tmp); err != nil {
				if apierrors.IsNotFound(err) {
					// Cache only after a successful create so ClearApplied never
					// tries to delete a resource that was never applied.
					if err = s.cli.Patch(ctx, obj, client.Apply, client.FieldOwner(managerField), client.ForceOwnership); err != nil { //nolint:staticcheck // client.Apply deprecated; migrate to client.Client.Apply() after confirming new API
						return err
					}

					if ensureOpts.ToCache {
						s.applied.Put(obj.GetName(), obj)
					}

					return nil
				}

				return fmt.Errorf("get object: %w", err)
			}

			return s.cli.Patch(ctx, obj, client.Apply, client.FieldOwner(managerField)) //nolint:staticcheck // client.Apply deprecated; migrate to client.Client.Apply() after confirming new API
		})
	})
}

// Delete removes obj from the cluster. Not-found responses are treated as
// success. The object is removed from the applied cache before deletion.
// Retries on transient server-timeout and service-unavailable errors.
func (s *Service) Delete(ctx context.Context, obj client.Object) error {
	return retry.OnError(retry.DefaultBackoff, apierrors.IsServerTimeout, func() error {
		return retry.OnError(retry.DefaultBackoff, apierrors.IsServiceUnavailable, func() error {
			if obj == nil {
				return nil
			}

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
			}

			return nil
		})
	})
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

// Filter fetches the latest version of obj from the API server, evaluates the
// JQ expression against it, and returns the first result as a string.
func (s *Service) Filter(ctx context.Context, obj client.Object, expression string) (string, error) {
	if obj == nil {
		return "", interrors.ErrNilObject
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

	for _, obj := range s.applied.Iter() {
		if err := s.Delete(ctx, obj); err != nil {
			return fmt.Errorf("delete the applied object '%s': %w", obj.GetName(), err)
		}
	}

	return nil
}
