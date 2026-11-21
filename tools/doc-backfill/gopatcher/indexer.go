package gopatcher

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type parsedVariantFile struct {
	filePath  string
	isTmpl    bool
	origBytes []byte
	codeBytes []byte
	tmpl      tmplInfo
	fset      *token.FileSet
	tfile     *token.File
	astFile   *ast.File
}

func (pf *parsedVariantFile) offset(pos token.Pos) int {
	return pf.tfile.Offset(pos)
}

func (pf *parsedVariantFile) exprCode(e ast.Expr) string {
	if e == nil {
		return ""
	}
	s := pf.offset(e.Pos())
	end := pf.offset(e.End())
	if s < 0 || end > len(pf.codeBytes) || s >= end {
		return ""
	}
	return strings.TrimSpace(string(pf.codeBytes[s:end]))
}

type funcSym struct {
	pf   *parsedVariantFile
	decl *ast.FuncDecl
}

type varSym struct {
	pf   *parsedVariantFile
	name string
	expr ast.Expr
}

type methodKey struct {
	recvType string
	name     string
}

type boundExpr struct {
	pf     *parsedVariantFile
	expr   ast.Expr
	helper string
}

type serviceVariantPkg struct {
	serviceName string
	files       []*parsedVariantFile
	funcs       map[string]funcSym
	methods     map[methodKey]funcSym
	vars        map[string]varSym
	// zeroArgResConstructors maps constructor func name (e.g. "ResourceComputeInstance")
	// to the canonical resource name it owns, preventing cloned schemas from traversing into it.
	zeroArgResConstructors map[string]string
}

// IndexServicesRoot indexes all service directories under servicesRoot and returns a deterministic
// map keyed by canonical ResourceName (including "google_<base>_iam" for handwritten IAM schemas).
func IndexServicesRoot(servicesRoot string) (map[string]*ResourceTarget, error) {
	entries, err := os.ReadDir(servicesRoot)
	if err != nil {
		return nil, fmt.Errorf("reading services root %s: %w", servicesRoot, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	out := make(map[string]*ResourceTarget)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		svcDir := filepath.Join(servicesRoot, entry.Name())
		targets, err := IndexServiceDir(svcDir)
		if err != nil {
			return nil, err
		}
		for _, rt := range targets {
			if existing, ok := out[rt.ResourceName]; ok {
				mergeResourceTargets(existing, rt)
			} else {
				out[rt.ResourceName] = rt
			}
		}
	}
	return out, nil
}

// IndexServiceDir indexes all handwritten SDKv2, Plugin Framework, and IAM resources in serviceDir.
func IndexServiceDir(serviceDir string) ([]*ResourceTarget, error) {
	cleanDir := filepath.Clean(serviceDir)
	serviceName := filepath.Base(cleanDir)

	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return nil, fmt.Errorf("reading service dir %s: %w", cleanDir, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	type rawServiceFile struct {
		filePath  string
		isTmpl    bool
		origBytes []byte
		tmpl      tmplInfo
		goParsed  *parsedVariantFile
	}

	var rawFiles []rawServiceFile
	hasTmpl := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if shouldSkipServiceFile(name) {
			continue
		}
		fullPath := filepath.Join(cleanDir, name)
		origBytes, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", fullPath, err)
		}
		isTmpl := strings.HasSuffix(name, ".go.tmpl")
		rf := rawServiceFile{
			filePath:  fullPath,
			isTmpl:    isTmpl,
			origBytes: origBytes,
		}
		if isTmpl {
			hasTmpl = true
			rf.tmpl = parseTemplateStructure(origBytes)
		} else {
			fset := token.NewFileSet()
			af, err := parser.ParseFile(fset, fullPath, origBytes, parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("parsing %s: %w", fullPath, err)
			}
			tf := fset.File(af.Pos())
			rf.goParsed = &parsedVariantFile{
				filePath:  fullPath,
				isTmpl:    false,
				origBytes: origBytes,
				codeBytes: origBytes,
				fset:      fset,
				tfile:     tf,
				astFile:   af,
			}
		}
		rawFiles = append(rawFiles, rf)
	}

	if len(rawFiles) == 0 {
		return nil, nil
	}

	versions := []string{"beta"}
	if hasTmpl {
		versions = []string{"beta", "ga"}
	}

	byName := make(map[string]*ResourceTarget)
	for _, ver := range versions {
		var variantFiles []*parsedVariantFile
		for _, rf := range rawFiles {
			if !rf.isTmpl {
				variantFiles = append(variantFiles, rf.goParsed)
				continue
			}
			masked := renderMaskedGoTemplate(rf.origBytes, rf.tmpl, ver)
			if strings.TrimSpace(string(masked)) == "" {
				// Entire file is conditionally excluded in this target version (e.g. beta-only file in GA).
				continue
			}
			fset := token.NewFileSet()
			af, err := parser.ParseFile(fset, rf.filePath, masked, parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("parsing template %s (version=%s): %w", rf.filePath, ver, err)
			}
			tf := fset.File(af.Pos())
			variantFiles = append(variantFiles, &parsedVariantFile{
				filePath:  rf.filePath,
				isTmpl:    true,
				origBytes: rf.origBytes,
				codeBytes: masked,
				tmpl:      rf.tmpl,
				fset:      fset,
				tfile:     tf,
				astFile:   af,
			})
		}

		pkg := buildServiceVariantPkg(serviceName, variantFiles)
		targets := pkg.indexVariantResources()
		for _, rt := range targets {
			if existing, ok := byName[rt.ResourceName]; ok {
				mergeResourceTargets(existing, rt)
			} else {
				byName[rt.ResourceName] = rt
			}
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]*ResourceTarget, 0, len(names))
	for _, name := range names {
		rt := byName[name]
		sortResourceTargetFields(rt)
		result = append(result, rt)
	}
	return result, nil
}

func shouldSkipServiceFile(name string) bool {
	if strings.Contains(name, "_test.go") || strings.Contains(name, "_sweeper.go") {
		return true
	}
	return !(strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".go.tmpl"))
}

func mergeResourceTargets(dst, src *ResourceTarget) {
	if dst.Fields == nil {
		dst.Fields = make(map[string][]*FieldTarget)
	}
	if !dst.HasResDescription && src.HasResDescription {
		dst.HasResDescription = src.HasResDescription
		dst.CurrentResDesc = src.CurrentResDesc
		dst.CanPatchResDesc = src.CanPatchResDesc
		dst.resPatchMeta = src.resPatchMeta
	}
	for path, list := range src.Fields {
		for _, ft := range list {
			appendUniqueFieldTarget(dst, path, ft)
		}
	}
}

