package controller

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed templates/*
var templatesFS embed.FS

// TemplateRenderer handles rendering of external template files
type TemplateRenderer struct {
	buildahTemplate    *template.Template
	packageTemplate    *template.Template
	entrypointTemplate *template.Template
	dockerfileTemplate *template.Template
	templatesDir       string
}

// NewTemplateRenderer creates a new template renderer with external template files
func NewTemplateRenderer() *TemplateRenderer {
	return &TemplateRenderer{
		buildahTemplate:    loadEmbeddedTemplate("templates/buildah-script.sh"),
		packageTemplate:    loadEmbeddedTemplate("templates/package.json.tmpl"),
		entrypointTemplate: loadEmbeddedTemplate("templates/entrypoint.sh.tmpl"),
		dockerfileTemplate: loadEmbeddedTemplate("templates/Dockerfile.tmpl"),
		templatesDir:       "embedded",
	}
}

// getTemplatesDir returns the templates directory path
func getTemplatesDir() string {
	// Try environment variable first
	if dir := os.Getenv("TEMPLATES_DIR"); dir != "" {
		return dir
	}

	// Try to find templates directory relative to the current file
	if cwd, err := os.Getwd(); err == nil {
		// If we're in the operator root directory
		if filepath.Base(cwd) == "operator" {
			return "internal/controller/templates"
		}
		// If we're in the controller directory
		if filepath.Base(cwd) == "controller" {
			return "templates"
		}
		// If we're in a test environment, try to find the operator root
		for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
			templatesPath := filepath.Join(dir, "internal", "controller", "templates")
			if _, err := os.Stat(templatesPath); err == nil {
				return templatesPath
			}
		}
	}

	// Default fallback
	return "internal/controller/templates"
}

// loadEmbeddedTemplate loads a template from embedded filesystem
func loadEmbeddedTemplate(templatePath string) *template.Template {
	content, err := templatesFS.ReadFile(templatePath)
	if err != nil {
		panic(fmt.Sprintf("failed to read embedded template file %s: %v", templatePath, err))
	}

	tmpl, err := template.New(templatePath).Parse(string(content))
	if err != nil {
		panic(fmt.Sprintf("failed to parse embedded template file %s: %v", templatePath, err))
	}

	return tmpl
}

// loadTemplate loads a template file and panics on error (similar to template.Must)
func loadTemplate(filepath string) *template.Template {
	content, err := os.ReadFile(filepath)
	if err != nil {
		panic(fmt.Sprintf("failed to read template file %s: %v", filepath, err))
	}

	tmpl, err := template.New(filepath).Parse(string(content))
	if err != nil {
		panic(fmt.Sprintf("failed to parse template file %s: %v", filepath, err))
	}

	return tmpl
}

// RenderBuildahScript renders the buildah script template
func (tr *TemplateRenderer) RenderBuildahScript(data TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := tr.buildahTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute buildah template: %w", err)
	}
	return buf.String(), nil
}

// RenderPackageJson renders the package.json template
func (tr *TemplateRenderer) RenderPackageJson(data TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := tr.packageTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute package template: %w", err)
	}
	return buf.String(), nil
}

// RenderDockerfile renders the Dockerfile template
func (tr *TemplateRenderer) RenderDockerfile(data TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := tr.dockerfileTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute dockerfile template: %w", err)
	}
	return buf.String(), nil
}

// RenderEntrypoint renders the entrypoint.sh template
func (tr *TemplateRenderer) RenderEntrypoint(data TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := tr.entrypointTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute entrypoint template: %w", err)
	}
	return buf.String(), nil
}
