package validate

// rootPointer addresses the case document as a whole. RFC 6901 spells the root
// as the empty string; kube2e prints "/" so every problem carries a location.
const rootPointer = "/"

// Report holds the validation results for every test suite under a directory.
type Report struct {
	// Dir is the directory the suites were discovered in.
	Dir string `json:"dir"`
	// Suites holds one entry per discovered suite, in discovery order.
	Suites []SuiteReport `json:"suites"`
}

// SuiteReport holds the validation results for one test suite directory.
type SuiteReport struct {
	// Name is the suite name, derived from the directory base name.
	Name string `json:"name"`
	// Path is the filesystem path of the suite directory.
	Path string `json:"path"`
	// Cases holds one entry per case file, in alphabetical filename order.
	Cases []CaseReport `json:"cases"`
}

// CaseReport holds the validation result for a single case file.
type CaseReport struct {
	// Path is the filesystem path of the case file.
	Path string `json:"path"`
	// Problems lists everything that makes the case invalid. It is empty for a
	// valid case and holds a single entry when the file is not parsable at all.
	Problems []Problem `json:"problems,omitempty"`
}

// Problem is one reason a case file is invalid: a schema violation, or the
// parse failure that prevented the file from being checked against the schema.
type Problem struct {
	// Pointer is the RFC 6901 JSON pointer of the offending value.
	Pointer string `json:"pointer"`
	// Message describes the violation.
	Message string `json:"message"`
}

// Valid reports whether every checked case file matched the schema.
func (r *Report) Valid() bool {
	_, invalid := r.Totals()

	return invalid == 0
}

// Totals returns how many case files were checked and how many are invalid.
func (r *Report) Totals() (int, int) {
	var total, invalid int

	for _, suite := range r.Suites {
		for _, caseReport := range suite.Cases {
			total++

			if !caseReport.Valid() {
				invalid++
			}
		}
	}

	return total, invalid
}

// Valid reports whether the case file matched the schema.
func (c CaseReport) Valid() bool {
	return len(c.Problems) == 0
}
