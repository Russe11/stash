// Command fix-dataloaden-imports repairs the import blocks of the dataloaden-generated
// *_gen.go files in internal/api/loaders.
//
// Why this exists: github.com/vektah/dataloaden (pinned at v0.3.0, the latest published
// release) has an unfixed bug — vektah/dataloaden#54 — where its template hardcodes a
// `"time"` import AND separately emits an import for the key/value type's package. When a
// loader's key or value type lives in the `time` package (e.g. `*time.Time`, `[]time.Time`),
// the generated file imports `"time"` twice, producing `time redeclared in this block` and
// failing to compile. `go generate` is therefore not idempotent on its own.
//
// There is no newer dataloaden release to pin to, and the duplicate is never collapsed by
// gofmt. Rather than fork the generator, this tiny post-generate pass (run as the final
// //go:generate directive in dataloaders.go) deletes any duplicate import specs from each
// generated file, restoring a buildable, byte-stable result. It is a no-op on files that have
// no duplicates, so running it repeatedly is safe and deterministic.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*_gen.go"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fix-dataloaden-imports: glob: %v\n", err)
		os.Exit(1)
	}
	sort.Strings(matches)

	for _, path := range matches {
		if err := fixFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "fix-dataloaden-imports: %s: %v\n", path, err)
			os.Exit(1)
		}
	}
}

// fixFile rewrites path with any duplicate imports removed. It only writes when it actually
// removed something, so clean files are left untouched.
func fixFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return err
	}

	// Count occurrences of each (name, path) import. astutil.DeleteNamedImport deletes ALL
	// matching specs in one call (it does not remove a single occurrence), so to keep exactly
	// one we delete the whole path and then add it back once with AddNamedImport. We preserve
	// the original first-seen order of distinct imports so re-adds land deterministically.
	type key struct{ name, importPath string }
	counts := map[key]int{}
	var order []key
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return fmt.Errorf("unquote import %q: %w", imp.Path.Value, err)
		}
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		}
		k := key{name, p}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}

	removed := false
	for _, k := range order {
		if counts[k] <= 1 {
			continue
		}
		// Remove every occurrence, then re-add a single one.
		if k.name != "" {
			astutil.DeleteNamedImport(fset, file, k.name, k.importPath)
			astutil.AddNamedImport(fset, file, k.name, k.importPath)
		} else {
			astutil.DeleteImport(fset, file, k.importPath)
			astutil.AddImport(fset, file, k.importPath)
		}
		removed = true
	}

	if !removed {
		return nil
	}

	// Drop now-empty parenthesized import groups left behind by the deletion.
	cleanupEmptyImportDecls(file)

	var buf []byte
	{
		b := &writeBuf{}
		if err := format.Node(b, fset, file); err != nil {
			return err
		}
		buf = b.bytes
	}

	formatted, err := format.Source(buf)
	if err != nil {
		return err
	}

	return os.WriteFile(path, formatted, 0o644)
}

// cleanupEmptyImportDecls removes any `import ()` declarations that became empty after spec
// deletion so the formatter does not emit a dangling empty block.
func cleanupEmptyImportDecls(file *ast.File) {
	decls := file.Decls[:0]
	for _, d := range file.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT && len(gd.Specs) == 0 {
			continue
		}
		decls = append(decls, d)
	}
	file.Decls = decls
}

type writeBuf struct{ bytes []byte }

func (w *writeBuf) Write(p []byte) (int, error) {
	w.bytes = append(w.bytes, p...)
	return len(p), nil
}