func appendUniqueFieldTarget(rt *ResourceTarget, path string, ft *FieldTarget) {
	if rt.Fields == nil {
		rt.Fields = make(map[string][]*FieldTarget)
	}
	for _, existing := range rt.Fields[path] {
		if existing.TargetID == ft.TargetID {
			return
		}
	}
	rt.Fields[path] = append(rt.Fields[path], ft)
}

func sortResourceTargetFields(rt *ResourceTarget) {
	for path := range rt.Fields {
		list := rt.Fields[path]
		sort.Slice(list, func(i, j int) bool {
			return list[i].TargetID < list[j].TargetID
		})
	}
}

func buildServiceVariantPkg(serviceName string, files []*parsedVariantFile) *serviceVariantPkg {
	pkg := &serviceVariantPkg{
		serviceName:            serviceName,
		files:                  files,
		funcs:                  make(map[string]funcSym),
		methods:                make(map[methodKey]funcSym),
		vars:                   make(map[string]varSym),
		zeroArgResConstructors: make(map[string]string),
	}

	for _, pf := range files {
		for _, decl := range pf.astFile.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil || len(d.Recv.List) == 0 {
					if d.Name.Name != "init" {
						pkg.funcs[d.Name.Name] = funcSym{pf: pf, decl: d}
					}
				} else {
					recvName := extractReceiverTypeName(d.Recv.List[0].Type)
					if recvName != "" {
						pkg.methods[methodKey{recvType: recvName, name: d.Name.Name}] = funcSym{pf: pf, decl: d}
					}
				}
			case *ast.GenDecl:
				if d.Tok == token.VAR {
					for _, spec := range d.Specs {
						vs, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
						for idx, ident := range vs.Names {
							if idx < len(vs.Values) {
								pkg.vars[ident.Name] = varSym{
									pf:   pf,
									name: ident.Name,
									expr: vs.Values[idx],
								}
							}
						}
					}
				}
			}
		}
	}

	// Pre-scan zero-arg SDKv2 resource constructors so cloned resources (like
	// compute_instance_from_template calling ResourceComputeInstance().Schema) do not
	// claim another resource's schema fields.
	for _, pf := range files {
		for _, reg := range findRegistryRegistrations(pf) {
			if reg.regStruct == "registry.Schema" && reg.schemaType == "registry.SchemaTypeResource" && reg.schemaExpr != nil {
				if call, ok := reg.schemaExpr.(*ast.CallExpr); ok && len(call.Args) == 0 {
					if id, ok := call.Fun.(*ast.Ident); ok {
						pkg.zeroArgResConstructors[id.Name] = reg.name
					}
				}
			}
		}
	}

	return pkg
}

func extractReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return extractReceiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return extractReceiverTypeName(t.X)
	}
	return ""
}

type regEntry struct {
	pf         *parsedVariantFile
	regStruct  string // "registry.Schema" or "registry.FrameworkResource"
	name       string
	schemaType string
	schemaExpr ast.Expr
	funcExpr   ast.Expr
}

func findRegistryRegistrations(pf *parsedVariantFile) []regEntry {
	var out []regEntry
	ast.Inspect(pf.astFile, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Register" {
			return true
		}
		comp, ok := sel.X.(*ast.CompositeLit)
		if !ok || comp.Type == nil {
			return true
		}
		regStruct := pf.exprCode(comp.Type)
		if regStruct != "registry.Schema" && regStruct != "registry.FrameworkResource" {
			return true
		}
		entry := regEntry{
			pf:        pf,
			regStruct: regStruct,
		}
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key := pf.exprCode(kv.Key)
			switch key {
			case "Name":
				if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.STRING {
					if u, err := strconv.Unquote(bl.Value); err == nil {
						entry.name = u
					}
				}
			case "Type":
				entry.schemaType = pf.exprCode(kv.Value)
			case "Schema":
				entry.schemaExpr = kv.Value
			case "Func":
				entry.funcExpr = kv.Value
			}
		}
		if entry.name != "" {
			out = append(out, entry)
		}
		return true
	})
	return out
}

