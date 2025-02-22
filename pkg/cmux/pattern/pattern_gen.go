//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"os"
	"strconv"
	"strings"

	"github.com/bingoohuang/anyproxy/crun"
)

func main() {
	os.WriteFile("pattern.go", Generate(), 0666)
}

func Generate() []byte {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("package pattern\n\n")
	buf.WriteString("//go:generate go run pattern_gen.go\n")
	buf.WriteString("//go:generate go fmt .\n\n")

	buf.WriteString("const (\n")
	for _, pattern := range patterns {
		key := pattern[0]

		buf.WriteString("\t")
		buf.WriteString(strings.ToUpper(key))
		buf.WriteString(" = ")
		buf.WriteString(strconv.Quote(key))
		buf.WriteString("\n")
	}
	buf.WriteString(")\n\n")
	buf.WriteString("var Pattern = map[string][]string{\n")
	for _, pattern := range patterns {
		key := pattern[0]
		reg := crun.MustCompile(pattern[1])

		buf.WriteString("\t")
		buf.WriteString(strings.ToUpper(key))
		buf.WriteString(": {\n")
		reg.Range(func(s string) bool {
			buf.WriteString("\t\t")
			buf.WriteString(strconv.Quote(s))
			buf.WriteString(",\n")
			return true
		})
		buf.WriteString("\t},\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes()
}

// RegisterRegexp pattern
var patterns = [...][2]string{

	// tls
	// tls.VersionSSL30
	// tls.VersionTLS10
	// tls.VersionTLS11
	// tls.VersionTLS12
	// tls.VersionTLS13
	// 0       1       2       3       4       5       6       7       8
	// +-------+-------+-------+-------+-------+-------+-------+-------+-------+
	// |record |    version    |                   ...                         |
	// +-------+---------------+---------------+-------------------------------+
	{"tls", "^\x16\x03(\x00|\x01|\x02|\x03|\x04)"},

	// socks
	// 0       1       2       3       4       5       6       7       8
	// +-------+-------+-------+-------+-------+-------+-------+-------+-------+
	// |version|command|                       ...                             |
	// +-------+-------+-------------------------------------------------------+
	{"socks4", "^\x04(\x01|\x02)"},
	{"socks5", "^\x05(\x01|\x02|\x03)"},

	// http
	// http.MethodGet
	// http.MethodHead
	// http.MethodPost
	// http.MethodPut
	// http.MethodPatch
	// http.MethodDelete
	// http.MethodConnect
	// http.MethodOptions
	// http.MethodTrace
	{"http", "^(GET|HEAD|POST|PUT|PATCH|DELETE|CONNECT|OPTIONS|TRACE) "},
	{"http2", "^PRI \\* HTTP/2\\.0"},

	{"ssh", "^SSH-"},
}
