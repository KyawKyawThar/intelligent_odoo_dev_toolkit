package migration

import (
	"fmt"
	"regexp"
	"strings"
)

// CodeFinding is one match found in source code.
type CodeFinding struct {
	// File is the relative path of the scanned file.
	File string `json:"file"`
	// Line is the 1-based line number of the match.
	Line int `json:"line"`
	// Snippet is the matching line, trimmed, capped at 200 chars.
	Snippet string `json:"snippet"`

	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Model       string `json:"model,omitempty"`
	Field       string `json:"field,omitempty"`
	OldName     string `json:"old_name,omitempty"`
	NewName     string `json:"new_name,omitempty"`
	Message     string `json:"message"`
	Fix         string `json:"fix,omitempty"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

// CodeScanResult holds all source-code findings from one scan run.
type CodeScanResult struct {
	Findings      []CodeFinding `json:"findings"`
	BreakingCount int           `json:"breaking_count"`
	WarningCount  int           `json:"warning_count"`
	MinorCount    int           `json:"minor_count"`
	FilesScanned  int           `json:"files_scanned"`
}

// Rule definition
type patternRule struct {
	match func(string) bool
	build func(string) []string
}

// ScanSource scans Odoo module source files (Python and XML) for references
// to deprecated patterns in the given version transition path.
//
// files maps relative filename → UTF-8 file content.
// Only .py and .xml files are processed; others are silently skipped.
//
// Cross-version scanning is handled automatically: scanning v15→v18 will
// apply all rules from v15→v16, v16→v17, and v17→v18 in one pass.
// Duplicate findings (same symbol flagged in multiple hops) are deduplicated.
func ScanSource(files map[string]string, fromVersion, toVersion string) (*CodeScanResult, error) {
	rules := PathBetween(fromVersion, toVersion)
	if rules == nil {
		return nil, fmt.Errorf("unsupported version path: %s → %s", fromVersion, toVersion)
	}

	var singleLineRules, multiLineRules []Rule
	for _, r := range rules {
		if r.OldName == "BaseModel.read_group (lazy=False)" {
			multiLineRules = append(multiLineRules, r)
		} else {
			singleLineRules = append(singleLineRules, r)
		}
	}

	pyPatterns := compilePatterns(singleLineRules, langPython)
	pyMultiPatterns := compilePatterns(multiLineRules, langPython)
	xmlPatterns := compilePatterns(rules, langXML)

	result := &CodeScanResult{Findings: []CodeFinding{}}

	for filename, content := range files {
		ext := fileExt(filename)
		switch ext {
		case ".py":
			result.FilesScanned++
			result.Findings = append(result.Findings, scanLines(filename, content, pyPatterns)...)
			result.Findings = append(result.Findings, scanContent(filename, content, pyMultiPatterns)...)
		case ".xml":
			result.FilesScanned++
			result.Findings = append(result.Findings, scanLines(filename, content, xmlPatterns)...)
		}
	}

	// Deduplicate: when the same symbol is flagged by multiple version hops
	// (e.g. user_type_id in both v15→v16 and v16→v17), keep only the first
	// occurrence per (file, line, identity) tuple.
	result.Findings = deduplicateFindings(result.Findings)

	for _, f := range result.Findings {
		switch f.Severity {
		case SeverityBreaking:
			result.BreakingCount++
		case SeverityWarning:
			result.WarningCount++
		case SeverityMinor:
			result.MinorCount++
		}
	}

	return result, nil
}

// deduplicateFindings removes duplicate findings that arise from cross-version
// scanning. When a field is removed in v15→v16 AND v16→v17, a v15→v17 scan
// would produce two findings for the same line. We keep the earliest one.
func deduplicateFindings(findings []CodeFinding) []CodeFinding {
	seen := make(map[string]bool, len(findings))
	out := make([]CodeFinding, 0, len(findings))
	for _, f := range findings {
		// Key: same file + line + what was flagged.
		// OldName covers renames/methods; Field covers field removals; Model covers model removals.
		key := fmt.Sprintf("%s|%d|%s|%s|%s", f.File, f.Line, f.OldName, f.Field, f.Model)
		if !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}

// ─── Internal types ───────────────────────────────────────────────────────────

type lang int

const (
	langPython lang = iota
	langXML
)

// compiledRule pairs a compiled regexp with its originating rule and a stable
// identity key used to deduplicate findings within the same line.
type compiledRule struct {
	re      *regexp.Regexp
	rule    Rule
	ruleKey string // FromVersion|ToVersion|Kind|Model|Field|OldName
}

// ─── Pattern compilation ──────────────────────────────────────────────────────

func compilePatterns(rules []Rule, l lang) []compiledRule {
	var out []compiledRule
	for _, rule := range rules {
		var pats []string
		if l == langPython {
			pats = pythonPatterns(rule)
		} else {
			pats = xmlPatterns(rule)
		}
		key := ruleIdentity(rule)
		for _, p := range pats {
			re, err := regexp.Compile(p)
			if err != nil {
				continue
			}
			out = append(out, compiledRule{re: re, rule: rule, ruleKey: key})
		}
	}
	return out
}

func ruleIdentity(r Rule) string {
	return r.FromVersion + "|" + r.ToVersion + "|" + r.Kind + "|" + r.Model + "|" + r.Field + "|" + r.OldName
}

// ─── Line scanner ─────────────────────────────────────────────────────────────

// scanContent applies compiled patterns to the entire file content. It's suitable
// for multi-line patterns but slower than scanLines. It calculates the line number
// for each match by counting newlines.
func scanContent(filename, content string, patterns []compiledRule) []CodeFinding {
	var findings []CodeFinding
	seen := make(map[string]bool) // Deduplicate findings within the same file

	for _, p := range patterns {
		matches := p.re.FindAllStringIndex(content, -1)
		for _, match := range matches {
			startOffset := match[0]
			lineNum := 1 + strings.Count(content[:startOffset], "\n")

			// Deduplicate based on file, line, and rule
			key := fmt.Sprintf("%s|%d|%s", filename, lineNum, p.ruleKey)
			if seen[key] {
				continue
			}
			seen[key] = true

			// Generate snippet
			lineStart := strings.LastIndex(content[:startOffset], "\n") + 1
			lineEnd := len(content)
			if nextNewline := strings.Index(content[startOffset:], "\n"); nextNewline != -1 {
				lineEnd = startOffset + nextNewline
			}
			snippet := strings.TrimSpace(content[lineStart:lineEnd])
			if len(snippet) > 200 {
				snippet = snippet[:200] + "…"
			}

			findings = append(findings, CodeFinding{
				File:        filename,
				Line:        lineNum,
				Snippet:     snippet,
				Severity:    p.rule.Severity,
				Kind:        p.rule.Kind,
				Model:       p.rule.Model,
				Field:       p.rule.Field,
				OldName:     p.rule.OldName,
				NewName:     p.rule.NewName,
				Message:     p.rule.Message,
				Fix:         p.rule.Fix,
				FromVersion: p.rule.FromVersion,
				ToVersion:   p.rule.ToVersion,
			})
		}
	}
	return findings
}

// scanLines applies compiled patterns to each line of content.
// At most one finding is emitted per (line, rule) pair.
func scanLines(filename, content string, patterns []compiledRule) []CodeFinding {
	lines := strings.Split(content, "\n")
	var findings []CodeFinding

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isCommentLine(trimmed) {
			continue
		}
		// Track rule keys that have already fired on this line.
		fired := make(map[string]bool, 4)
		for _, cr := range patterns {
			if fired[cr.ruleKey] {
				continue
			}
			if cr.re.MatchString(line) {
				fired[cr.ruleKey] = true
				snippet := trimmed
				if len(snippet) > 200 {
					snippet = snippet[:200] + "…"
				}
				findings = append(findings, CodeFinding{
					File:        filename,
					Line:        i + 1,
					Snippet:     snippet,
					Severity:    cr.rule.Severity,
					Kind:        cr.rule.Kind,
					Model:       cr.rule.Model,
					Field:       cr.rule.Field,
					OldName:     cr.rule.OldName,
					NewName:     cr.rule.NewName,
					Message:     cr.rule.Message,
					Fix:         cr.rule.Fix,
					FromVersion: cr.rule.FromVersion,
					ToVersion:   cr.rule.ToVersion,
				})
			}
		}
	}
	return findings
}

func isCommentLine(trimmed string) bool {
	return strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "<!--") ||
		strings.HasPrefix(trimmed, "//")
}

// ─── Python pattern generators ────────────────────────────────────────────────

// pythonPatterns returns regexps that detect a rule's deprecated artifact in
// Python source code.
func pythonPatterns(rule Rule) []string {
	switch rule.Kind {

	case KindAPIDecorator:
		// Matches @api.multi, @api.one, @api.returns(...) etc.
		if rule.OldName == "" {
			return nil
		}
		return []string{`@` + regexp.QuoteMeta(rule.OldName) + `\b`}

	case KindModelRemoved, KindModelRenamed:
		target := rule.Model
		if rule.OldName != "" {
			target = rule.OldName
		}
		if target == "" {
			return nil
		}
		e := regexp.QuoteMeta(target)
		return []string{
			// env['model.name'] / env["model.name"]
			`\.env\[['"]` + e + `['"]\]`,
			// _inherit = 'model.name' or _inherit = ['model.name']
			`_inherit\s*=\s*(?:['"]` + e + `['"]|\[.*?['"]` + e + `['"].*?\])`,
			// _name = 'model.name'
			`_name\s*=\s*['"]` + e + `['"]`,
			// comodel_name='model.name'
			`comodel_name\s*=\s*['"]` + e + `['"]`,
		}

	case KindFieldRemoved, KindFieldRenamed:
		fieldName := fieldTarget(rule)
		if fieldName == "" {
			return nil
		}
		e := regexp.QuoteMeta(fieldName)
		return []string{
			// Domain tuple: ('field_name', '=', ...)
			`\(['"]` + e + `['"]`,
			// Attribute access: .field_name
			`\.` + e + `\b`,
			// String literal: 'field_name' (read/write/search_read)
			`['"]` + e + `['"]`,
		}

	case KindFieldTypeChanged:
		fieldName := rule.Field
		if fieldName == "" {
			fieldName = rule.OldName
		}
		if fieldName == "" {
			return nil
		}
		e := regexp.QuoteMeta(fieldName)
		return []string{
			`\.` + e + `\b`,
			`['"]` + e + `['"]`,
		}

	case KindMethodDeprecated:
		return methodPythonPatterns(rule.OldName)

	case KindXMLIDChanged:
		old := rule.OldName
		if old == "" {
			return nil
		}
		e := regexp.QuoteMeta(old)
		return []string{
			`env\.ref\(['"]` + e + `['"]\)`,
			`ref\(['"]` + e + `['"]\)`,
		}

	case KindXMLPattern:
		// XML patterns don't apply to Python files.
		return nil
	}
	return nil
}

