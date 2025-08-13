// package testcase executes steps that form a single test case.
// a case references templates and step definitions from its directory to
// validate a particular behavior of the system under test.
//
// Example:
//
//	err := Run(ctx, kubeSvc, "./cases/example", logger)
//	if err != nil {
//	    // handle error
//	}
//
// each case loads templates and runs steps to validate system behavior.
package testcase
