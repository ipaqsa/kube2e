// Package action implements Kubernetes resource operations (Ensure, Delete, Wait, Patch, Assert).
package action

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svckube "github.com/ipaqsa/kube2e/internal/kube"
)

const (
	// defaultWaitInterval is the polling cadence for Wait.
	defaultWaitInterval = 2 * time.Second
	// defaultWaitTimeout is the hard deadline for Wait and delete-wait.
	defaultWaitTimeout = 2 * time.Minute
	// defaultExecTimeout is the deadline for the whole Exec action, retries included.
	defaultExecTimeout = 30 * time.Second
)

// Config is the shared configuration passed to all Run* functions.
type Config struct {
	Kube        *svckube.Service
	Object      client.Object
	Annotations map[string]string
	Logger      *slog.Logger
}

// RunEnsure creates or updates the object on the cluster using Server-Side Apply.
func RunEnsure(ctx context.Context, conf *Config, act *Ensure) (*Report, error) {
	report := newReport(NameEnsure, Target{Object: act.Object})
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "ensure", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("ensure object")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := runWithTimeout(ctx, act.TimeoutOrDefault(0), func(ctx context.Context) error {
		return conf.Kube.Ensure(ctx, conf.Object,
			svckube.WithEnsureAnnotations(conf.Annotations),
			svckube.WithEnsureRetry(act.Retry.attempts(), act.Retry.backoff()))
	})

	return finishReport(report, err)
}

// RunDelete removes the object from the cluster. When act.Wait is true it
// blocks until the object disappears. The timeout bounds the whole action; with
// wait it defaults to defaultWaitTimeout, without wait it applies only when set.
func RunDelete(ctx context.Context, conf *Config, act *Delete) (*Report, error) {
	report := newReport(NameDelete, act.Target)
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "delete", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("delete object")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	timeout := act.TimeoutOrDefault(0)
	if act.Wait {
		timeout = act.TimeoutOrDefault(defaultWaitTimeout)
	}

	err := runWithTimeout(ctx, timeout, func(ctx context.Context) error {
		return conf.Kube.Delete(ctx, conf.Object,
			svckube.WithDeleteWait(act.Wait, timeout),
			svckube.WithDeleteInterval(act.IntervalOrDefault(defaultWaitInterval)),
			svckube.WithDeleteRetry(act.Retry.attempts(), act.Retry.backoff()))
	})

	return finishReport(report, err)
}

// RunWait polls the object until all JQ conditions pass or the timeout expires.
func RunWait(ctx context.Context, conf *Config, act *Wait) (*Report, error) {
	report := newReport(NameWait, act.Target)
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "wait", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("wait for conditions")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := conf.Kube.Wait(ctx, conf.Object,
		svckube.WithWaitConditions(act.Conditions),
		svckube.WithWaitInterval(act.IntervalOrDefault(defaultWaitInterval)),
		svckube.WithWaitTimeout(act.TimeoutOrDefault(defaultWaitTimeout)))

	return finishReport(report, err)
}

// RunPatch applies RFC 6902 JSON patches to the live object on the cluster.
func RunPatch(ctx context.Context, conf *Config, act *Patch) (*Report, error) {
	report := newReport(NamePatch, act.Target)
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "patch", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	if len(act.Patches) == 0 {
		return finishReport(report, nil)
	}

	log.Info("patch object")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := runWithTimeout(ctx, act.TimeoutOrDefault(0), func(ctx context.Context) error {
		return conf.Kube.Patch(ctx, conf.Object, act.Patches,
			svckube.WithPatchRetry(act.Retry.attempts(), act.Retry.backoff()))
	})

	return finishReport(report, err)
}

// RunAssert fetches the object once and checks that all JQ conditions evaluate
// to true. When act.Retry is set the check is repeated up to Retry.Attempts
// times with Retry.Backoff between each attempt.
func RunAssert(ctx context.Context, conf *Config, act *Assert) (*Report, error) {
	report := newReport(NameAssert, act.Target)
	gvk := conf.Object.GetObjectKind().GroupVersionKind().String()
	log := conf.Logger.With("action", "assert", "name", conf.Object.GetName(), "gvk", gvk)

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("assert conditions")

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := runWithTimeout(ctx, act.TimeoutOrDefault(0), func(ctx context.Context) error {
		return conf.Kube.Check(ctx, conf.Object, act.Conditions,
			svckube.WithCheckRetry(act.Retry.attempts(), act.Retry.backoff()))
	})

	return finishReport(report, err)
}

