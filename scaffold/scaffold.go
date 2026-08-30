// Package scaffold provides code generation and Fx module registration utilities.
package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/jungo-dev/junkit/console"
)

// FeatureData holds derived name forms for scaffold templates.
type FeatureData struct {
	// Name is the feature name as given, in snake_case (e.g. "product").
	Name string
	// Pascal is Name converted to PascalCase (e.g. "Product").
	Pascal string
	// Camel is Name converted to camelCase (e.g. "product").
	Camel string
	// Table is the database table name the feature's migration and queries target.
	Table string
	// Module is the target project's Go module path.
	Module string
}

// NewFeatureData derives name forms (PascalCase, camelCase, table) from a feature name.
//
// Usage:
//
//	data := scaffold.NewFeatureData("product", "jungo", "")
//	// data.Pascal == "Product", data.Camel == "product", data.Table == "products"
func NewFeatureData(name, module, table string) FeatureData {
	if table == "" {
		table = name + "s"
	}
	return FeatureData{
		Name:   name,
		Pascal: toPascalCase(name),
		Camel:  toCamelCase(name),
		Table:  table,
		Module: module,
	}
}

// toPascalCase converts a snake_case string to PascalCase (e.g. "order_item" -> "OrderItem").
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

// toCamelCase converts a snake_case string to camelCase (e.g. "order_item" -> "orderItem").
func toCamelCase(s string) string {
	p := toPascalCase(s)
	if p == "" {
		return p
	}
	return strings.ToLower(p[:1]) + p[1:]
}

// File represents a single output template file.
type File struct {
	Path     string
	Template string
}

// GenerateConfig configures feature code generation.
type GenerateConfig struct {
	// Data is passed to every entry in Files.
	Data FeatureData
	// Files are the templated files to create.
	Files []File
	// FxFile is the path to the Fx composition root file (e.g. "internal/app/fx.go").
	FxFile string
	// ImportPath is the Go import path added to FxFile's import block.
	ImportPath string
	// Marker is the comment line in FxFile after which ModuleLine is inserted.
	Marker string
	// ModuleLine is the option-list line inserted into FxFile (e.g. "product.Module,").
	ModuleLine string
}

// Generate renders template files and registers the module in the Fx composition root.
func Generate(cfg GenerateConfig) error {
	for _, f := range cfg.Files {
		if _, err := os.Stat(f.Path); err == nil {
			return console.NewError("%s already exists — refusing to overwrite", f.Path)
		}
	}

	for _, f := range cfg.Files {
		if err := writeTemplate(f, cfg.Data); err != nil {
			return err
		}
		console.Successf("Created %s", f.Path)
	}

	if err := gofmtPaths(pathsOf(cfg.Files)); err != nil {
		console.Warnf("gofmt: %v", err)
	}

	console.Stepf("→", "Registering %s in %s", cfg.ImportPath, cfg.FxFile)
	if err := addImportLine(cfg.FxFile, cfg.ImportPath, cfg.Data.Module); err != nil {
		return console.NewError("add import: %w", err)
	}
	if err := addModuleLine(cfg.FxFile, cfg.Marker, cfg.ModuleLine); err != nil {
		return console.NewError("add module line: %w", err)
	}
	if err := gofmtPaths([]string{cfg.FxFile}); err != nil {
		console.Warnf("gofmt %s: %v", cfg.FxFile, err)
	}
	console.Successf("Registered in %s", cfg.FxFile)

	return nil
}

func writeTemplate(f File, data FeatureData) error {
	tmpl, err := template.New(f.Path).Parse(f.Template)
	if err != nil {
		return console.NewError("parse template for %s: %w", f.Path, err)
	}

	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return console.NewError("create directory for %s: %w", f.Path, err)
	}

	file, err := os.Create(f.Path)
	if err != nil {
		return console.NewError("create %s: %w", f.Path, err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return console.NewError("render %s: %w", f.Path, err)
	}
	return nil
}

func pathsOf(files []File) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return paths
}

func gofmtPaths(paths []string) error {
	goFiles := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.HasSuffix(p, ".go") {
			goFiles = append(goFiles, p)
		}
	}
	if len(goFiles) == 0 {
		return nil
	}
	return exec.Command("gofmt", append([]string{"-w"}, goFiles...)...).Run()
}
