package projectmap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// FuncInfo is one extracted function or method.
type FuncInfo struct {
	Name      string
	Receiver  string // Go method receiver type, "" for plain functions
	Signature string // one line, used as the node's default summary
	Exported  bool
	LineStart int
	LineEnd   int
}

// ClassInfo is one extracted type: Go structs/interfaces, TS/JS classes.
type ClassInfo struct {
	Name      string
	Kind      string // struct|interface|class
	Exported  bool
	LineStart int
	LineEnd   int
}

// Extraction is a file's structural skeleton — everything the deterministic
// pass knows without an LLM.
type Extraction struct {
	Functions []FuncInfo
	Classes   []ClassInfo
	Imports   []string
}

// Extract dispatches to the language extractor. Languages without one return
// nil — their fingerprint carries no structure signature, and any change is
// conservatively classified STRUCTURAL (same policy as Understand-Anything).
func Extract(language string, content []byte, path string) *Extraction {
	switch language {
	case "go":
		ex, err := ExtractGo(content, path)
		if err != nil {
			return nil
		}
		return ex
	case "typescript", "javascript", "svelte":
		return ExtractScript(content)
	default:
		return nil
	}
}

// ExtractGo parses with the real Go AST — better structural extraction than
// any grammar port: typed receivers, the uppercase-is-exported rule, exact
// line ranges from the FileSet.
func ExtractGo(content []byte, path string) (*Extraction, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	ex := &Extraction{}
	for _, imp := range file.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil {
			ex.Imports = append(ex.Imports, p)
		}
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			fn := FuncInfo{
				Name:      d.Name.Name,
				Exported:  d.Name.IsExported(),
				LineStart: fset.Position(d.Pos()).Line,
				LineEnd:   fset.Position(d.End()).Line,
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				fn.Receiver = receiverTypeName(d.Recv.List[0].Type)
			}
			fn.Signature = goFuncSignature(&fn, d)
			ex.Functions = append(ex.Functions, fn)
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				kind := "struct"
				if _, isIface := ts.Type.(*ast.InterfaceType); isIface {
					kind = "interface"
				}
				ex.Classes = append(ex.Classes, ClassInfo{
					Name:      ts.Name.Name,
					Kind:      kind,
					Exported:  ts.Name.IsExported(),
					LineStart: fset.Position(ts.Pos()).Line,
					LineEnd:   fset.Position(ts.End()).Line,
				})
			}
		}
	}
	return ex, nil
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // generic receiver
		return receiverTypeName(t.X)
	}
	return ""
}

func goFuncSignature(fn *FuncInfo, d *ast.FuncDecl) string {
	params := 0
	if d.Type.Params != nil {
		params = len(d.Type.Params.List)
	}
	if fn.Receiver != "" {
		return fmt.Sprintf("func (%s) %s — %d param(s)", fn.Receiver, fn.Name, params)
	}
	return fmt.Sprintf("func %s — %d param(s)", fn.Name, params)
}

// Script extraction is regex-level on purpose: it exists to feed fingerprints
// and navigation seeds, not to be a compiler. Svelte gets the same treatment
// (its <script> block is the part with structure) — tree-sitter grammars are
// the M6 upgrade path if measurements demand them.
var (
	scriptImportRe  = regexp.MustCompile(`(?m)^\s*import\s+(?:[^'"]*?\s+from\s+)?['"]([^'"]+)['"]`)
	scriptRequireRe = regexp.MustCompile(`require\(\s*['"]([^'"]+)['"]\s*\)`)
	scriptFuncRe    = regexp.MustCompile(`(?m)^\s*(export\s+)?(?:default\s+)?(?:async\s+)?function\s+(\w+)`)
	scriptArrowRe   = regexp.MustCompile(`(?m)^\s*(export\s+)?const\s+(\w+)\s*=\s*(?:async\s*)?(?:\([^)]*\)|\w+)\s*=>`)
	scriptClassRe   = regexp.MustCompile(`(?m)^\s*(export\s+)?(?:default\s+)?class\s+(\w+)`)
)

func ExtractScript(content []byte) *Extraction {
	src := string(content)
	ex := &Extraction{}

	for _, m := range scriptImportRe.FindAllStringSubmatch(src, -1) {
		ex.Imports = append(ex.Imports, m[1])
	}
	for _, m := range scriptRequireRe.FindAllStringSubmatch(src, -1) {
		ex.Imports = append(ex.Imports, m[1])
	}

	addFunc := func(exported bool, name string, offset int) {
		line := 1 + strings.Count(src[:offset], "\n")
		ex.Functions = append(ex.Functions, FuncInfo{
			Name:      name,
			Signature: "function " + name,
			Exported:  exported,
			LineStart: line,
			LineEnd:   line,
		})
	}
	for _, m := range scriptFuncRe.FindAllStringSubmatchIndex(src, -1) {
		exported := m[2] >= 0
		addFunc(exported, src[m[4]:m[5]], m[0])
	}
	for _, m := range scriptArrowRe.FindAllStringSubmatchIndex(src, -1) {
		exported := m[2] >= 0
		addFunc(exported, src[m[4]:m[5]], m[0])
	}
	for _, m := range scriptClassRe.FindAllStringSubmatchIndex(src, -1) {
		line := 1 + strings.Count(src[:m[0]], "\n")
		ex.Classes = append(ex.Classes, ClassInfo{
			Name:      src[m[4]:m[5]],
			Kind:      "class",
			Exported:  m[2] >= 0,
			LineStart: line,
			LineEnd:   line,
		})
	}
	return ex
}

// StructureStrings flattens an extraction into sorted, deterministic signature
// lines — the input of the structure fingerprint. Formatting-only edits leave
// this list identical, which is exactly what makes COSMETIC detectable.
func (ex *Extraction) StructureStrings() []string {
	if ex == nil {
		return nil
	}
	var out []string
	for _, f := range ex.Functions {
		out = append(out, fmt.Sprintf("func|%s|%s|%v", f.Receiver, f.Name, f.Exported))
	}
	for _, c := range ex.Classes {
		out = append(out, fmt.Sprintf("type|%s|%s|%v", c.Kind, c.Name, c.Exported))
	}
	for _, imp := range ex.Imports {
		out = append(out, "import|"+imp)
	}
	sort.Strings(out)
	return out
}
