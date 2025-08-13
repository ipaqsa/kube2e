// package action defines an operation executed during a test step.
// each action renders a kubernetes object from a template and then
// applies a command such as ensure, delete, wait or patch.
//
// Example:
//
//	act := Action{Command: "Ensure", Object: Object{Template: "pod"}}
//	_ = Run(ctx, kubeSvc, tmplMgr, act, logger)
//
// actions can optionally supply conditions, timeouts and json patches to
// fine tune how the command interacts with the cluster.
package action
