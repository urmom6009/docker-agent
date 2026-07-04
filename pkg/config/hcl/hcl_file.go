package hcl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
)

// fileFunction implements `file(path)` and `file(path, vars)`. With a single
// argument the file contents are returned verbatim. With a vars object, the
// file is rendered as an HCL template with each key exposed as a variable.
func fileFunction(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{
			Name: "path",
			Type: cty.String,
		}},
		VarParam: &function.Parameter{
			Name:             "vars",
			Type:             cty.DynamicPseudoType,
			AllowDynamicType: true,
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			if len(args) > 2 {
				return cty.NilVal, errors.New("file() takes a path and an optional variables object")
			}
			path := args[0].AsString()

			data, readPath, err := readFileForHCL(path, baseDir)
			if err != nil {
				return cty.NilVal, fmt.Errorf("reading file %q: %w", readPath, err)
			}
			if len(args) == 1 {
				return cty.StringVal(string(data)), nil
			}
			return renderTemplate(data, readPath, args[1])
		},
	})
}

// renderTemplate evaluates data as an HCL template with the keys of vars as
// the only available variables. No functions are exposed, so templates
// cannot recursively call file().
func renderTemplate(data []byte, filename string, vars cty.Value) (cty.Value, error) {
	t := vars.Type()
	if vars.IsNull() || !vars.IsKnown() || (!t.IsObjectType() && !t.IsMapType()) {
		return cty.NilVal, errors.New(`template variables must be an object, e.g. { name = "value" }`)
	}

	expr, diags := hclsyntax.ParseTemplate(data, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return cty.NilVal, fmt.Errorf("parsing template %q: %s", filename, diags.Error())
	}

	evalCtx := &hcl.EvalContext{Variables: map[string]cty.Value{}}
	for it := vars.ElementIterator(); it.Next(); {
		k, v := it.Element()
		evalCtx.Variables[k.AsString()] = v
	}

	val, diags := expr.Value(evalCtx)
	if diags.HasErrors() {
		return cty.NilVal, fmt.Errorf("rendering template %q: %s", filename, diags.Error())
	}
	out, err := convert.Convert(val, cty.String)
	if err != nil {
		return cty.NilVal, fmt.Errorf("rendering template %q: %w", filename, err)
	}
	return out, nil
}

func readFileForHCL(path, baseDir string) ([]byte, string, error) {
	if baseDir == "" {
		data, err := os.ReadFile(path)
		return data, path, err
	}
	if !filepath.IsLocal(path) {
		return nil, path, errors.New("path must be a local relative path inside the config directory")
	}

	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, baseDir, fmt.Errorf("opening config directory %q: %w", baseDir, err)
	}
	defer root.Close()

	data, err := root.ReadFile(filepath.ToSlash(path))
	return data, filepath.Join(baseDir, path), err
}
