// package test orchestrates complete test scenarios composed of multiple cases.
// before executing cases the service can ensure custom resources and a
// namespace based on the test configuration.
//
// Example:
//
//	err := Run(ctx, cfg, "./testdata", "", logger)
//	if err != nil {
//	    // handle error
//	}
//
// the service ensures crds and namespace before running the test cases.
package test
