package validate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/ipaqsa/kube2e/internal/validate"
)

// minimalCase is a case file that satisfies the schema.
const minimalCase = `version: v1
name: minimal
steps:
  - name: apply
    ensure:
      object: example
`

// ValidateSuite covers directory discovery and case validation.
type ValidateSuite struct {
	suite.Suite

	validator *validate.Validator
}

func TestValidateSuite(t *testing.T) {
	t.Parallel()

	suite.Run(t, new(ValidateSuite))
}

// SetupTest compiles a fresh validator for every test.
func (s *ValidateSuite) SetupTest() {
	validator, err := validate.New()
	s.Require().NoError(err)

	s.validator = validator
}

// TestExamplesAreValid checks the suites shipped in examples/.
func (s *ValidateSuite) TestExamplesAreValid() {
	report, err := s.validator.Dir("../../examples")
	s.Require().NoError(err)
	s.Require().NotEmpty(report.Suites)

	total, invalid := report.Totals()
	s.Positive(total)
	s.Zero(invalid)
	s.True(report.Valid())
}

// TestSingleSuiteDir accepts a suite directory directly, not only its parent.
func (s *ValidateSuite) TestSingleSuiteDir() {
	report, err := s.validator.Dir("../../examples/nginx")
	s.Require().NoError(err)
	s.Require().Len(report.Suites, 1)
	s.Equal("nginx", report.Suites[0].Name)
	s.True(report.Valid())
}

// TestInvalidCaseIsReported records the violation without failing the run.
func (s *ValidateSuite) TestInvalidCaseIsReported() {
	dir := s.writeSuite("broken", map[string]string{
		"good.yaml": minimalCase,
		"bad.yaml": `version: v1
name: bad
steps:
  - name: wait
    wait:
      target:
        object: app
      timeout: tomorrow
`,
	})

	report, err := s.validator.Dir(dir)
	s.Require().NoError(err)

	total, invalid := report.Totals()
	s.Equal(2, total)
	s.Equal(1, invalid)
	s.False(report.Valid())

	cases := report.Suites[0].Cases
	s.Require().Len(cases, 2)
	// Cases are reported in alphabetical filename order: bad.yaml, good.yaml.
	s.False(cases[0].Valid())
	s.True(cases[1].Valid())

	s.Require().Len(cases[0].Problems, 1)
	s.Equal("/steps/0/wait/timeout", cases[0].Problems[0].Pointer)
	s.Contains(cases[0].Problems[0].Message, "does not match pattern")
}

// TestInvalidObjectKeyIsReported names the offending key. The pointer stays at
// the document root because jsonschema/v6 loses the location of a propertyNames
// violation; the message carries the key instead.
func (s *ValidateSuite) TestInvalidObjectKeyIsReported() {
	dir := s.writeSuite("names", map[string]string{"case.yaml": `version: v1
name: bad-key
objects:
  Invalid_Name: deployment
steps:
  - name: apply
    ensure:
      object: example
`})

	report, err := s.validator.Dir(dir)
	s.Require().NoError(err)

	problems := report.Suites[0].Cases[0].Problems
	s.Require().NotEmpty(problems)
	s.Equal("/", problems[0].Pointer)
	s.Contains(problems[0].Message, "Invalid_Name")
}

// TestUnparsableCaseIsReported turns a YAML syntax error into a problem.
func (s *ValidateSuite) TestUnparsableCaseIsReported() {
	dir := s.writeSuite("broken", map[string]string{"case.yaml": "version: v1\n\tname: tab\n"})

	report, err := s.validator.Dir(dir)
	s.Require().NoError(err)

	problems := report.Suites[0].Cases[0].Problems
	s.Require().Len(problems, 1)
	s.Equal("/", problems[0].Pointer)
	s.Contains(problems[0].Message, "parse yaml")
}