// methodPythonPatterns maps known deprecated method/API names to precise patterns.
// Specific cases are listed first to avoid the generic fallback producing
// false positives (e.g. _context matching self.env.context).
func methodPythonPatterns(oldName string) []string {
	for _, rule := range pythonPatternRules {
		if rule.match(oldName) {
			return rule.build(oldName)
		}
	}
	return fallbackPattern(oldName)
}

var pythonPatternRules = []patternRule{

	// ── v19 record attribute deprecations ──────────────────────────

	{
		match: func(s string) bool {
			return strings.HasSuffix(s, "._uid") || s == "record._uid"
		},
		build: func(string) []string {
			return []string{`\bself\._uid\b`}
		},
	},
	{
		match: func(s string) bool {
			return strings.HasSuffix(s, "._context") || s == "record._context"
		},
		build: func(string) []string {
			return []string{`\bself\._context\b`}
		},
	},
	{
		match: func(s string) bool {
			return strings.HasSuffix(s, "._cr") || s == "record._cr"
		},
		build: func(string) []string {
			return []string{`\bself\._cr\b`}
		},
	},

	// ── v19 removed methods ────────────────────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "_apply_ir_rules")
		},
		build: func(string) []string {
			return []string{`\._apply_ir_rules\s*\(`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "_flush_search")
		},
		build: func(string) []string {
			return []string{`def _flush_search\s*\(`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "_company_default_get")
		},
		build: func(string) []string {
			return []string{`\._company_default_get\s*\(`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "name_get") && !strings.Contains(s, "_name_get")
		},
		build: func(string) []string {
			return []string{`def name_get\s*\(\s*self\s*\)`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "get_xml_id")
		},
		build: func(string) []string {
			return []string{`\.get_xml_id\s*\(\s*\)`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "fields_get_keys")
		},
		build: func(string) []string {
			return []string{`\.fields_get_keys\s*\(\s*\)`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "get_module_resource")
		},
		build: func(string) []string {
			return []string{
				`from\s+odoo\.modules\.module\s+import\s+.*\bget_module_resource\b`,
				`\bget_module_resource\s*\(`,
			}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "_rec_name_search")
		},
		build: func(string) []string {
			return []string{`\b_rec_name_search\b`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "read_action")
		},
		build: func(string) []string {
			return []string{`\.read_action\s*\(`}
		},
	},

	// ── v19 osv.expression removal ─────────────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "odoo.osv")
		},
		build: func(string) []string {
			return []string{
				`from\s+odoo\.osv(?:\.expression)?\s+import`,
				`import\s+odoo\.osv`,
				`\bodoo\.osv\b`,
				`\bexpression\.(AND|OR|FALSE_DOMAIN|TRUE_DOMAIN|normalize_domain)\b`,
			}
		},
	},

	// ── v19 read_group lazy=False ──────────────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "read_group") && strings.Contains(s, "lazy")
		},
		build: func(string) []string {
			return []string{`\.read_group\s*\([\s\S]*?lazy\s*=\s*False`}
		},
	},

	// ── v18 search count=True ──────────────────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "search") && strings.Contains(s, "count=True")
		},
		build: func(string) []string {
			return []string{`\.search\s*\(.*count\s*=\s*True`}
		},
	},

	// ── v17 fill_temporal ─────────────────────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "fill_temporal")
		},
		build: func(string) []string {
			return []string{`\bfill_temporal\b`}
		},
	},

	// ── v16 _inherits_check ───────────────────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "_inherits_check")
		},
		build: func(string) []string {
			return []string{`\b_inherits_check\b`}
		},
	},

	// ── v18 AbstractReportDownloadMixin ───────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "AbstractReportDownloadMixin")
		},
		build: func(string) []string {
			return []string{`\bAbstractReportDownloadMixin\b`}
		},
	},

	// ── v17 env.ref ───────────────────────────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "env.ref") || strings.Contains(s, "Environment.ref")
		},
		build: func(string) []string {
			return []string{
				`(?:self\.)?env\.ref\(\s*['"][^'"]+['"]\s*\)(?!\s*#.*raise_if_not_found)`,
			}
		},
	},

	// ── v14 old API ───────────────────────────────────────────────

	{
		match: func(s string) bool {
			return strings.Contains(s, "pycompat")
		},
		build: func(string) []string {
			return []string{
				`from\s+odoo\.tools\s+import\s+pycompat`,
				`import\s+odoo\.tools\.pycompat`,
				`odoo\.tools\.pycompat`,
			}
		},
	},
	{
		match: func(s string) bool {
			return s == "openerp"
		},
		build: func(string) []string {
			return []string{
				`\bimport\s+openerp\b`,
				`\bfrom\s+openerp\b`,
				`\bopenerp\s*\.`,
			}
		},
	},
	{
		match: func(s string) bool {
			return s == "_defaults"
		},
		build: func(string) []string {
			return []string{`^\s*_defaults\s*=\s*\{`}
		},
	},
	{
		match: func(s string) bool {
			return s == "_columns"
		},
		build: func(string) []string {
			return []string{`^\s*_columns\s*=\s*\{`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "odoo.exceptions.Warning")
		},
		build: func(string) []string {
			return []string{
				`from\s+odoo\.exceptions\s+import\s+.*\bWarning\b`,
				`from\s+odoo\.exceptions\s+import\s+Warning`,
			}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "raise Warning")
		},
		build: func(string) []string {
			return []string{`raise\s+Warning\s*\(`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "cr_uid_context_signature")
		},
		build: func(string) []string {
			return []string{`def\s+\w+\s*\(\s*self\s*,\s*cr\s*,\s*uid\b`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "sudo_with_user_arg")
		},
		build: func(string) []string {
			return []string{`\.sudo\(\s*(?:self\.env\.user|self\._uid|uid|user)\s*\)`}
		},
	},
	{
		match: func(s string) bool {
			return s == "self.pool"
		},
		build: func(string) []string {
			return []string{
				`self\.pool\.get\(`,
				`self\.pool\[`,
			}
		},
	},
	{
		match: func(s string) bool {
			return s == "ustr"
		},
		build: func(string) []string {
			return []string{
				`from\s+odoo\.tools\.misc\s+import\s+.*\bustr\b`,
				`\bustr\s*\(`,
			}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "digits_tuple")
		},
		build: func(string) []string {
			return []string{`fields\.Float\([^)]*digits\s*=\s*\(\s*\d+\s*,\s*\d+\s*\)`}
		},
	},
	{
		match: func(s string) bool {
			return strings.Contains(s, "select_true")
		},
		build: func(string) []string {
			return []string{`fields\.\w+\([^)]*\bselect\s*=\s*True`}
		},
	},
}

