package server

import (
	"html/template"
	"io"
)

// TemplateExecutor defines the interface for executing templates.
// This allows for easy testing and swapping of template implementations.
type TemplateExecutor interface {
	ExecuteTemplate(wr io.Writer, name string, data interface{}) error
}

// HTMLTemplate is a wrapper around html/template.Template that provides
// context-aware escaping for HTML templates.
type HTMLTemplate struct {
	*template.Template
}

// NewHTMLTemplate creates a new HTMLTemplate.
func NewHTMLTemplate() *HTMLTemplate {
	return &HTMLTemplate{
		Template: template.New(""),
	}
}

// ExecuteTemplate executes a template with the given data and writes the output to the writer.
func (t *HTMLTemplate) ExecuteTemplate(wr io.Writer, name string, data interface{}) error {
	return t.Template.ExecuteTemplate(wr, name, data)
}
