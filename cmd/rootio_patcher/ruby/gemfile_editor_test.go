package ruby

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const src = "https://pkg.root.io/ruby"

func patch(t *testing.T, in, name, ver string) string {
	t.Helper()
	e := NewGemfileEditor(in)
	require.NoError(t, e.PatchDependency(name, ver))
	require.NoError(t, e.SetGemSource(name, src))
	return e.String()
}

func TestPatchDependency_Constraints(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"no constraint", "gem 'rack'\n", "gem 'rack', '>= 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\n"},
		{"loose", "gem 'rack', '>= 2.0'\n", "gem 'rack', '>= 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\n"},
		{"pessimistic", "gem 'rack', '~> 2.2.0'\n", "gem 'rack', '~> 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\n"},
		{"exact pin", "gem 'rack', '= 2.2.3'\n", "gem 'rack', '= 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\n"},
		{"bare version", "gem 'rack', '2.2.3'\n", "gem 'rack', '2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\n"},
		{"upper kept", "gem 'rack', '>= 2.0', '< 3'\n", "gem 'rack', '>= 2.2.4.41610.1', '< 3', source: 'https://pkg.root.io/ruby'\n"},
		{"upper bumped", "gem 'rack', '>= 1.0', '< 2'\n", "gem 'rack', '>= 2.2.4.41610.1', '< 3.0', source: 'https://pkg.root.io/ruby'\n"},
		{"combined string", "gem 'rack', '>= 1.0, < 2'\n", "gem 'rack', '>= 2.2.4.41610.1', '< 3.0', source: 'https://pkg.root.io/ruby'\n"},
		{"options kept", "gem 'rack', '~> 2.0', require: false\n", "gem 'rack', '~> 2.2.4.41610.1', require: false, source: 'https://pkg.root.io/ruby'\n"},
		{"no version with options", "gem \"rack\", require: false # keep\n", "gem \"rack\", '>= 2.2.4.41610.1', require: false, source: \"https://pkg.root.io/ruby\" # keep\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := patch(t, tc.in, "rack", "2.2.4.41610.1")
			if tc.name == "no version with options" {
				// quote style follows the file (double); version quote follows too
				tc.want = "gem \"rack\", \">= 2.2.4.41610.1\", require: false, source: \"https://pkg.root.io/ruby\" # keep\n"
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSetGemSource_ReplacesExistingAndStripsGit(t *testing.T) {
	got := patch(t, "gem 'rack', git: 'https://github.com/rack/rack', branch: 'main'\n", "rack", "2.2.4.41610.1")
	assert.Equal(t, "gem 'rack', '>= 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\n", got)

	got = patch(t, "gem 'rack', '~> 2.0', source: 'https://old.example/x'\n", "rack", "2.2.4.41610.1")
	assert.Equal(t, "gem 'rack', '~> 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\n", got)

	got = patch(t, "gem 'rack', :source => 'https://old.example/x'\n", "rack", "2.2.4.41610.1")
	assert.Contains(t, got, "'https://pkg.root.io/ruby'")
	assert.NotContains(t, got, "old.example")
}

func TestPatch_AddsTransitiveGem(t *testing.T) {
	got := patch(t, "source 'https://rubygems.org'\n\ngem 'rails'\n", "nokogiri", "1.16.5.41610.1")
	assert.Equal(t, "source 'https://rubygems.org'\n\ngem 'rails'\n\ngem 'nokogiri', '>= 1.16.5.41610.1', source: 'https://pkg.root.io/ruby'\n", got)
}

func TestPatch_MultiLineStatement(t *testing.T) {
	in := "gem 'rack',\n  '~> 2.0',\n  require: false\n"
	got := patch(t, in, "rack", "2.2.4.41610.1")
	assert.Equal(t, "gem 'rack',\n  '~> 2.2.4.41610.1',\n  require: false, source: 'https://pkg.root.io/ruby'\n", got)
}

func TestSetGemSource_HoistsOutOfSourceBlock(t *testing.T) {
	in := `source 'https://rubygems.org' do
  gem 'rails'
  group :test do
    gem 'rack', '~> 2.0'
    gem 'rspec'
  end
end

gem 'puma'
`
	want := `source 'https://rubygems.org' do
  gem 'rails'
  group :test do
    gem 'rspec'
  end
end

group :test do
  gem 'rack', '~> 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'
end

gem 'puma'
`
	assert.Equal(t, want, patch(t, in, "rack", "2.2.4.41610.1"))
}

func TestPatch_MultipleDeclarations(t *testing.T) {
	in := "platforms :ruby do\n  gem 'rack', '~> 2.0'\nend\ngem 'rack', '>= 2.0'\n"
	got := patch(t, in, "rack", "2.2.4.41610.1")
	assert.Equal(t, "platforms :ruby do\n  gem 'rack', '~> 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\nend\ngem 'rack', '>= 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\n", got)
}

func TestPatch_PreservesCRLF(t *testing.T) {
	got := patch(t, "gem 'rack'\r\n", "rack", "2.2.4.41610.1")
	assert.Equal(t, "gem 'rack', '>= 2.2.4.41610.1', source: 'https://pkg.root.io/ruby'\r\n", got)
}

func TestPatch_UnsupportedDeclarationErrors(t *testing.T) {
	e := NewGemfileEditor("gem('rack', '~> 2.0')\n")
	assert.Error(t, e.PatchDependency("rack", "2.2.4.41610.1"))
}

func TestPinDependency_ReplacesEveryConstraintWithAnExactPin(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"no declaration", "gem 'rails'\n", "gem 'rails'\n\ngem 'rack', '= 2.2.4'\n"},
		{"no constraint", "gem 'rack'\n", "gem 'rack', '= 2.2.4'\n"},
		{"loose", "gem 'rack', '>= 2.0'\n", "gem 'rack', '= 2.2.4'\n"},
		{"pessimistic with upper", "gem 'rack', '~> 2.0', '< 3'\n", "gem 'rack', '= 2.2.4'\n"},
		{"options kept", "gem 'rack', '~> 2.0', require: false\n", "gem 'rack', '= 2.2.4', require: false\n"},
		{"git ref is not touched", "gem 'rack', git: 'https://github.com/rack/rack'\n", "gem 'rack', '= 2.2.4', git: 'https://github.com/rack/rack'\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := NewGemfileEditor(tc.in)
			require.NoError(t, e.PinDependency("rack", "2.2.4"))
			assert.Equal(t, tc.want, e.String())
		})
	}
}