func fallbackPattern(oldName string) []string {
	parts := strings.FieldsFunc(oldName, func(r rune) bool {
		return r == ' ' || r == '(' || r == ')' || r == '.'
	})

	if len(parts) == 0 {
		return nil
	}

	last := parts[len(parts)-1]
	if last == "" {
		return nil
	}

	return []string{`\b` + regexp.QuoteMeta(last) + `\b`}
}

// func methodPythonPatterns(oldName string) []string {
// 	switch {

// 	// ── v19 record attribute deprecations ──────────────────────────
// 	// These must be specific: self._uid not self.env.uid,
// 	// self._context not self.env.context, etc.

// 	case strings.HasSuffix(oldName, "._uid") || oldName == "record._uid":
// 		return []string{`\bself\._uid\b`}

// 	case strings.HasSuffix(oldName, "._context") || oldName == "record._context":
// 		return []string{`\bself\._context\b`}

// 	case strings.HasSuffix(oldName, "._cr") || oldName == "record._cr":
// 		return []string{`\bself\._cr\b`}

// 	// ── v19 removed methods ────────────────────────────────────────

// 	case strings.Contains(oldName, "_apply_ir_rules"):
// 		return []string{`\._apply_ir_rules\s*\(`}

// 	case strings.Contains(oldName, "_flush_search"):
// 		return []string{`def _flush_search\s*\(`}

