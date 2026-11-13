// ESM → IIFE transform: mygo-music plugins are single-file ES modules; goja has no
// ESM support, so we strip export statements, collect exported names, and wrap the
// module body in a function returning an exports object.
package pluginhost

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reExportDefault = regexp.MustCompile(`(?m)^export\s+default\s+`)
	reExportFn      = regexp.MustCompile(`(?m)^export\s+(async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`)
	reExportClass   = regexp.MustCompile(`(?m)^export\s+class\s+([A-Za-z_$][\w$]*)`)
	reExportDecl    = regexp.MustCompile(`(?m)^export\s+(const|let|var)\s+([A-Za-z_$][\w$]*)`)
	reExportList    = regexp.MustCompile(`(?s)export\s*\{(.*?)\};?`)
)

// transformESM rewrites ES module source into `(function(){ ...; return {a,b}; })()`
// and returns the exported names in declaration order.
func transformESM(src string) (string, []string, error) {
	var names []string
	seen := map[string]bool{}
	// pairs of exported key → local identifier (identity unless `x as y`)
	type kv struct{ key, local string }
	var listKeys []kv
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	addPair := func(key, local string) {
		add(key)
		listKeys = append(listKeys, kv{key: key, local: local})
	}

	// export { a, b as c };  → drop the statement, record key→local pairs (before other
	// rewrites so `export default` fragments inside lists can't confuse the regexes).
	src = reExportList.ReplaceAllStringFunc(src, func(m string) string {
		sub := reExportList.FindStringSubmatch(m)
		for _, item := range strings.Split(sub[1], ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if i := strings.LastIndex(item, " as "); i >= 0 {
				addPair(strings.TrimSpace(item[i+4:]), strings.TrimSpace(item[:i]))
			} else {
				addPair(item, item)
			}
		}
		return ";"
	})
	// export default <expr>  →  const __default = <expr>
	src = reExportDefault.ReplaceAllStringFunc(src, func(m string) string {
		add("default")
		return "const __default = "
	})
	src = reExportFn.ReplaceAllStringFunc(src, func(m string) string {
		sub := reExportFn.FindStringSubmatch(m)
		add(sub[2])
		async := ""
		if sub[1] != "" {
			async = "async "
		}
		return async + "function " + sub[2]
	})
	src = reExportClass.ReplaceAllStringFunc(src, func(m string) string {
		add(reExportClass.FindStringSubmatch(m)[1])
		return "class " + reExportClass.FindStringSubmatch(m)[1]
	})
	src = reExportDecl.ReplaceAllStringFunc(src, func(m string) string {
		sub := reExportDecl.FindStringSubmatch(m)
		add(sub[2])
		return sub[1] + " " + sub[2]
	})

	if len(names) == 0 {
		return "", nil, fmt.Errorf("no exports found — is this a mygo-music plugin?")
	}
	// export-list keys map alias → local identifier; declared exports map 1:1.
	pairs := map[string]string{}
	for _, p := range listKeys {
		pairs[p.key] = p.local
	}
	keys := make([]string, 0, len(names))
	for _, n := range names {
		if n == "default" {
			keys = append(keys, `"default": __default`)
			continue
		}
		if local, ok := pairs[n]; ok && local != n {
			keys = append(keys, fmt.Sprintf("%q: %s", n, local))
		} else {
			keys = append(keys, n)
		}
	}
	wrapped := "(function(){\n" + src + "\n;return {" + strings.Join(keys, ",") + "};\n})()"
	return wrapped, names, nil
}
