package allservices_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
	_ "github.com/hashicorp/terraform-provider-google/google/allservices"
	"github.com/hashicorp/terraform-provider-google/google/fwprovider"
	"github.com/hashicorp/terraform-provider-google/google/provider"
	"github.com/hashicorp/terraform-provider-google/google/registry"
)

type astRefreshViolation struct {
	FilePath string
	FuncName string
	acctest.RefreshStateViolation
}

func TestValidateRefreshStateSteps(t *testing.T) {
	ctx := context.Background()
	schemas := acctest.BuildRefreshStateSchemaMap(
		registry.ResourceMap(),
		fwprovider.New(provider.Provider()).Resources(ctx),
	)

	pkgRoot, err := findProviderPackageRoot()
	if err != nil {
		t.Fatalf("Failed to find provider package root: %v", err)
	}

	var dirsToScan []string
	servicesDir := filepath.Join(pkgRoot, "services")
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		t.Fatalf("Failed reading %s: %v", servicesDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			dirsToScan = append(dirsToScan, filepath.Join(servicesDir, entry.Name()))
		}
	}
	for _, extra := range []string{"provider", "provider/universe", "fwprovider", "functions"} {
		extraDir := filepath.Join(pkgRoot, extra)
		if info, err := os.Stat(extraDir); err == nil && info.IsDir() {
			dirsToScan = append(dirsToScan, extraDir)
		}
	}

	var allViolations []astRefreshViolation
	for _, dir := range dirsToScan {
		vs, err := checkRefreshStateInDir(dir, schemas)
		if err != nil {
			t.Fatalf("Error scanning %s: %v", dir, err)
		}
		allViolations = append(allViolations, vs...)
	}

	for _, v := range allViolations {
		relPath, err := filepath.Rel(pkgRoot, v.FilePath)
		if err != nil {
			relPath = v.FilePath
		}
		switch v.Rule {
		case acctest.RefreshStateRuleStepProviders:
			t.Errorf("%s:%d: %s: `{RefreshState: true}` step (line %d) omits step-level provider configuration (`ProtoV5ProviderFactories`, `ExternalProviders`, etc.) and `ExpectNonEmptyPlan: true` in a `TestCase` without top-level providers [%s]",
				relPath, v.ImportStepLine, v.FuncName, v.ImportStepLine, v.Rule)
			continue
		case acctest.RefreshStateRuleStepProvidersExpectNonEmptyPlan:
			t.Errorf("%s:%d: %s: `{RefreshState: true}` step (line %d) specifies step-level providers without `ExpectNonEmptyPlan: true` (terraform-plugin-testing overwrites .tf config with a provider-only block when step-level providers are set, making the post-refresh plan non-empty) [%s]",
				relPath, v.ImportStepLine, v.FuncName, v.ImportStepLine, v.Rule)
			continue
		}
		detail := v.Rule
		if len(v.UnignoredSetPaths) > 0 {
			detail = fmt.Sprintf("%s: %s", v.Rule, strings.Join(v.UnignoredSetPaths, ", "))
		}
		t.Errorf("%s:%d: %s: missing `{RefreshState: true},` after Config step (line %d) before ImportStateVerify step (line %d) for %q [%s]",
			relPath, v.ImportStepLine, v.FuncName, v.ConfigStepLine, v.ImportStepLine, v.ResourceName, detail)
	}
}

func findProviderPackageRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	parent := filepath.Clean(filepath.Join(wd, ".."))
	if info, err := os.Stat(filepath.Join(parent, "services")); err == nil && info.IsDir() {
		return parent, nil
	}

	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				return "", readErr
			}
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), "google") {
					cand := filepath.Join(dir, e.Name())
					if info, statErr := os.Stat(filepath.Join(cand, "services")); statErr == nil && info.IsDir() {
						return cand, nil
					}
				}
			}
			break
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return "", fmt.Errorf("could not locate provider package root containing services/ from %s", wd)
}

