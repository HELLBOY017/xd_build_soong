package bp2build

import (
	"android/soong/android"
	"reflect"
	"sort"
	"strings"

	"github.com/google/blueprint/proptools"
)

type BazelFile struct {
	Dir      string
	Basename string
	Contents string
}

func CreateBazelFiles(
	ruleShims map[string]RuleShim,
	buildToTargets map[string][]BazelTarget,
	bp2buildEnabled bool) []BazelFile {
	files := make([]BazelFile, 0, len(ruleShims)+len(buildToTargets)+numAdditionalFiles)

	// Write top level files: WORKSPACE and BUILD. These files are empty.
	files = append(files, newFile("", "WORKSPACE", ""))
	// Used to denote that the top level directory is a package.
	files = append(files, newFile("", "BUILD", ""))

	files = append(files, newFile(bazelRulesSubDir, "BUILD", ""))

	if !bp2buildEnabled {
		// These files are only used for queryview.
		files = append(files, newFile(bazelRulesSubDir, "providers.bzl", providersBzl))

		for bzlFileName, ruleShim := range ruleShims {
			files = append(files, newFile(bazelRulesSubDir, bzlFileName+".bzl", ruleShim.content))
		}
		files = append(files, newFile(bazelRulesSubDir, "soong_module.bzl", generateSoongModuleBzl(ruleShims)))
	}

	files = append(files, createBuildFiles(buildToTargets, bp2buildEnabled)...)

	return files
}

func createBuildFiles(buildToTargets map[string][]BazelTarget, bp2buildEnabled bool) []BazelFile {
	files := make([]BazelFile, 0, len(buildToTargets))
	for _, dir := range android.SortedStringKeys(buildToTargets) {
		content := soongModuleLoad
		if bp2buildEnabled {
			// No need to load soong_module for bp2build BUILD files.
			content = ""
		}
		targets := buildToTargets[dir]
		sort.Slice(targets, func(i, j int) bool { return targets[i].name < targets[j].name })
		for _, t := range targets {
			content += "\n\n"
			content += t.content
		}
		files = append(files, newFile(dir, "BUILD.bazel", content))
	}
	return files
}

func newFile(dir, basename, content string) BazelFile {
	return BazelFile{
		Dir:      dir,
		Basename: basename,
		Contents: content,
	}
}

const (
	bazelRulesSubDir = "build/bazel/queryview_rules"

	// additional files:
	//  * workspace file
	//  * base BUILD file
	//  * rules BUILD file
	//  * rules providers.bzl file
	//  * rules soong_module.bzl file
	numAdditionalFiles = 5
)

var (
	// Certain module property names are blocklisted/ignored here, for the reasons commented.
	ignoredPropNames = map[string]bool{
		"name":       true, // redundant, since this is explicitly generated for every target
		"from":       true, // reserved keyword
		"in":         true, // reserved keyword
		"arch":       true, // interface prop type is not supported yet.
		"multilib":   true, // interface prop type is not supported yet.
		"target":     true, // interface prop type is not supported yet.
		"visibility": true, // Bazel has native visibility semantics. Handle later.
		"features":   true, // There is already a built-in attribute 'features' which cannot be overridden.
	}
)

func shouldGenerateAttribute(prop string) bool {
	return !ignoredPropNames[prop]
}

func shouldSkipStructField(field reflect.StructField) bool {
	if field.PkgPath != "" {
		// Skip unexported fields. Some properties are
		// internal to Soong only, and these fields do not have PkgPath.
		return true
	}
	// fields with tag `blueprint:"mutated"` are exported to enable modification in mutators, etc
	// but cannot be set in a .bp file
	if proptools.HasTag(field, "blueprint", "mutated") {
		return true
	}
	return false
}

// FIXME(b/168089390): In Bazel, rules ending with "_test" needs to be marked as
// testonly = True, forcing other rules that depend on _test rules to also be
// marked as testonly = True. This semantic constraint is not present in Soong.
// To work around, rename "*_test" rules to "*_test_".
func canonicalizeModuleType(moduleName string) string {
	if strings.HasSuffix(moduleName, "_test") {
		return moduleName + "_"
	}

	return moduleName
}