func (pkg *serviceVariantPkg) indexVariantResources() []*ResourceTarget {
	byName := make(map[string]*ResourceTarget)
	var order []string

	getOrCreate := func(resName string, kind SDKKind, primaryFile string) *ResourceTarget {
		if existing, ok := byName[resName]; ok {
			return existing
		}
		rt := &ResourceTarget{
			ResourceName: resName,
			Service:      pkg.serviceName,
			Kind:         kind,
			PrimaryFile:  primaryFile,
			Fields:       make(map[string][]*FieldTarget),
		}
		byName[resName] = rt
		order = append(order, resName)
		return rt
	}

	for _, pf := range pkg.files {
		for _, reg := range findRegistryRegistrations(pf) {
			switch {
			case reg.regStruct == "registry.Schema" && reg.schemaType == "registry.SchemaTypeResource":
				rt := getOrCreate(reg.name, SDKv2, pf.filePath)
				pkg.indexSDKv2Resource(rt, pf, reg.schemaExpr)

			case reg.regStruct == "registry.FrameworkResource":
				rt := getOrCreate(reg.name, PluginFramework, pf.filePath)
				pkg.indexFrameworkResource(rt, pf, reg.funcExpr)

			case reg.regStruct == "registry.Schema" && reg.schemaType == "registry.SchemaTypeIAMResource":
				iamName := canonicalIAMResourceName(reg.name)
				rt := getOrCreate(iamName, IAM, pf.filePath)
				pkg.indexIAMResource(rt, pf, reg.schemaExpr)
			}
		}
	}

	out := make([]*ResourceTarget, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

func canonicalIAMResourceName(regName string) string {
	for _, suf := range []string{"_iam_binding", "_iam_member", "_iam_policy", "_iam_audit_config"} {
		if strings.HasSuffix(regName, suf) {
			return strings.TrimSuffix(regName, suf) + "_iam"
		}
	}
	if idx := strings.Index(regName, "_iam_"); idx != -1 {
		return regName[:idx] + "_iam"
	}
	return regName
}

// sdkWalkCtx holds per-resource traversal state for SDKv2 and IAM schema indexing.
type sdkWalkCtx struct {
	pkg          *serviceVariantPkg
	rt           *ResourceTarget
	ownerResName string
	rootFunc     string
	activeFuncs  map[string]bool
	activeVars   map[string]bool
}

type resolvedSchemaMap struct {
	keys    []string
	entries map[string]boundExpr
}

func newResolvedSchemaMap() *resolvedSchemaMap {
	return &resolvedSchemaMap{
		entries: make(map[string]boundExpr),
	}
}

func (m *resolvedSchemaMap) set(key string, val boundExpr) {
	if _, exists := m.entries[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.entries[key] = val
}

func (m *resolvedSchemaMap) remove(key string) {
	delete(m.entries, key)
}

func (m *resolvedSchemaMap) mergeFrom(other *resolvedSchemaMap) {
	if other == nil {
		return
	}
	for _, k := range other.keys {
		if v, ok := other.entries[k]; ok {
			m.set(k, v)
		}
	}
}

type resolvedResource struct {
	schemaMap  *resolvedSchemaMap
	rootLit    *ast.CompositeLit
	rootLitPF  *parsedVariantFile
	blockedRef bool
}

func (pkg *serviceVariantPkg) indexSDKv2Resource(rt *ResourceTarget, pf *parsedVariantFile, schemaExpr ast.Expr) {
	if schemaExpr == nil {
		return
	}
	rootFunc := ""
	if call, ok := schemaExpr.(*ast.CallExpr); ok {
		if id, ok := call.Fun.(*ast.Ident); ok {
			rootFunc = id.Name
		}
	}
	ctx := &sdkWalkCtx{
		pkg:          pkg,
		rt:           rt,
		ownerResName: rt.ResourceName,
		rootFunc:     rootFunc,
		activeFuncs:  make(map[string]bool),
		activeVars:   make(map[string]bool),
	}
	res := ctx.evalResourceExpr(pf, schemaExpr, "", nil, true)
	if res == nil || res.blockedRef {
		return
	}
	if res.rootLit != nil && res.rootLitPF != nil {
		hasDesc, curDesc, _, meta := inspectCompositeDescription(res.rootLitPF, res.rootLit)
		if hasDesc {
			rt.HasResDescription = true
			rt.CurrentResDesc = curDesc
			rt.resPatchMeta = meta
		}
	}
	if res.schemaMap != nil {
		ctx.emitSchemaMap("", res.schemaMap, nil)
	}
}

func (pkg *serviceVariantPkg) indexIAMResource(rt *ResourceTarget, pf *parsedVariantFile, schemaExpr ast.Expr) {
	if schemaExpr == nil {
		return
	}
	parentExpr, parentPF := pkg.findIAMParentSchemaExpr(pf, schemaExpr, make(map[string]bool))
	if parentExpr == nil || parentPF == nil {
		return
	}
	ctx := &sdkWalkCtx{
		pkg:          pkg,
		rt:           rt,
		ownerResName: rt.ResourceName,
		rootFunc:     "",
		activeFuncs:  make(map[string]bool),
		activeVars:   make(map[string]bool),
	}
	smap := ctx.evalSchemaMapExpr(parentPF, parentExpr, "", nil)
	if smap != nil {
		ctx.emitSchemaMap("", smap, nil)
	}
}

func (pkg *serviceVariantPkg) findIAMParentSchemaExpr(pf *parsedVariantFile, expr ast.Expr, seen map[string]bool) (ast.Expr, *parsedVariantFile) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, nil
	}
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		pkgIdent, _ := fun.X.(*ast.Ident)
		if pkgIdent != nil && pkgIdent.Name == "tpgiamresource" && strings.HasPrefix(fun.Sel.Name, "ResourceIam") && len(call.Args) > 0 {
			return call.Args[0], pf
		}
	case *ast.Ident:
		if seen[fun.Name] {
			return nil, nil
		}
		seen[fun.Name] = true
		if fnSym, ok := pkg.funcs[fun.Name]; ok && fnSym.decl.Body != nil {
			var foundExpr ast.Expr
			var foundPF *parsedVariantFile
			ast.Inspect(fnSym.decl.Body, func(n ast.Node) bool {
				if foundExpr != nil {
					return false
				}
				ic, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				e, f := pkg.findIAMParentSchemaExpr(fnSym.pf, ic, seen)
				if e != nil {
					foundExpr = e
					foundPF = f
					return false
				}
				return true
			})
			return foundExpr, foundPF
		}
	}
	return nil, nil
}

func (ctx *sdkWalkCtx) helperForFunc(fnName string, currentHelper string) string {
	if fnName == ctx.rootFunc {
		return currentHelper
	}
	return fnName
}

func (ctx *sdkWalkCtx) isBlockedOtherResource(fnName string, isDirectRootCall bool) bool {
	if isDirectRootCall && fnName == ctx.rootFunc {
		return false
	}
	if owner, ok := ctx.pkg.zeroArgResConstructors[fnName]; ok && owner != ctx.ownerResName {
		return true
	}
	return false
}

func (ctx *sdkWalkCtx) evalResourceExpr(pf *parsedVariantFile, expr ast.Expr, currentHelper string, params map[string]boundExpr, isDirectRootCall bool) *resolvedResource {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return ctx.evalResourceExpr(pf, e.X, currentHelper, params, isDirectRootCall)
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return ctx.evalResourceExpr(pf, e.X, currentHelper, params, isDirectRootCall)
		}
	case *ast.CompositeLit:
		if !isResourceCompositeLit(pf, e) {
			return nil
		}
		res := &resolvedResource{
			schemaMap: newResolvedSchemaMap(),
			rootLit:   e,
			rootLitPF: pf,
		}
		for _, elt := range e.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if pf.exprCode(kv.Key) == "Schema" {
				smap := ctx.evalSchemaMapExpr(pf, kv.Value, currentHelper, params)
				res.schemaMap.mergeFrom(smap)
			}
		}
		return res

	case *ast.Ident:
		if b, ok := params[e.Name]; ok {
			return ctx.evalResourceExpr(b.pf, b.expr, b.helper, nil, false)
		}
		if ctx.activeVars[e.Name] {
			return nil
		}
		if vs, ok := ctx.pkg.vars[e.Name]; ok {
			ctx.activeVars[e.Name] = true
			defer delete(ctx.activeVars, e.Name)
			return ctx.evalResourceExpr(vs.pf, vs.expr, e.Name, nil, false)
		}

	case *ast.CallExpr:
		id, ok := e.Fun.(*ast.Ident)
		if !ok {
			return nil
		}
		if ctx.isBlockedOtherResource(id.Name, isDirectRootCall) {
			return &resolvedResource{blockedRef: true}
		}
		if ctx.activeFuncs[id.Name] {
			return nil
		}
		fnSym, ok := ctx.pkg.funcs[id.Name]
		if !ok || fnSym.decl.Body == nil {
			return nil
		}
		nextHelper := ctx.helperForFunc(id.Name, currentHelper)
		bound := bindCallParams(pf, e.Args, fnSym, currentHelper, params)
		ctx.activeFuncs[id.Name] = true
		defer delete(ctx.activeFuncs, id.Name)
		return ctx.evalFuncBodyForResource(fnSym, nextHelper, bound)
	}
	return nil
}

func isResourceCompositeLit(pf *parsedVariantFile, comp *ast.CompositeLit) bool {
	if comp.Type == nil {
		return false
	}
	ts := pf.exprCode(comp.Type)
	return ts == "schema.Resource" || strings.HasSuffix(ts, ".Resource")
}