func checkRefreshStateInDir(dir string, schemas map[string]acctest.RefreshStateSchemaInfo) ([]astRefreshViolation, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var parsedFiles []*ast.File
	var filePaths []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		af, err := parser.ParseFile(fset, fullPath, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", fullPath, err)
		}
		parsedFiles = append(parsedFiles, af)
		filePaths = append(filePaths, fullPath)
	}

	return checkRefreshStateInParsedFiles(fset, parsedFiles, filePaths, schemas), nil
}

func checkRefreshStateInParsedFiles(
	fset *token.FileSet,
	parsedFiles []*ast.File,
	filePaths []string,
	schemas map[string]acctest.RefreshStateSchemaInfo,
) []astRefreshViolation {
	stringConsts := make(map[string]string)
	sliceConsts := make(map[string][]string)
	funcDecls := make(map[string]*ast.FuncDecl)

	for _, af := range parsedFiles {
		for _, decl := range af.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				funcDecls[d.Name.Name] = d
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, nameIdent := range vs.Names {
						if i >= len(vs.Values) {
							break
						}
						if s, ok := evalASTStringExpr(vs.Values[i], stringConsts); ok {
							stringConsts[nameIdent.Name] = s
						} else if sl, ok := evalASTStringSliceExpr(vs.Values[i], stringConsts, sliceConsts); ok {
							sliceConsts[nameIdent.Name] = sl
						}
					}
				}
			}
		}
	}

	var violations []astRefreshViolation

	for idx, af := range parsedFiles {
		fullPath := filePaths[idx]
		for _, decl := range af.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}

			localStrings := make(map[string]string, len(stringConsts))
			for k, v := range stringConsts {
				localStrings[k] = v
			}
			localSlices := make(map[string][]string, len(sliceConsts))
			for k, v := range sliceConsts {
				localSlices[k] = v
			}
			localTestCases := make(map[string]*ast.CompositeLit)
			localStepSlices := make(map[string][]ast.Expr)

			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch st := n.(type) {
				case *ast.AssignStmt:
					for i, lhs := range st.Lhs {
						if i >= len(st.Rhs) {
							break
						}
						id, ok := lhs.(*ast.Ident)
						if !ok {
							continue
						}
						rhs := st.Rhs[i]
						if s, ok := evalASTStringExpr(rhs, localStrings); ok {
							localStrings[id.Name] = s
						} else if sl, ok := evalASTStringSliceExpr(rhs, localStrings, localSlices); ok {
							localSlices[id.Name] = sl
						} else if cl, ok := rhs.(*ast.CompositeLit); ok {
							if isTestCaseCompositeLit(cl) {
								localTestCases[id.Name] = cl
							} else if isTestStepSliceLit(cl) {
								localStepSlices[id.Name] = append([]ast.Expr(nil), cl.Elts...)
							}
						} else if elems, ok := evalStepSliceAppend(rhs, localStepSlices); ok {
							localStepSlices[id.Name] = elems
						}
					}
				case *ast.ValueSpec:
					for i, nameIdent := range st.Names {
						if i >= len(st.Values) {
							break
						}
						val := st.Values[i]
						if s, ok := evalASTStringExpr(val, localStrings); ok {
							localStrings[nameIdent.Name] = s
						} else if sl, ok := evalASTStringSliceExpr(val, localStrings, localSlices); ok {
							localSlices[nameIdent.Name] = sl
						} else if cl, ok := val.(*ast.CompositeLit); ok {
							if isTestCaseCompositeLit(cl) {
								localTestCases[nameIdent.Name] = cl
							} else if isTestStepSliceLit(cl) {
								localStepSlices[nameIdent.Name] = append([]ast.Expr(nil), cl.Elts...)
							}
						}
					}
				}
				return true
			})

			ast.Inspect(fd.Body, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok || !isASTVcrTestCall(ce) {
					return true
				}
				tcLit := findASTTestCaseLit(ce, localTestCases)
				if tcLit == nil {
					return true
				}
				stepExprs, ok := findASTStepExprs(tcLit, localStepSlices)
				if !ok {
					return true
				}

				tcHasProviders := hasASTProviderFields(tcLit)
				var stepInputs []acctest.RefreshStateStepInput
				for sIdx, elt := range stepExprs {
					inp, resolved := parseASTStepExpr(sIdx, elt, fset, funcDecls, localStrings, localSlices)
					if !resolved {
						inp.SkipImportVerifyCheck = true
					}
					stepInputs = append(stepInputs, inp)
				}

				for _, v := range acctest.CheckRefreshStateProviderSteps(tcHasProviders, stepInputs) {
					violations = append(violations, astRefreshViolation{
						FilePath:              fullPath,
						FuncName:              fd.Name.Name,
						RefreshStateViolation: v,
					})
				}

				for _, v := range acctest.CheckRefreshStateSteps(stepInputs, schemas) {
					violations = append(violations, astRefreshViolation{
						FilePath:              fullPath,
						FuncName:              fd.Name.Name,
						RefreshStateViolation: v,
					})
				}
				return true
			})
		}
	}

	return violations
}

