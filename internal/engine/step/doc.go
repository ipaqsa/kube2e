// package step runs a list of actions described in a yaml step file.
// each step file contains metadata and a sequence of actions that are
// executed against the cluster in order.
//
// Example:
//
//	err := Run(ctx, kubeSvc, tmplMgr, "01_step.yaml", logger)
//	if err != nil {
//	    // handle error
//	}
//
// a step groups multiple actions that are executed sequentially.
package step