func bindCallParams(callerPF *parsedVariantFile, args []ast.Expr, callee funcSym, callerHelper string, callerParams map[string]boundExpr) map[string]boundExpr {
	if callee.decl.Type.Params == nil || len(args) == 0 {
		return nil
	}
	var paramNames []string
	for _, field := range callee.decl.Type.Params.List {
		for _, name := range field.Names {
			paramNames = append(paramNames, name.Name)
		}
	}
	if len(paramNames) == 0 {
		return nil
	}
	out := make(map[string]boundExpr, len(paramNames))
	for i, pName := range paramNames {
		if i >= len(args) {
			break
		}
		arg := args[i]
		if id, ok := arg.(*ast.Ident); ok {
			if existing, ok := callerParams[id.Name]; ok {
				out[pName] = existing
				continue
			}
		}
		out[pName] = boundExpr{
			pf:     callerPF,
			expr:   arg,
			helper: callerHelper,
		}
	}
	return out
}

func (ctx *sdkWalkCtx) evalFuncBodyForResource(fnSym funcSym, helper string, params map[string]boundExpr) *resolvedResource {
	localRes := make(map[string]*resolvedResource)
	localMaps := make(map[string]*resolvedSchemaMap)
	var returned *resolvedResource

	var walkStmts func(stmts []ast.Stmt)
	walkStmts = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *ast.AssignStmt:
				ctx.handleFuncAssign(fnSym.pf, s, helper, params, localRes, localMaps)
			case *ast.DeclStmt:
				if gd, ok := s.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
					for _, sp := range gd.Specs {
						if vs, ok := sp.(*ast.ValueSpec); ok {
							for idx, nm := range vs.Names {
								if idx < len(vs.Values) {
									if r := ctx.evalResourceExprWithLocals(fnSym.pf, vs.Values[idx], helper, params, localRes, localMaps); r != nil {
										localRes[nm.Name] = r
									} else if m := ctx.evalSchemaMapExprWithLocals(fnSym.pf, vs.Values[idx], helper, params, localRes, localMaps); m != nil {
										localMaps[nm.Name] = m
									}
								}
							}
						}
					}
				}
			case *ast.ExprStmt:
				ctx.handleFuncExprStmt(fnSym.pf, s.X, localMaps)
			case *ast.IfStmt:
				if s.Body != nil {
					walkStmts(s.Body.List)
				}
				if s.Else != nil {
					switch el := s.Else.(type) {
					case *ast.BlockStmt:
						walkStmts(el.List)
					case *ast.IfStmt:
						walkStmts([]ast.Stmt{el})
					}
				}
			case *ast.BlockStmt:
				walkStmts(s.List)
			case *ast.ReturnStmt:
				for _, resExpr := range s.Results {
					r := ctx.evalResourceExprWithLocals(fnSym.pf, resExpr, helper, params, localRes, localMaps)
					if r != nil && !r.blockedRef {
						if returned == nil {
							returned = r
						} else {
							returned.schemaMap.mergeFrom(r.schemaMap)
						}
					}
				}
			}
		}
	}

	walkStmts(fnSym.decl.Body.List)
	return returned
}

func (ctx *sdkWalkCtx) evalFuncBodyForSchemaMap(fnSym funcSym, helper string, params map[string]boundExpr) *resolvedSchemaMap {
	localRes := make(map[string]*resolvedResource)
	localMaps := make(map[string]*resolvedSchemaMap)
	var returned *resolvedSchemaMap

	var walkStmts func(stmts []ast.Stmt)
	walkStmts = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *ast.AssignStmt:
				ctx.handleFuncAssign(fnSym.pf, s, helper, params, localRes, localMaps)
			case *ast.DeclStmt:
				if gd, ok := s.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
					for _, sp := range gd.Specs {
						if vs, ok := sp.(*ast.ValueSpec); ok {
							for idx, nm := range vs.Names {
								if idx < len(vs.Values) {
									if m := ctx.evalSchemaMapExprWithLocals(fnSym.pf, vs.Values[idx], helper, params, localRes, localMaps); m != nil {
										localMaps[nm.Name] = m
									} else if r := ctx.evalResourceExprWithLocals(fnSym.pf, vs.Values[idx], helper, params, localRes, localMaps); r != nil {
										localRes[nm.Name] = r
									}
								}
							}
						}
					}
				}
			case *ast.ExprStmt:
				ctx.handleFuncExprStmt(fnSym.pf, s.X, localMaps)
			case *ast.IfStmt:
				if s.Body != nil {
					walkStmts(s.Body.List)
				}
				if s.Else != nil {
					switch el := s.Else.(type) {
					case *ast.BlockStmt:
						walkStmts(el.List)
					case *ast.IfStmt:
						walkStmts([]ast.Stmt{el})
					}
				}
			case *ast.BlockStmt:
				walkStmts(s.List)
			case *ast.ReturnStmt:
				for _, resExpr := range s.Results {
					m := ctx.evalSchemaMapExprWithLocals(fnSym.pf, resExpr, helper, params, localRes, localMaps)
					if m != nil {
						if returned == nil {
							returned = newResolvedSchemaMap()
						}
						returned.mergeFrom(m)
					}
				}
			}
		}
	}

	walkStmts(fnSym.decl.Body.List)
	return returned
}

func (ctx *sdkWalkCtx) handleFuncAssign(
	pf *parsedVariantFile,
	s *ast.AssignStmt,
	helper string,
	params map[string]boundExpr,
	localRes map[string]*resolvedResource,
	localMaps map[string]*resolvedSchemaMap,
) {
	if len(s.Lhs) != len(s.Rhs) {
		return
	}
	for i := range s.Lhs {
		lhs := s.Lhs[i]
		rhs := s.Rhs[i]
		switch l := lhs.(type) {
		case *ast.Ident:
			if r := ctx.evalResourceExprWithLocals(pf, rhs, helper, params, localRes, localMaps); r != nil {
				if r.blockedRef {
					localRes[l.Name] = &resolvedResource{schemaMap: newResolvedSchemaMap()}
				} else {
					localRes[l.Name] = r
				}
			} else if m := ctx.evalSchemaMapExprWithLocals(pf, rhs, helper, params, localRes, localMaps); m != nil {
				localMaps[l.Name] = m
			}
		case *ast.IndexExpr:
			keyLit, ok := l.Index.(*ast.BasicLit)
			if !ok || keyLit.Kind != token.STRING {
				continue
			}
			fieldKey, err := strconv.Unquote(keyLit.Value)
			if err != nil || fieldKey == "" {
				continue
			}
			switch target := l.X.(type) {
			case *ast.Ident:
				m, exists := localMaps[target.Name]
				if !exists {
					m = newResolvedSchemaMap()
					localMaps[target.Name] = m
				}
				m.set(fieldKey, boundExpr{pf: pf, expr: rhs, helper: helper})
			case *ast.SelectorExpr:
				if target.Sel.Name == "Schema" {
					if recvIdent, ok := target.X.(*ast.Ident); ok {
						r, exists := localRes[recvIdent.Name]
						if !exists {
							r = &resolvedResource{schemaMap: newResolvedSchemaMap()}
							localRes[recvIdent.Name] = r
						}
						r.schemaMap.set(fieldKey, boundExpr{pf: pf, expr: rhs, helper: helper})
					}
				}
			}
		}
	}
}