func isASTVcrTestCall(ce *ast.CallExpr) bool {
	switch fn := ce.Fun.(type) {
	case *ast.Ident:
		return fn.Name == "VcrTest"
	case *ast.SelectorExpr:
		return fn.Sel.Name == "VcrTest"
	}
	return false
}

func isTestCaseCompositeLit(cl *ast.CompositeLit) bool {
	if sel, ok := cl.Type.(*ast.SelectorExpr); ok {
		return sel.Sel.Name == "TestCase"
	}
	if id, ok := cl.Type.(*ast.Ident); ok {
		return id.Name == "TestCase"
	}
	return false
}

func isTestStepSliceLit(cl *ast.CompositeLit) bool {
	at, ok := cl.Type.(*ast.ArrayType)
	if !ok {
		return false
	}
	if sel, ok := at.Elt.(*ast.SelectorExpr); ok {
		return sel.Sel.Name == "TestStep"
	}
	if id, ok := at.Elt.(*ast.Ident); ok {
		return id.Name == "TestStep"
	}
	return false
}

func evalStepSliceAppend(expr ast.Expr, stepSlices map[string][]ast.Expr) ([]ast.Expr, bool) {
	ce, ok := expr.(*ast.CallExpr)
	if !ok || len(ce.Args) < 1 || ce.Ellipsis.IsValid() {
		return nil, false
	}
	id, ok := ce.Fun.(*ast.Ident)
	if !ok || id.Name != "append" {
		return nil, false
	}
	baseID, ok := ce.Args[0].(*ast.Ident)
	if !ok {
		return nil, false
	}
	base, ok := stepSlices[baseID.Name]
	if !ok {
		return nil, false
	}
	combined := append([]ast.Expr(nil), base...)
	combined = append(combined, ce.Args[1:]...)
	return combined, true
}

func findASTTestCaseLit(ce *ast.CallExpr, localTestCases map[string]*ast.CompositeLit) *ast.CompositeLit {
	for _, arg := range ce.Args {
		switch a := arg.(type) {
		case *ast.CompositeLit:
			if isTestCaseCompositeLit(a) {
				return a
			}
		case *ast.Ident:
			if tc, ok := localTestCases[a.Name]; ok {
				return tc
			}
		}
	}
	return nil
}

func hasASTProviderFields(cl *ast.CompositeLit) bool {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch keyIdent.Name {
		case "Providers", "ProviderFactories", "ProtoV5ProviderFactories", "ProtoV6ProviderFactories", "ExternalProviders":
			if id, ok := kv.Value.(*ast.Ident); ok && id.Name == "nil" {
				continue
			}
			return true
		}
	}
	return false
}