// TestNonCaseFilesAreSkipped ignores everything that is not a .yaml file.
func (s *ValidateSuite) TestNonCaseFilesAreSkipped() {
	dir := s.writeSuite("mixed", map[string]string{
		"case.yaml":  minimalCase,
		"notes.md":   "not a case",
		"case.yml":   "version: nope",
		"case.yaml~": "version: nope",
	})

	report, err := s.validator.Dir(dir)
	s.Require().NoError(err)
	s.Require().Len(report.Suites[0].Cases, 1)
	s.True(report.Valid())
}

// TestHiddenSuitesAreSkipped keeps directories such as .git out of discovery.
func (s *ValidateSuite) TestHiddenSuitesAreSkipped() {
	root := s.T().TempDir()
	s.writeCases(filepath.Join(root, "visible"), map[string]string{"case.yaml": minimalCase})
	s.writeCases(filepath.Join(root, ".hidden"), map[string]string{"case.yaml": "version: nope"})

	report, err := s.validator.Dir(root)
	s.Require().NoError(err)
	s.Require().Len(report.Suites, 1)
	s.Equal("visible", report.Suites[0].Name)
}

// TestDirWithoutSuites reports ErrNoSuites.
func (s *ValidateSuite) TestDirWithoutSuites() {
	_, err := s.validator.Dir(s.T().TempDir())
	s.Require().ErrorIs(err, validate.ErrNoSuites)
}

// TestMissingDir reports the stat failure.
func (s *ValidateSuite) TestMissingDir() {
	_, err := s.validator.Dir(filepath.Join(s.T().TempDir(), "absent"))
	s.Require().ErrorIs(err, os.ErrNotExist)
}

// TestFileArgument rejects a path that is not a directory.
func (s *ValidateSuite) TestFileArgument() {
	path := filepath.Join(s.T().TempDir(), "case.yaml")
	s.Require().NoError(os.WriteFile(path, []byte(minimalCase), 0o600))

	_, err := s.validator.Dir(path)
	s.Require().Error(err)
	s.Require().NotErrorIs(err, validate.ErrNoSuites)
}

// TestFileMissing reports an unreadable case file as an error, not a problem.
func (s *ValidateSuite) TestFileMissing() {
	_, err := s.validator.File(filepath.Join(s.T().TempDir(), "absent.yaml"))
	s.Require().ErrorIs(err, os.ErrNotExist)
}

// writeSuite creates a suite directory named name with the given case files and
// returns its path.
func (s *ValidateSuite) writeSuite(name string, cases map[string]string) string {
	dir := filepath.Join(s.T().TempDir(), name)
	s.writeCases(dir, cases)

	return dir
}

// writeCases creates dir/cases and fills it with the given files.
func (s *ValidateSuite) writeCases(dir string, cases map[string]string) {
	s.T().Helper()

	casesDir := filepath.Join(dir, "cases")
	s.Require().NoError(os.MkdirAll(casesDir, 0o750))

	for name, content := range cases {
		s.Require().NoError(os.WriteFile(filepath.Join(casesDir, name), []byte(content), 0o600))
	}
}

// TestSchemaFixtures validates the checked-in schema fixtures through the
// public API, so the CLI and the schema tests agree on what is valid.
func TestSchemaFixtures(t *testing.T) {
	t.Parallel()

	validator, err := validate.New()
	require.NoError(t, err)

	for _, tt := range []struct {
		dir   string
		valid bool
	}{
		{dir: "../../schemas/testdata/valid", valid: true},
		{dir: "../../schemas/testdata/invalid", valid: false},
	} {
		paths, globErr := filepath.Glob(filepath.Join(tt.dir, "*.yaml"))
		require.NoError(t, globErr)
		require.NotEmpty(t, paths)

		for _, path := range paths {
			t.Run(filepath.Base(tt.dir)+"/"+filepath.Base(path), func(t *testing.T) {
				t.Parallel()

				report, fileErr := validator.File(path)
				require.NoError(t, fileErr)
				require.Equal(t, tt.valid, report.Valid(), "problems: %v", report.Problems)
			})
		}
	}
}