func (ctx *sdkWalkCtx) handleFuncExprStmt(pf *parsedVariantFile, expr ast.Expr, localMaps map[string]*resolvedSchemaMap) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return
	}
	if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "delete" {
		if mapId, ok := call.Args[0].(*ast.Ident); ok {
			if keyLit, ok := call.Args[1].(*ast.BasicLit); ok && keyLit.Kind == token.STRING {
				if k, err := strconv.Unquote(keyLit.Value); err == nil {
					if m, exists := localMaps[mapId.Name]; exists {
						m.remove(k)
					}
				}
			}
		}
	}
}

func (ctx *sdkWalkCtx) evalResourceExprWithLocals(
	pf *parsedVariantFile,
	expr ast.Expr,
	helper string,
	params map[string]boundExpr,
	localRes map[string]*resolvedResource,
	localMaps map[string]*resolvedSchemaMap,
) *resolvedResource {
	if id, ok := expr.(*ast.Ident); ok {
		if r, exists := localRes[id.Name]; exists {
			return r
		}
	}
	if comp, ok := unwrapAddrComposite(expr); ok && isResourceCompositeLit(pf, comp) {
		res := &resolvedResource{
			schemaMap: newResolvedSchemaMap(),
			rootLit:   comp,
			rootLitPF: pf,
		}
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if pf.exprCode(kv.Key) == "Schema" {
				smap := ctx.evalSchemaMapExprWithLocals(pf, kv.Value, helper, params, localRes, localMaps)
				res.schemaMap.mergeFrom(smap)
			}
		}
		return res
	}
	return ctx.evalResourceExpr(pf, expr, helper, params, false)
}

func (ctx *sdkWalkCtx) evalSchemaMapExprWithLocals(
	pf *parsedVariantFile,
	expr ast.Expr,
	helper string,
	params map[string]boundExpr,
	localRes map[string]*resolvedResource,
	localMaps map[string]*resolvedSchemaMap,
) *resolvedSchemaMap {
	if id, ok := expr.(*ast.Ident); ok {
		if m, exists := localMaps[id.Name]; exists {
			return m
		}
	}
	if sel, ok := expr.(*ast.SelectorExpr); ok && sel.Sel.Name == "Schema" {
		r := ctx.evalResourceExprWithLocals(pf, sel.X, helper, params, localRes, localMaps)
		if r == nil || r.blockedRef {
			return newResolvedSchemaMap()
		}
		return r.schemaMap
	}
	return ctx.evalSchemaMapExpr(pf, expr, helper, params)
}

func (ctx *sdkWalkCtx) evalSchemaMapExpr(pf *parsedVariantFile, expr ast.Expr, currentHelper string, params map[string]boundExpr) *resolvedSchemaMap {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return ctx.evalSchemaMapExpr(pf, e.X, currentHelper, params)
	case *ast.CompositeLit:
		if !isSchemaMapCompositeLit(pf, e) {
			return nil
		}
		out := newResolvedSchemaMap()
		for _, elt := range e.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			kl, ok := kv.Key.(*ast.BasicLit)
			if !ok || kl.Kind != token.STRING {
				continue
			}
			fieldName, err := strconv.Unquote(kl.Value)
			if err != nil || fieldName == "" {
				continue
			}
			out.set(fieldName, boundExpr{pf: pf, expr: kv.Value, helper: currentHelper})
		}
		return out

	case *ast.Ident:
		if b, ok := params[e.Name]; ok {
			h := b.helper
			if vid, ok := b.expr.(*ast.Ident); ok {
				if _, isPkgVar := ctx.pkg.vars[vid.Name]; isPkgVar {
					h = vid.Name
				}
			}
			return ctx.evalSchemaMapExpr(b.pf, b.expr, h, nil)
		}
		if ctx.activeVars[e.Name] {
			return nil
		}
		if vs, ok := ctx.pkg.vars[e.Name]; ok {
			ctx.activeVars[e.Name] = true
			defer delete(ctx.activeVars, e.Name)
			return ctx.evalSchemaMapExpr(vs.pf, vs.expr, e.Name, nil)
		}

	case *ast.SelectorExpr:
		if e.Sel.Name == "Schema" {
			r := ctx.evalResourceExpr(pf, e.X, currentHelper, params, false)
			if r == nil || r.blockedRef {
				return newResolvedSchemaMap()
			}
			return r.schemaMap
		}

	case *ast.CallExpr:
		// Support tpgresource.MergeSchemas(m1, m2, ...)
		if isMergeSchemasCall(e.Fun) {
			merged := newResolvedSchemaMap()
			for _, arg := range e.Args {
				sub := ctx.evalSchemaMapExpr(pf, arg, currentHelper, params)
				merged.mergeFrom(sub)
			}
			return merged
		}
		id, ok := e.Fun.(*ast.Ident)
		if !ok {
			return nil
		}
		if ctx.isBlockedOtherResource(id.Name, false) {
			return newResolvedSchemaMap()
		}
		if ctx.activeFuncs[id.Name] {
			return nil
		}
		fnSym, ok := ctx.pkg.funcs[id.Name]
		if !ok || fnSym.decl.Body == nil {
			return nil
		}
		nextHelper := ctx.helperForFunc(id.Name, currentHelper)
		bound := bindCallParams(pf, e.Args, fnSym, currentHelper, params)
		ctx.activeFuncs[id.Name] = true
		defer delete(ctx.activeFuncs, id.Name)
		return ctx.evalFuncBodyForSchemaMap(fnSym, nextHelper, bound)
	}
	return nil
}

func isSchemaMapCompositeLit(pf *parsedVariantFile, comp *ast.CompositeLit) bool {
	mt, ok := comp.Type.(*ast.MapType)
	if !ok {
		return false
	}
	ts := pf.exprCode(mt)
	return strings.Contains(ts, "schema.Schema")
}

func isMergeSchemasCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "MergeSchemas"
	case *ast.SelectorExpr:
		return f.Sel.Name == "MergeSchemas"
	}
	return false
}