func findASTStepExprs(tc *ast.CompositeLit, localStepSlices map[string][]ast.Expr) ([]ast.Expr, bool) {
	for _, elt := range tc.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		id, ok := kv.Key.(*ast.Ident)
		if !ok || id.Name != "Steps" {
			continue
		}
		switch v := kv.Value.(type) {
		case *ast.CompositeLit:
			return v.Elts, true
		case *ast.Ident:
			if elems, ok := localStepSlices[v.Name]; ok {
				return elems, true
			}
		}
	}
	return nil, false
}

func parseASTStepExpr(
	idx int,
	expr ast.Expr,
	fset *token.FileSet,
	funcDecls map[string]*ast.FuncDecl,
	localStrings map[string]string,
	localSlices map[string][]string,
) (acctest.RefreshStateStepInput, bool) {
	if cl, ok := expr.(*ast.CompositeLit); ok {
		return parseASTStepCompositeLit(idx, cl, fset, localStrings, localSlices), true
	}
	if ce, ok := expr.(*ast.CallExpr); ok {
		if id, ok := ce.Fun.(*ast.Ident); ok {
			if helperFD, exists := funcDecls[id.Name]; exists {
				if inp, ok := resolveASTStepHelperCall(idx, ce, helperFD, fset, localStrings, localSlices); ok {
					inp.Line = fset.Position(expr.Pos()).Line
					return inp, true
				}
			}
		}
	}
	return acctest.RefreshStateStepInput{
		Index: idx,
		Line:  fset.Position(expr.Pos()).Line,
	}, false
}

func resolveASTStepHelperCall(
	idx int,
	call *ast.CallExpr,
	helperFD *ast.FuncDecl,
	fset *token.FileSet,
	callerStrings map[string]string,
	callerSlices map[string][]string,
) (acctest.RefreshStateStepInput, bool) {
	if helperFD.Body == nil {
		return acctest.RefreshStateStepInput{}, false
	}
	helperStrings := make(map[string]string, len(callerStrings))
	for k, v := range callerStrings {
		helperStrings[k] = v
	}
	helperSlices := make(map[string][]string, len(callerSlices))
	for k, v := range callerSlices {
		helperSlices[k] = v
	}

	if helperFD.Type.Params != nil {
		argPos := 0
		for _, field := range helperFD.Type.Params.List {
			for _, nameIdent := range field.Names {
				if argPos < len(call.Args) {
					argExpr := call.Args[argPos]
					if s, ok := evalASTStringExpr(argExpr, callerStrings); ok {
						helperStrings[nameIdent.Name] = s
					} else if sPrefix := extractASTSprintfPrefix(argExpr, callerStrings); sPrefix != "" {
						helperStrings[nameIdent.Name] = sPrefix
					} else if sl, ok := evalASTStringSliceExpr(argExpr, callerStrings, callerSlices); ok {
						helperSlices[nameIdent.Name] = sl
					}
				}
				argPos++
			}
		}
	}

	var retLit *ast.CompositeLit
	for _, stmt := range helperFD.Body.List {
		switch st := stmt.(type) {
		case *ast.AssignStmt:
			if len(st.Lhs) == 1 && len(st.Rhs) == 1 {
				if id, ok := st.Lhs[0].(*ast.Ident); ok {
					if s, ok := evalASTStringExpr(st.Rhs[0], helperStrings); ok {
						helperStrings[id.Name] = s
					} else if sl, ok := evalASTStringSliceExpr(st.Rhs[0], helperStrings, helperSlices); ok {
						helperSlices[id.Name] = sl
					}
				}
			}
		case *ast.ReturnStmt:
			if len(st.Results) == 1 {
				if cl, ok := st.Results[0].(*ast.CompositeLit); ok {
					retLit = cl
				}
			}
		}
	}
	if retLit == nil {
		return acctest.RefreshStateStepInput{}, false
	}
	return parseASTStepCompositeLit(idx, retLit, fset, helperStrings, helperSlices), true
}