// 	case strings.Contains(oldName, "_company_default_get"):
// 		return []string{`\._company_default_get\s*\(`}

// 	case strings.Contains(oldName, "name_get") && !strings.Contains(oldName, "_name_get"):
// 		return []string{`def name_get\s*\(\s*self\s*\)`}

// 	case strings.Contains(oldName, "get_xml_id"):
// 		return []string{`\.get_xml_id\s*\(\s*\)`}

// 	case strings.Contains(oldName, "fields_get_keys"):
// 		return []string{`\.fields_get_keys\s*\(\s*\)`}

// 	case strings.Contains(oldName, "get_module_resource"):
// 		return []string{
// 			`from\s+odoo\.modules\.module\s+import\s+.*\bget_module_resource\b`,
// 			`\bget_module_resource\s*\(`,
// 		}

// 	case strings.Contains(oldName, "_rec_name_search"):
// 		return []string{`\b_rec_name_search\b`}

// 	case strings.Contains(oldName, "read_action"):
// 		return []string{`\.read_action\s*\(`}

// 	// ── v19 osv.expression removal ─────────────────────────────────

// 	case strings.Contains(oldName, "odoo.osv"):
// 		return []string{
// 			`from\s+odoo\.osv(?:\.expression)?\s+import`,
// 			`import\s+odoo\.osv`,
// 			`\bodoo\.osv\b`,
// 			// Standalone AND/OR imported from expression
// 			`\bexpression\.(AND|OR|FALSE_DOMAIN|TRUE_DOMAIN|normalize_domain)\b`,
// 		}