func unwrapAddrComposite(expr ast.Expr) (*ast.CompositeLit, bool) {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return e, true
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			if cl, ok := e.X.(*ast.CompositeLit); ok {
				return cl, true
			}
		}
	}
	return nil, false
}

func joinFieldPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func (ctx *sdkWalkCtx) emitSchemaMap(prefix string, smap *resolvedSchemaMap, params map[string]boundExpr) {
	if smap == nil {
		return
	}
	for _, key := range smap.keys {
		be, ok := smap.entries[key]
		if !ok {
			continue
		}
		fieldPath := joinFieldPath(prefix, key)
		ctx.walkSDKv2FieldExpr(fieldPath, be.pf, be.expr, be.helper, params)
	}
}

func (ctx *sdkWalkCtx) walkSDKv2FieldExpr(fieldPath string, pf *parsedVariantFile, expr ast.Expr, currentHelper string, params map[string]boundExpr) {
	if expr == nil {
		return
	}
	if comp, ok := unwrapAddrComposite(expr); ok {
		// Distinguish &schema.Resource (shouldn't directly be a map[string]*schema.Schema value) vs *schema.Schema/{...}
		if comp.Type != nil && isResourceCompositeLit(pf, comp) {
			return
		}
		ft := buildFieldTarget(pf, comp, fieldPath, currentHelper)
		appendUniqueFieldTarget(ctx.rt, fieldPath, ft)

		// Inspect Elem for nested *schema.Resource blocks (inline, var, or helper func).
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if pf.exprCode(kv.Key) == "Elem" {
				ctx.walkSDKv2ElemExpr(fieldPath, pf, kv.Value, currentHelper, params)
			}
		}
		return
	}

	switch e := expr.(type) {
	case *ast.ParenExpr:
		ctx.walkSDKv2FieldExpr(fieldPath, pf, e.X, currentHelper, params)
	case *ast.Ident:
		if b, ok := params[e.Name]; ok {
			ctx.walkSDKv2FieldExpr(fieldPath, b.pf, b.expr, b.helper, nil)
			return
		}
		if ctx.activeVars[e.Name] {
			return
		}
		if vs, ok := ctx.pkg.vars[e.Name]; ok {
			ctx.activeVars[e.Name] = true
			defer delete(ctx.activeVars, e.Name)
			ctx.walkSDKv2FieldExpr(fieldPath, vs.pf, vs.expr, e.Name, nil)
		}
	case *ast.CallExpr:
		id, ok := e.Fun.(*ast.Ident)
		if !ok {
			// External package helper (e.g. tpgresource.DeletionPolicySchemaEntry): do not index.
			return
		}
		if ctx.isBlockedOtherResource(id.Name, false) || ctx.activeFuncs[id.Name] {
			return
		}
		fnSym, ok := ctx.pkg.funcs[id.Name]
		if !ok || fnSym.decl.Body == nil {
			return
		}
		nextHelper := ctx.helperForFunc(id.Name, currentHelper)
		bound := bindCallParams(pf, e.Args, fnSym, currentHelper, params)
		ctx.activeFuncs[id.Name] = true
		defer delete(ctx.activeFuncs, id.Name)

		// Find returned *schema.Schema expressions in helper body.
		ctx.walkReturnedSchemaExprs(fieldPath, fnSym, nextHelper, bound)
	}
}

func (ctx *sdkWalkCtx) walkReturnedSchemaExprs(fieldPath string, fnSym funcSym, helper string, params map[string]boundExpr) {
	localSchemaVars := make(map[string]ast.Expr)
	ast.Inspect(fnSym.decl.Body, func(n ast.Node) bool {
		// Do not descend into nested anonymous funcs (e.g. DiffSuppressFunc / ValidateFunc / StateFunc).
		if _, isFuncLit := n.(*ast.FuncLit); isFuncLit {
			return false
		}
		switch st := n.(type) {
		case *ast.AssignStmt:
			if len(st.Lhs) == len(st.Rhs) {
				for i := range st.Lhs {
					if id, ok := st.Lhs[i].(*ast.Ident); ok {
						localSchemaVars[id.Name] = st.Rhs[i]
					}
				}
			}
		case *ast.ReturnStmt:
			for _, res := range st.Results {
				target := res
				if id, ok := res.(*ast.Ident); ok {
					if v, exists := localSchemaVars[id.Name]; exists {
						target = v
					}
				}
				ctx.walkSDKv2FieldExpr(fieldPath, fnSym.pf, target, helper, params)
			}
		}
		return true
	})
}

func (ctx *sdkWalkCtx) walkSDKv2ElemExpr(fieldPath string, pf *parsedVariantFile, elemExpr ast.Expr, currentHelper string, params map[string]boundExpr) {
	// If Elem is &schema.Schema{...} (list/set/map of primitives), do NOT treat it as a nested block.
	if comp, ok := unwrapAddrComposite(elemExpr); ok {
		if isResourceCompositeLit(pf, comp) {
			res := ctx.evalResourceExpr(pf, comp, currentHelper, params, false)
			if res != nil && res.schemaMap != nil {
				ctx.emitSchemaMap(fieldPath, res.schemaMap, params)
			}
		}
		return
	}
	// Elem may be a helper call (e.g. DataplexZoneDiscoverySpecSchema()) or a package var (e.g. cidrBlockConfig)
	// returning *schema.Resource. evalResourceExpr returns non-nil ONLY if the resolved expression is *schema.Resource.
	res := ctx.evalResourceExpr(pf, elemExpr, currentHelper, params, false)
	if res != nil && !res.blockedRef && res.schemaMap != nil {
		ctx.emitSchemaMap(fieldPath, res.schemaMap, params)
	}
}

// --- Plugin Framework Indexing ---

type fwWalkCtx struct {
	pkg         *serviceVariantPkg
	rt          *ResourceTarget
	activeFuncs map[string]bool
	activeVars  map[string]bool
}