func parseASTStepCompositeLit(
	idx int,
	cl *ast.CompositeLit,
	fset *token.FileSet,
	localStrings map[string]string,
	localSlices map[string][]string,
) acctest.RefreshStateStepInput {
	inp := acctest.RefreshStateStepInput{
		Index: idx,
		Line:  fset.Position(cl.Pos()).Line,
	}
	hasResourceNameExpr := false
	resolvedResourceName := false
	dynamicIgnore := false

	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch keyIdent.Name {
		case "Config":
			inp.HasConfig = true
		case "ConfigDirectory", "ConfigFile":
			inp.HasConfig = true
			if id, ok := kv.Value.(*ast.Ident); !ok || id.Name != "nil" {
				inp.HasConfigDirOrFile = true
			}
		case "Providers", "ProviderFactories", "ProtoV5ProviderFactories", "ProtoV6ProviderFactories", "ExternalProviders":
			if id, ok := kv.Value.(*ast.Ident); !ok || id.Name != "nil" {
				inp.HasStepProviders = true
			}
		case "Destroy":
			inp.Destroy = evalASTBoolTrue(kv.Value)
		case "PlanOnly":
			inp.PlanOnly = evalASTBoolTrue(kv.Value)
		case "ExpectError":
			if id, ok := kv.Value.(*ast.Ident); !ok || id.Name != "nil" {
				inp.HasExpectError = true
			}
		case "Query":
			inp.Query = evalASTBoolTrue(kv.Value)
		case "RefreshState":
			inp.RefreshState = evalASTBoolTrue(kv.Value)
		case "ExpectNonEmptyPlan":
			inp.ExpectNonEmptyPlan = evalASTBoolTrue(kv.Value)
		case "ImportState":
			inp.ImportState = evalASTBoolTrue(kv.Value)
		case "ImportStateVerify":
			inp.ImportStateVerify = evalASTBoolTrue(kv.Value)
		case "ResourceName":
			hasResourceNameExpr = true
			if s, ok := evalASTStringExpr(kv.Value, localStrings); ok {
				inp.ResourceName = s
				resolvedResourceName = true
			} else if sPrefix := extractASTSprintfPrefix(kv.Value, localStrings); sPrefix != "" {
				inp.ResourceName = sPrefix
				resolvedResourceName = true
			}
		case "ImportStateVerifyIgnore":
			if sl, ok := evalASTStringSliceExpr(kv.Value, localStrings, localSlices); ok {
				inp.ImportStateVerifyIgnore = sl
			} else {
				dynamicIgnore = true
			}
		}
	}

	if (hasResourceNameExpr && !resolvedResourceName) || dynamicIgnore {
		inp.SkipImportVerifyCheck = true
	}
	return inp
}

func evalASTBoolTrue(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "true"
}

func evalASTStringExpr(e ast.Expr, vars map[string]string) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			if s, err := strconv.Unquote(v.Value); err == nil {
				return s, true
			}
		}
	case *ast.Ident:
		if s, ok := vars[v.Name]; ok {
			return s, true
		}
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			l, ok1 := evalASTStringExpr(v.X, vars)
			r, ok2 := evalASTStringExpr(v.Y, vars)
			if ok1 && ok2 {
				return l + r, true
			}
		}
	}
	return "", false
}

func extractASTSprintfPrefix(e ast.Expr, vars map[string]string) string {
	ce, ok := e.(*ast.CallExpr)
	if !ok || len(ce.Args) == 0 {
		return ""
	}
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Sprintf" && sel.Sel.Name != "Nprintf") {
		return ""
	}
	if fmtStr, ok := evalASTStringExpr(ce.Args[0], vars); ok {
		if dot := strings.IndexByte(fmtStr, '.'); dot > 0 && !strings.Contains(fmtStr[:dot], "%") {
			return fmtStr
		}
	}
	return ""
}