// 	// ── v19 read_group lazy=False ──────────────────────────────────

// 	case strings.Contains(oldName, "read_group") && strings.Contains(oldName, "lazy"):
// 		return []string{`\.read_group\s*\(.*lazy\s*=\s*False`}

// 	// ── v18 search count=True ──────────────────────────────────────

// 	case strings.Contains(oldName, "search") && strings.Contains(oldName, "count=True"):
// 		return []string{`\.search\s*\(.*count\s*=\s*True`}

// 	// ── v17 fill_temporal ─────────────────────────────────────────

// 	case strings.Contains(oldName, "fill_temporal"):
// 		return []string{`\bfill_temporal\b`}

// 	// ── v16 _inherits_check ───────────────────────────────────────

// 	case strings.Contains(oldName, "_inherits_check"):
// 		return []string{`\b_inherits_check\b`}

// 	// ── v18 AbstractReportDownloadMixin ───────────────────────────

// 	case strings.Contains(oldName, "AbstractReportDownloadMixin"):
// 		return []string{`\bAbstractReportDownloadMixin\b`}

// 	// ── v17 env.ref raise_if_not_found ────────────────────────────

// 	case strings.Contains(oldName, "env.ref") || strings.Contains(oldName, "Environment.ref"):
// 		return []string{
// 			`(?:self\.)?env\.ref\(\s*['"][^'"]+['"]\s*\)(?!\s*#.*raise_if_not_found)`,
// 		}