func (pkg *serviceVariantPkg) indexFrameworkResource(rt *ResourceTarget, pf *parsedVariantFile, funcExpr ast.Expr) {
	if funcExpr == nil {
		return
	}
	recvType := pkg.resolveFrameworkReceiverType(pf, funcExpr)
	if recvType == "" {
		return
	}
	schemaMethod, ok := pkg.methods[methodKey{recvType: recvType, name: "Schema"}]
	if !ok || schemaMethod.decl.Body == nil {
		return
	}

	ctx := &fwWalkCtx{
		pkg:         pkg,
		rt:          rt,
		activeFuncs: make(map[string]bool),
		activeVars:  make(map[string]bool),
	}

	// Find resp.Schema = ... inside Schema method
	ast.Inspect(schemaMethod.decl.Body, func(n ast.Node) bool {
		if _, isFuncLit := n.(*ast.FuncLit); isFuncLit {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		sel, ok := as.Lhs[0].(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Schema" {
			return true
		}
		ctx.walkFrameworkRootSchema(schemaMethod.pf, as.Rhs[0], "")
		return false
	})
}

func (pkg *serviceVariantPkg) resolveFrameworkReceiverType(pf *parsedVariantFile, funcExpr ast.Expr) string {
	var body *ast.BlockStmt
	switch f := funcExpr.(type) {
	case *ast.Ident:
		fnSym, ok := pkg.funcs[f.Name]
		if !ok {
			return ""
		}
		body = fnSym.decl.Body
	case *ast.FuncLit:
		body = f.Body
	}
	if body == nil {
		return ""
	}

	localTypes := make(map[string]string)
	var found string
	ast.Inspect(body, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		switch st := n.(type) {
		case *ast.AssignStmt:
			if len(st.Lhs) == 1 && len(st.Rhs) == 1 {
				if id, ok := st.Lhs[0].(*ast.Ident); ok {
					if tn := extractConstructedStructName(st.Rhs[0]); tn != "" {
						localTypes[id.Name] = tn
					}
				}
			}
		case *ast.ReturnStmt:
			for _, r := range st.Results {
				if tn := extractConstructedStructName(r); tn != "" {
					found = tn
					return false
				}
				if id, ok := r.(*ast.Ident); ok {
					if tn, exists := localTypes[id.Name]; exists {
						found = tn
						return false
					}
				}
			}
		}
		return true
	})
	return found
}

func extractConstructedStructName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return extractConstructedStructName(e.X)
		}
	case *ast.CompositeLit:
		if id, ok := e.Type.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "new" && len(e.Args) == 1 {
			if tid, ok := e.Args[0].(*ast.Ident); ok {
				return tid.Name
			}
		}
	}
	return ""
}

func (ctx *fwWalkCtx) walkFrameworkRootSchema(pf *parsedVariantFile, expr ast.Expr, helper string) {
	comp, compPF, nextHelper := ctx.resolveFrameworkComposite(pf, expr, helper)
	if comp == nil || compPF == nil {
		return
	}
	hasDesc, curDesc, isLitOrMissing, meta := inspectCompositeDescription(compPF, comp)
	ctx.rt.HasResDescription = hasDesc
	ctx.rt.CurrentResDesc = curDesc
	ctx.rt.CanPatchResDesc = isLitOrMissing
	ctx.rt.resPatchMeta = meta

	ctx.walkFrameworkContainerFields("", compPF, comp, nextHelper)
}

func (ctx *fwWalkCtx) resolveFrameworkComposite(pf *parsedVariantFile, expr ast.Expr, helper string) (*ast.CompositeLit, *parsedVariantFile, string) {
	if comp, ok := unwrapAddrComposite(expr); ok {
		return comp, pf, helper
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if ctx.activeVars[e.Name] {
			return nil, nil, helper
		}
		if vs, ok := ctx.pkg.vars[e.Name]; ok {
			ctx.activeVars[e.Name] = true
			defer delete(ctx.activeVars, e.Name)
			return ctx.resolveFrameworkComposite(vs.pf, vs.expr, e.Name)
		}
	case *ast.CallExpr:
		id, ok := e.Fun.(*ast.Ident)
		if !ok || ctx.activeFuncs[id.Name] {
			return nil, nil, helper
		}
		fnSym, ok := ctx.pkg.funcs[id.Name]
		if !ok || fnSym.decl.Body == nil {
			return nil, nil, helper
		}
		ctx.activeFuncs[id.Name] = true
		defer delete(ctx.activeFuncs, id.Name)
		var foundComp *ast.CompositeLit
		ast.Inspect(fnSym.decl.Body, func(n ast.Node) bool {
			if foundComp != nil {
				return false
			}
			if _, isFuncLit := n.(*ast.FuncLit); isFuncLit {
				return false
			}
			if ret, ok := n.(*ast.ReturnStmt); ok {
				for _, r := range ret.Results {
					if c, ok := unwrapAddrComposite(r); ok {
						foundComp = c
						return false
					}
				}
			}
			return true
		})
		if foundComp != nil {
			return foundComp, fnSym.pf, id.Name
		}
	}
	return nil, nil, helper
}

func (ctx *fwWalkCtx) walkFrameworkContainerFields(prefix string, pf *parsedVariantFile, comp *ast.CompositeLit, helper string) {
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key := pf.exprCode(kv.Key)
		switch key {
		case "Attributes", "Blocks":
			ctx.walkFrameworkFieldMap(prefix, pf, kv.Value, helper)
		case "NestedObject":
			nestedComp, nestedPF, nestedHelper := ctx.resolveFrameworkComposite(pf, kv.Value, helper)
			if nestedComp != nil && nestedPF != nil {
				ctx.walkFrameworkContainerFields(prefix, nestedPF, nestedComp, nestedHelper)
			}
		}
	}
}

func (ctx *fwWalkCtx) walkFrameworkFieldMap(prefix string, pf *parsedVariantFile, mapExpr ast.Expr, helper string) {
	comp, compPF, mapHelper := ctx.resolveFrameworkComposite(pf, mapExpr, helper)
	if comp == nil || compPF == nil {
		return
	}
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		kl, ok := kv.Key.(*ast.BasicLit)
		if !ok || kl.Kind != token.STRING {
			continue
		}
		name, err := strconv.Unquote(kl.Value)
		if err != nil || name == "" {
			continue
		}
		fieldPath := joinFieldPath(prefix, name)
		fieldComp, fieldPF, fieldHelper := ctx.resolveFrameworkComposite(compPF, kv.Value, mapHelper)
		if fieldComp == nil || fieldPF == nil {
			continue
		}
		ft := buildFieldTarget(fieldPF, fieldComp, fieldPath, fieldHelper)
		appendUniqueFieldTarget(ctx.rt, fieldPath, ft)
		ctx.walkFrameworkContainerFields(fieldPath, fieldPF, fieldComp, fieldHelper)
	}
}

// --- Composite Literal Inspection & Insertion Metadata ---

var scalarAnchorKeys = map[string]bool{
	"Version":     true,
	"Type":        true,
	"ElementType": true,
	"Required":    true,
	"Optional":    true,
	"Computed":    true,
	"ForceNew":    true,
	"Sensitive":   true,
	"WriteOnly":   true,
	"Default":     true,
	"MaxItems":    true,
	"MinItems":    true,
}

func buildFieldTarget(pf *parsedVariantFile, comp *ast.CompositeLit, fieldPath string, sharedHelper string) *FieldTarget {
	hasDesc, curDesc, isLitOrMissing, meta := inspectCompositeDescription(pf, comp)
	return &FieldTarget{
		Path:               fieldPath,
		FilePath:           pf.filePath,
		TargetID:           fmt.Sprintf("%s:%d", pf.filePath, meta.compStart),
		HasDescription:     hasDesc,
		CurrentDescription: curDesc,
		IsLiteralOrMissing: isLitOrMissing,
		SharedHelper:       sharedHelper,
		patchMeta:          meta,
	}
}