func evalASTStringSliceExpr(e ast.Expr, strVars map[string]string, sliceVars map[string][]string) ([]string, bool) {
	switch v := e.(type) {
	case *ast.CompositeLit:
		var out []string
		for _, elt := range v.Elts {
			s, ok := evalASTStringExpr(elt, strVars)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	case *ast.Ident:
		if v.Name == "nil" {
			return nil, true
		}
		if sl, ok := sliceVars[v.Name]; ok {
			return sl, true
		}
	case *ast.CallExpr:
		if id, ok := v.Fun.(*ast.Ident); ok && id.Name == "append" && len(v.Args) >= 1 {
			base, ok := evalASTStringSliceExpr(v.Args[0], strVars, sliceVars)
			if !ok {
				return nil, false
			}
			combined := append([]string(nil), base...)
			for i, arg := range v.Args[1:] {
				if v.Ellipsis.IsValid() && i == len(v.Args)-2 {
					more, ok := evalASTStringSliceExpr(arg, strVars, sliceVars)
					if !ok {
						return nil, false
					}
					combined = append(combined, more...)
				} else {
					s, ok := evalASTStringExpr(arg, strVars)
					if !ok {
						return nil, false
					}
					combined = append(combined, s)
				}
			}
			return combined, true
		}
	}
	return nil, false
}

func TestRefreshStateASTChecker(t *testing.T) {
	schemas := acctest.BuildRefreshStateSchemaMap(map[string]*schema.Resource{
		"google_set_res": {
			Schema: map[string]*schema.Schema{
				"tags": {Type: schema.TypeSet, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
				"single_cfg": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"items": {Type: schema.TypeSet, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
						},
					},
				},
				"multi_cfg": {
					Type:     schema.TypeList,
					Optional: true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"items": {Type: schema.TypeSet, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
						},
					},
				},
			},
		},
		"google_etag_res": {
			Schema: map[string]*schema.Schema{
				"etag": {Type: schema.TypeString, Computed: true},
			},
		},
	}, nil)

	cases := []struct {
		name           string
		src            string
		wantViolations int
		wantRule       string
	}{
		{
			name: "missing RefreshState on TypeSet resource is caught",
			src: `package foo_test
import ("testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
func TestAccSynthetic_missing(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: "resource \"google_set_res\" \"a\" {}"},
			{ResourceName: "google_set_res.a", ImportState: true, ImportStateVerify: true},
		},
	})
}`,
			wantViolations: 1,
			wantRule:       acctest.RefreshStateRuleA,
		},
		{
			name: "present RefreshState passes",
			src: `package foo_test
import ("testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
func TestAccSynthetic_ok(t *testing.T) {
	resName := "google_set_res.a"
	acctest.VcrTest(t, resource.TestCase{
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{Config: "resource \"google_set_res\" \"a\" {}"},
			{RefreshState: true},
			{ResourceName: resName, ImportState: true, ImportStateVerify: true},
		},
	})
}`,
			wantViolations: 0,
		},
		{
			name: "per-step providers TestCase with RefreshState omitting step providers fails",
			src: `package foo_test
import ("testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
func TestAccSynthetic_missingStepProviders(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: "resource \"google_set_res\" \"a\" {}",
				ExternalProviders: map[string]resource.ExternalProvider{"google": {Source: "hashicorp/google"}},
			},
			{RefreshState: true},
			{
				ResourceName: "google_set_res.a",
				ImportState: true,
				ImportStateVerify: true,
				ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
			},
		},
	})
}`,
			wantViolations: 1,
			wantRule:       acctest.RefreshStateRuleStepProviders,
		},
		{
			name: "per-step providers TestCase with RefreshState omitting ExpectNonEmptyPlan fails",
			src: `package foo_test
import ("testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
func TestAccSynthetic_missingStepProvidersExpectNonEmptyPlan(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: "resource \"google_set_res\" \"a\" {}",
				ExternalProviders: map[string]resource.ExternalProvider{"google": {Source: "hashicorp/google"}},
			},
			{
				RefreshState: true,
				ExternalProviders: map[string]resource.ExternalProvider{"google": {Source: "hashicorp/google"}},
			},
			{
				ResourceName: "google_set_res.a",
				ImportState: true,
				ImportStateVerify: true,
				ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
			},
		},
	})
}`,
			wantViolations: 1,
			wantRule:       acctest.RefreshStateRuleStepProvidersExpectNonEmptyPlan,
		},
		{
			name: "per-step providers TestCase with RefreshState specifying step providers and ExpectNonEmptyPlan passes",
			src: `package foo_test
import ("testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
func TestAccSynthetic_withStepProviders(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: "resource \"google_set_res\" \"a\" {}",
				ExternalProviders: map[string]resource.ExternalProvider{"google": {Source: "hashicorp/google"}},
			},
			{
				RefreshState: true,
				ExpectNonEmptyPlan: true,
				ExternalProviders: map[string]resource.ExternalProvider{"google": {Source: "hashicorp/google"}},
			},
			{
				ResourceName: "google_set_res.a",
				ImportState: true,
				ImportStateVerify: true,
				ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
			},
		},
	})
}`,
			wantViolations: 0,
		},
		{
			name: "ignored top-level and nested sets pass",
			src: `package foo_test
import ("testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
var baseIgnore = []string{"tags", "single_cfg.0.items"}
func TestAccSynthetic_ignored(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: "resource \"google_set_res\" \"a\" {}"},
			{
				ResourceName: "google_set_res.a",
				ImportState: true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: append(baseIgnore, "multi_cfg"),
			},
		},
	})
}`,
			wantViolations: 0,
		},
		{
			name: "multi-item nested set ignored only at .0. fails",
			src: `package foo_test
import ("testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
func TestAccSynthetic_badMultiIgnore(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: "resource \"google_set_res\" \"a\" {}"},
			{
				ResourceName: "google_set_res.a",
				ImportState: true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"tags", "single_cfg.0.items", "multi_cfg.0.items"},
			},
		},
	})
}`,
			wantViolations: 1,
			wantRule:       acctest.RefreshStateRuleA,
		},
		{
			name: "helper function returning ImportStateVerify step is resolved and caught",
			src: `package foo_test
import ("fmt"; "testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
func customImportStep(name string) resource.TestStep {
	return resource.TestStep{
		ResourceName: name,
		ImportState: true,
		ImportStateVerify: true,
	}
}
func TestAccSynthetic_helper(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: "resource \"google_set_res\" \"a\" {}"},
			customImportStep(fmt.Sprintf("google_set_res.%s", "a")),
		},
	})
}`,
			wantViolations: 1,
			wantRule:       acctest.RefreshStateRuleA,
		},
		{
			name: "multi-import un-ignored etag without RefreshState fails Rule B",
			src: `package foo_test
import ("testing"; "github.com/hashicorp/terraform-plugin-testing/helper/resource"; "example/acctest")
func TestAccSynthetic_multiEtag(t *testing.T) {
	acctest.VcrTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: "resource \"google_etag_res\" \"a\" {}"},
			{ResourceName: "google_etag_res.a", ImportState: true, ImportStateVerify: true},
			{ResourceName: "google_etag_res.b", ImportState: true, ImportStateVerify: true},
		},
	})
}`,
			wantViolations: 1,
			wantRule:       acctest.RefreshStateRuleB,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			af, err := parser.ParseFile(fset, "synthetic_test.go", tc.src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("failed parsing synthetic AST: %v", err)
			}
			got := checkRefreshStateInParsedFiles(fset, []*ast.File{af}, []string{"synthetic_test.go"}, schemas)
			if len(got) != tc.wantViolations {
				t.Fatalf("got %d violations (%+v), want %d", len(got), got, tc.wantViolations)
			}
			if tc.wantViolations > 0 && got[0].Rule != tc.wantRule {
				t.Errorf("violation rule = %q, want %q", got[0].Rule, tc.wantRule)
			}
		})
	}
}