// 	// ── v14 old-API patterns ──────────────────────────────────────

// 	case strings.Contains(oldName, "pycompat"):
// 		return []string{
// 			`from\s+odoo\.tools\s+import\s+pycompat`,
// 			`import\s+odoo\.tools\.pycompat`,
// 			`odoo\.tools\.pycompat`,
// 		}

// 	case oldName == "openerp":
// 		return []string{
// 			`\bimport\s+openerp\b`,
// 			`\bfrom\s+openerp\b`,
// 			`\bopenerp\s*\.`,
// 		}

// 	case oldName == "_defaults":
// 		return []string{`^\s*_defaults\s*=\s*\{`}

// 	case oldName == "_columns":
// 		return []string{`^\s*_columns\s*=\s*\{`}

// 	case strings.Contains(oldName, "odoo.exceptions.Warning"):
// 		return []string{
// 			`from\s+odoo\.exceptions\s+import\s+.*\bWarning\b`,
// 			`from\s+odoo\.exceptions\s+import\s+Warning`,
// 		}

// 	case strings.Contains(oldName, "raise Warning"):
// 		return []string{`raise\s+Warning\s*\(`}

// 	case strings.Contains(oldName, "cr_uid_context_signature"):
// 		return []string{`def\s+\w+\s*\(\s*self\s*,\s*cr\s*,\s*uid\b`}

// 	case strings.Contains(oldName, "sudo_with_user_arg"):
// 		return []string{`\.sudo\(\s*(?:self\.env\.user|self\._uid|uid|user)\s*\)`}

// 	case oldName == "self.pool":
// 		return []string{
// 			`self\.pool\.get\(`,
// 			`self\.pool\[`,
// 		}

// 	case oldName == "ustr":
// 		return []string{
// 			`from\s+odoo\.tools\.misc\s+import\s+.*\bustr\b`,
// 			`\bustr\s*\(`,
// 		}

// 	case strings.Contains(oldName, "digits_tuple"):
// 		return []string{`fields\.Float\([^)]*digits\s*=\s*\(\s*\d+\s*,\s*\d+\s*\)`}

// 	case strings.Contains(oldName, "select_true"):
// 		return []string{`fields\.\w+\([^)]*\bselect\s*=\s*True`}

// 	default:
// 		// Generic fallback: extract the last meaningful word/symbol from OldName.
// 		// Used for simple method names that don't need special casing.
// 		parts := strings.FieldsFunc(oldName, func(r rune) bool {
// 			return r == ' ' || r == '(' || r == ')' || r == '.'
// 		})
// 		if len(parts) == 0 {
// 			return nil
// 		}
// 		last := parts[len(parts)-1]
// 		if last == "" {
// 			return nil
// 		}
// 		return []string{`\b` + regexp.QuoteMeta(last) + `\b`}
// 	}
// }