// RunLogs checks the logs of the named Pod against act.Contains using the
// configured match policy. The timeout is owned by the poll, not wrapped here:
// for match none it is the observation window, whose end is the success path.
func RunLogs(ctx context.Context, conf *Config, act *Logs) (*Report, error) {
	report := newReport(NameLogs, act.Target)
	log := conf.Logger.With("action", "logs", "target", targetName(conf, act.Target))

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("wait for log output", "contains", act.Contains)

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := conf.Kube.LogsContains(ctx, podTarget(conf, act.Target), act.Contains,
		svckube.WithLogsContainer(act.Container),
		svckube.WithLogsMatch(svckube.LogsMatch(act.Match)),
		svckube.WithLogsInterval(act.IntervalOrDefault(defaultWaitInterval)),
		svckube.WithLogsTimeout(act.TimeoutOrDefault(defaultWaitTimeout)))

	return finishReport(report, err)
}

// RunExec runs act.Command inside the resolved pod and succeeds when the
// command exits with code zero.
func RunExec(ctx context.Context, conf *Config, act *Exec) (*Report, error) {
	report := newReport(NameExec, act.Target)
	log := conf.Logger.With("action", "exec", "target", targetName(conf, act.Target))

	if err := applyDelay(ctx, log, act.Delay); err != nil {
		return finishReport(report, err)
	}

	log.Info("exec command", "command", act.Command)

	start := time.Now()
	defer func() { log.Debug("action finished", "duration", time.Since(start)) }()

	err := runWithTimeout(ctx, act.TimeoutOrDefault(defaultExecTimeout), func(ctx context.Context) error {
		return conf.Kube.Exec(ctx, podTarget(conf, act.Target), act.Command,
			svckube.WithExecContainer(act.Container),
			svckube.WithExecRetry(act.Retry.attempts(), act.Retry.backoff()))
	})

	return finishReport(report, err)
}

// podTarget builds a kube.PodTarget from the action target: a rendered object
// when the target names one, otherwise the kind + label selector.
func podTarget(conf *Config, t Target) svckube.PodTarget {
	if t.Object != "" {
		return svckube.PodTarget{Object: conf.Object}
	}

	return svckube.PodTarget{Kind: t.Kind, LabelSelector: t.LabelSelector}
}

// targetName is a short identifier for logs that works for both object and
// selector targets (conf.Object is nil for selector targets).
func targetName(conf *Config, t Target) string {
	if t.Object != "" && conf.Object != nil {
		return conf.Object.GetName()
	}

	return fmt.Sprintf("%s[%s]", t.Kind, t.LabelSelector)
}

// applyDelay waits for the configured duration if set, returning early with
// the context error if the context is canceled before the delay elapses.
func applyDelay(ctx context.Context, log *slog.Logger, delay *metav1.Duration) error {
	if delay == nil || delay.Duration <= 0 {
		return nil
	}

	log.Info("action delay", "duration", delay.Duration)

	timer := time.NewTimer(delay.Duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// runWithTimeout runs fn under ctx bounded by timeout, so every attempt, nested
// retry, and backoff of the action share one deadline. A zero timeout leaves ctx
// unbounded. When the action's own deadline (not the parent's) stopped it, the
// error says so and still satisfies errors.Is(context.DeadlineExceeded).
func runWithTimeout(ctx context.Context, timeout time.Duration, fn func(context.Context) error) error {
	if timeout <= 0 {
		return fn(ctx)
	}

	actionCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := fn(actionCtx)
	if err != nil && ctx.Err() == nil && errors.Is(actionCtx.Err(), context.DeadlineExceeded) {
		if !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("action timeout %s exceeded: %w: %w", timeout, context.DeadlineExceeded, err)
		}

		return fmt.Errorf("action timeout %s exceeded: %w", timeout, err)
	}

	return err
}
