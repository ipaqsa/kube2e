package kube

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"kube2e/internal/tools/filter"
)

const (
	managerField     = "kube2e"
	namespaceDefault = "default"

	defaultWaitInterval = 2 * time.Second
	defaultWaitTimeout  = 2 * time.Minute
)

type Service struct {
	cli         client.Client
	namespace   string
	mapper      meta.RESTMapper
	poller      *polling.StatusPoller
	labels      map[string]string
	annotations map[string]string
	disc        discovery.DiscoveryInterface
	applied     map[string]client.Object
	logger      *slog.Logger
}

type Option func(*Service)

func WithLabels(labels map[string]string) Option {
	return func(service *Service) {
		service.labels = labels
	}
}

func WithAnnotations(annotations map[string]string) Option {
	return func(service *Service) {
		service.annotations = annotations
	}
}

func WithNamespace(namespace string) Option {
	return func(service *Service) {
		if namespace != "" {
			service.namespace = namespace
		}
	}
}

func New(cfg *rest.Config, logger *slog.Logger, opts ...Option) (*Service, error) {
	if cfg == nil {
		return nil, errors.New("rest config is nil")
	}

	// Build a scheme that knows about core types and CRDs (apiextensions.k8s.io/v1).
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

	// Use a dynamic REST mapper so newly-registered CRDs are discoverable at runtime.
	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("build REST mapper: %w", err)
	}

	cli, err := client.New(cfg, client.Options{Scheme: sch, Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("create runtime client: %w", err)
	}

	svc := &Service{
		cli:         cli,
		mapper:      mapper,
		disc:        disc,
		logger:      logger.With("service", "kube"),
		poller:      polling.NewStatusPoller(cli, mapper, polling.Options{}),
		namespace:   namespaceDefault,
		applied:     make(map[string]client.Object),
		annotations: make(map[string]string),
		labels:      make(map[string]string),
	}

	for _, opt := range opts {
		opt(svc)
	}

	svc.logger.Debug("service initialized")

	return svc, nil
}

type EnsureOptions struct {
	Labels      map[string]string
	Annotations map[string]string
	ToCache     bool
}

type EnsureOptionFunc func(*EnsureOptions)

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

func WithEnsureToCache(toCache bool) EnsureOptionFunc {
	return func(o *EnsureOptions) {
		o.ToCache = toCache
	}
}

func (s *Service) Ensure(ctx context.Context, obj client.Object, opts ...EnsureOptionFunc) error {
	return retry.OnError(retry.DefaultBackoff, apierrors.IsServiceUnavailable, func() error {
		if obj == nil {
			return errors.New("nil object")
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

		for key, value := range ensureOpts.Labels {
			labels[key] = value
		}
		obj.SetLabels(labels)

		annotations := obj.GetAnnotations()
		if len(annotations) == 0 {
			annotations = make(map[string]string)
		}

		for key, value := range ensureOpts.Annotations {
			annotations[key] = value
		}
		obj.SetAnnotations(annotations)

		if err := s.setNamespace(obj); err != nil {
			return fmt.Errorf("set namespace for the '%s' object: %w", obj.GetName(), err)
		}

		s.logger.Debug("ensure object",
			"name", obj.GetName(),
			"gvk", obj.GetObjectKind().GroupVersionKind().String(),
			"namespace", obj.GetNamespace())

		tmp := obj.DeepCopyObject().(client.Object)
		if err := s.cli.Get(ctx, client.ObjectKeyFromObject(obj), tmp); err != nil {
			if apierrors.IsNotFound(err) {
				if ensureOpts.ToCache {
					// save applied object
					s.applied[obj.GetName()] = obj
				}

				return s.cli.Create(ctx, obj, client.FieldOwner(managerField))
			}

			return fmt.Errorf("get object: %w", err)
		}

		return s.cli.Patch(ctx, obj, client.Apply, client.FieldOwner(managerField))
	})
}

func (s *Service) Delete(ctx context.Context, obj client.Object) error {
	return retry.OnError(retry.DefaultBackoff, apierrors.IsServiceUnavailable, func() error {
		if obj == nil {
			return nil
		}

		delete(s.applied, obj.GetName())

		if err := s.setNamespace(obj); err != nil {
			return fmt.Errorf("set namespace for the '%s' object: %w", obj.GetName(), err)
		}

		s.logger.Debug("delete object",
			"name", obj.GetName(),
			"gvk", obj.GetObjectKind().GroupVersionKind().String(),
			"namespace", obj.GetNamespace())

		return s.cli.Delete(ctx, obj)
	})
}

func (s *Service) setNamespace(obj client.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return fmt.Errorf("object GVK is empty")
	}

	mapping, err := s.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("REST mapping for the '%s' object: %w", gvk.String(), err)
	}

	if mapping.Scope.Name() == meta.RESTScopeNameNamespace && obj.GetNamespace() == "" {
		obj.SetNamespace(s.namespace)
	}

	return nil
}

type WaitOptions struct {
	interval   time.Duration
	timeout    time.Duration
	condition  string
	onDeletion bool
}

type WaitOptionFunc func(*WaitOptions)

func WithWaitInterval(interval time.Duration) WaitOptionFunc {
	return func(o *WaitOptions) {
		if interval == 0 {
			return
		}

		o.interval = interval
	}
}

func WithWaitTimeout(timeout time.Duration) WaitOptionFunc {
	return func(o *WaitOptions) {
		if timeout == 0 {
			return
		}

		o.timeout = timeout
	}
}

func WithWaitCondition(condition string) WaitOptionFunc {
	return func(o *WaitOptions) {
		o.condition = condition
	}
}

func WithOnDeletion(onDeletion bool) WaitOptionFunc {
	return func(o *WaitOptions) {
		o.onDeletion = onDeletion
	}
}

func (s *Service) Wait(ctx context.Context, obj client.Object, opts ...WaitOptionFunc) error {
	if obj == nil {
		return errors.New("object is nil")
	}

	if err := s.setNamespace(obj); err != nil {
		return fmt.Errorf("set namespace for the '%s' object: %w", obj.GetName(), err)
	}

	s.logger.Debug("wait object",
		"name", obj.GetName(),
		"gvk", obj.GetObjectKind().GroupVersionKind().String(),
		"namespace", obj.GetNamespace())

	id := object.ObjMetadata{
		GroupKind: obj.GetObjectKind().GroupVersionKind().GroupKind(),
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	waitOpts := &WaitOptions{
		interval: defaultWaitInterval,
		timeout:  defaultWaitTimeout,
	}

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

		case event.ResourceUpdateEvent:
			if ev.Resource == nil || ev.Resource.Identifier != id || ev.Resource.Resource == nil {
				continue
			}

			// deletion mode: finish when kstatus says NotFound
			if waitOpts.onDeletion && ev.Resource.Status == status.NotFoundStatus {
				return nil
			}

			if len(waitOpts.condition) == 0 {
				return nil
			}

			pass, err := filter.Filter(ctx, waitOpts.condition, ev.Resource.Resource)
			if err != nil {
				return fmt.Errorf("filter resource update: %w", err)
			}
			if pass {
				return nil
			}

		default:
			continue
		}
	}

	return ctx.Err()
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

func (s *Service) GetApplied() map[string]client.Object {
	return s.applied
}

func (s *Service) ClearApplied(ctx context.Context) error {
	s.logger.Debug("clear applied resources")
	for _, obj := range s.applied {
		if err := s.Delete(ctx, obj); err != nil {
			return fmt.Errorf("delete the '%s' applied object: %w", obj.GetName(), err)
		}
	}

	return nil
}