// ─── XML pattern generators ───────────────────────────────────────────────────

// xmlPatterns returns regexps that detect a rule's deprecated artifact in
// Odoo XML files (views, data, etc.).
func xmlPatterns(rule Rule) []string {
	switch rule.Kind {

	case KindXMLPattern:
		// Pure XML view patterns: kanban-box, group expand="0", etc.
		if rule.OldName == "" {
			return nil
		}
		return []string{regexp.QuoteMeta(rule.OldName)}

	case KindAPIDecorator:
		// Decorators don't appear in XML files.
		return nil

	case KindModelRemoved, KindModelRenamed:
		target := rule.Model
		if rule.OldName != "" {
			target = rule.OldName
		}
		if target == "" {
			return nil
		}
		e := regexp.QuoteMeta(target)
		return []string{
			// matches <record model="model.name"> or res_model="model.name"
			`(?:res_)?model\s*=\s*["']` + e + `["']`,
			// matches src_model="model.name"
			`src_model\s*=\s*["']` + e + `["']`,
			// matches <field name="model">model.name</field>
			`<field\b[^>]*\bname\s*=\s*["']model["'][^>]*>` + e + `</field>`,
			// matches <field name="res_model">model.name</field>
			`<field\b[^>]*\bname\s*=\s*["']res_model["'][^>]*>` + e + `</field>`,
			// matches domain / eval expressions: ('model.name',
			`\(['"]` + e + `['"]`,
		}

	case KindFieldRemoved, KindFieldRenamed:
		fieldName := fieldTarget(rule)
		if fieldName == "" {
			return nil
		}
		e := regexp.QuoteMeta(fieldName)
		return []string{
			// <field name="field_name"> in views / data
			`<field\b[^>]*\bname\s*=\s*["']` + e + `["']`,
			// domain / eval attribute containing the field name
			`['"]` + e + `['"]`,
		}

	case KindFieldTypeChanged:
		fieldName := rule.Field
		if fieldName == "" {
			fieldName = rule.OldName
		}
		if fieldName == "" {
			return nil
		}
		e := regexp.QuoteMeta(fieldName)
		return []string{
			`<field\b[^>]*\bname\s*=\s*["']` + e + `["']`,
		}

	case KindMethodDeprecated:
		// Methods rarely appear directly in XML except in Python eval= expressions.
		// Use the same specific-case logic as Python for correctness.
		return methodXMLPatterns(rule.OldName)

	case KindXMLIDChanged:
		old := rule.OldName
		if old == "" {
			return nil
		}
		e := regexp.QuoteMeta(old)
		return []string{
			`ref\s*=\s*["']` + e + `["']`,
			`<field\b[^>]*\bref\s*=\s*["']` + e + `["']`,
			// Also catch it inside groups attribute
			`groups\s*=\s*["'][^"']*` + e + `[^"']*["']`,
		}
	}
	return nil
}

// methodXMLPatterns returns XML-specific patterns for method-level rules.
// Most method deprecations don't appear in XML; we only match the ones that do
// (e.g. model references inside eval= expressions, XML ID references).
func methodXMLPatterns(oldName string) []string {
	switch {
	case strings.Contains(oldName, "odoo.osv"):
		// osv references sometimes appear in eval= attributes
		return []string{`\bodoo\.osv\b`}
	default:
		// Generic: grab last meaningful word for eval= expression matching.
		parts := strings.FieldsFunc(oldName, func(r rune) bool {
			return r == ' ' || r == '(' || r == ')'
		})
		if len(parts) == 0 {
			return nil
		}
		last := parts[len(parts)-1]
		if last == "" || last == "default" || last == "True" || last == "False" {
			return nil
		}
		return []string{regexp.QuoteMeta(last)}
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// fieldTarget resolves the deprecated field/old-name for field rules.
func fieldTarget(rule Rule) string {
	if rule.OldName != "" {
		return rule.OldName
	}
	return rule.Field
}

// fileExt returns the lowercase extension including the leading dot, or "".
func fileExt(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		switch filename[i] {
		case '.':
			return strings.ToLower(filename[i:])
		case '/', '\\':
			return ""
		}
	}
	return ""
}