func inspectCompositeDescription(pf *parsedVariantFile, comp *ast.CompositeLit) (hasDesc bool, currentDesc string, isLitOrMissing bool, meta compositePatchMeta) {
	compStart := pf.offset(comp.Pos())
	lbraceOffset := pf.offset(comp.Lbrace)
	rbraceOffset := pf.offset(comp.Rbrace)

	meta = compositePatchMeta{
		filePath:     pf.filePath,
		compStart:    compStart,
		lbraceOffset: lbraceOffset,
		rbraceOffset: rbraceOffset,
	}

	var descKVs []*ast.KeyValueExpr
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		k := pf.exprCode(kv.Key)
		if k == "Description" || k == "MarkdownDescription" {
			descKVs = append(descKVs, kv)
		}
	}

	if len(descKVs) > 0 {
		hasDesc = true
		allLit := true
		var firstVal string
		var spans []byteSpan
		for idx, kv := range descKVs {
			bl, ok := kv.Value.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				allLit = false
				break
			}
			vStart := pf.offset(bl.Pos())
			vEnd := pf.offset(bl.End())
			unq, ok := unquoteGoLiteralInSource(pf.origBytes, vStart, vEnd, pf.isTmpl)
			if !ok {
				allLit = false
				break
			}
			if idx == 0 {
				firstVal = unq
			} else if unq != firstVal {
				allLit = false
				break
			}
			spans = append(spans, byteSpan{start: vStart, end: vEnd})
		}
		if allLit {
			isLitOrMissing = true
			currentDesc = firstVal
			meta.descSpans = spans
		} else {
			isLitOrMissing = false
			currentDesc = ""
		}
		return hasDesc, currentDesc, isLitOrMissing, meta
	}

	// Description is missing: compute exact non-overlapping insertion position and indentation.
	hasDesc = false
	currentDesc = ""
	isLitOrMissing = true

	src := pf.origBytes
	if !bytes.Contains(src[lbraceOffset:rbraceOffset], []byte("\n")) {
		// Single-line composite literal
		if len(comp.Elts) == 0 {
			meta.insertOffset = rbraceOffset
			meta.insertPrefix = "Description: "
			meta.insertSuffix = ""
		} else {
			lastEnd := pf.offset(comp.Elts[len(comp.Elts)-1].End())
			if bytes.Contains(src[lastEnd:rbraceOffset], []byte(",")) {
				meta.insertOffset = rbraceOffset
				meta.insertPrefix = " Description: "
				meta.insertSuffix = ","
			} else {
				meta.insertOffset = lastEnd
				meta.insertPrefix = ", Description: "
				meta.insertSuffix = ""
			}
		}
		return hasDesc, currentDesc, isLitOrMissing, meta
	}

	// Multi-line composite literal
	compScope := pf.tmpl.condScopeKey(lbraceOffset)
	fieldIndent := detectCompositeFieldIndent(pf, comp, lbraceOffset, rbraceOffset, compScope)

	bestAnchorLineEnd := -1
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyName := pf.exprCode(kv.Key)
		if !scalarAnchorKeys[keyName] {
			continue
		}
		kvStart := pf.offset(kv.Pos())
		kvEnd := pf.offset(kv.End())
		if pf.tmpl.condScopeKey(kvStart) != compScope || pf.tmpl.condScopeKey(kvEnd) != compScope {
			continue
		}
		lineEnd := findLineEnd(src, kvEnd)
		if lineEnd >= rbraceOffset || lineEnd >= len(src) {
			continue
		}
		if pf.tmpl.condScopeKey(lineEnd) != compScope {
			continue
		}
		if hasCommaBeforeLineComment(src[kvEnd:lineEnd]) {
			bestAnchorLineEnd = lineEnd
		}
	}

	if bestAnchorLineEnd >= 0 {
		meta.insertOffset = bestAnchorLineEnd + 1
		meta.insertPrefix = fieldIndent + "Description: "
		meta.insertSuffix = ",\n"
		return hasDesc, currentDesc, isLitOrMissing, meta
	}

	lbraceLineEnd := findLineEnd(src, lbraceOffset+1)
	if lbraceLineEnd < rbraceOffset && lbraceLineEnd < len(src) && isBlankOrLineComment(src[lbraceOffset+1:lbraceLineEnd]) {
		meta.insertOffset = lbraceLineEnd + 1
		meta.insertPrefix = fieldIndent + "Description: "
		meta.insertSuffix = ",\n"
		return hasDesc, currentDesc, isLitOrMissing, meta
	}

	rbraceLineStart := findLineStart(src, rbraceOffset)
	meta.insertOffset = rbraceLineStart
	meta.insertPrefix = fieldIndent + "Description: "
	meta.insertSuffix = ",\n"
	return hasDesc, currentDesc, isLitOrMissing, meta
}

func detectCompositeFieldIndent(pf *parsedVariantFile, comp *ast.CompositeLit, lbraceOffset, rbraceOffset int, compScope string) string {
	src := pf.origBytes
	lbraceLineStart := findLineStart(src, lbraceOffset)
	for pass := 0; pass < 2; pass++ {
		for _, elt := range comp.Elts {
			eltStart := pf.offset(elt.Pos())
			if pass == 0 && pf.tmpl.condScopeKey(eltStart) != compScope {
				continue
			}
			lineStart := findLineStart(src, eltStart)
			if lineStart <= lbraceLineStart {
				continue
			}
			return leadingHorizontalSpace(src[lineStart:eltStart])
		}
	}
	rbraceLineStart := findLineStart(src, rbraceOffset)
	return leadingHorizontalSpace(src[rbraceLineStart:rbraceOffset]) + "\t"
}

func findLineStart(src []byte, offset int) int {
	i := min(offset, len(src))
	for i > 0 && src[i-1] != '\n' {
		i--
	}
	return i
}

func findLineEnd(src []byte, offset int) int {
	i := max(offset, 0)
	for i < len(src) && src[i] != '\n' {
		i++
	}
	return i
}

func leadingHorizontalSpace(b []byte) string {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	return string(b[:i])
}

func hasCommaBeforeLineComment(b []byte) bool {
	for i := 0; i < len(b); i++ {
		if b[i] == ',' {
			return true
		}
		if b[i] == '/' && i+1 < len(b) && (b[i+1] == '/' || b[i+1] == '*') {
			return false
		}
	}
	return false
}

func isBlankOrLineComment(b []byte) bool {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	if i == len(b) {
		return true
	}
	return i+1 < len(b) && b[i] == '/' && b[i+1] == '/'
}
