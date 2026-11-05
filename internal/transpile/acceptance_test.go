package transpile

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestKeywordVocabulary(t *testing.T) {
	tests := []struct {
		jgo string
		goKeyword string
	}{
		{"パッケージ", "package"},
		{"読み込む", "import"},
		{"関数", "func"},
		{"変数", "var"},
		{"定数", "const"},
		{"型", "type"},
		{"構造体", "struct"},
		{"インターフェース", "interface"},
		{"マップ", "map"},
		{"チャネル", "chan"},
		{"もし", "if"},
		{"それ以外", "else"},
		{"繰り返し", "for"},
		{"列挙", "range"},
		{"抜ける", "break"},
		{"繰り返しを続ける", "continue"},
		{"分岐", "switch"},
		{"場合", "case"},
		{"既定", "default"},
		{"次の分岐へ", "fallthrough"},
		{"選ぶ", "select"},
		{"行け", "go"},
		{"関数の終わりに", "defer"},
		{"返す", "return"},
		{"飛ぶ", "goto"},
	}

	if len(tests) != 25 {
		t.Fatalf("keyword table must contain 25 entries, got %d", len(tests))
	}

	for _, tt := range tests {
		t.Run(tt.jgo, func(t *testing.T) {
			got := mapKeywords(items(tt.jgo))
			if len(got) != 1 || got[0].text != tt.goKeyword {
				t.Fatalf("mapKeywords(%q) = %#v, want %q", tt.jgo, got, tt.goKeyword)
			}
		})
	}
}

func TestExamplesCompileToGoSyntax(t *testing.T) {
	paths := []string{
		"natural-syntax.jgo",
		"natural-concurrency.jgo",
	}
	for _, name := range paths {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "examples", name)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", path, err)
			}
			out, err := Compile(src)
			if err != nil {
				t.Fatalf("Compile(%s): %v", path, err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), name+".go", out, parser.AllErrors); err != nil {
				t.Fatalf("generated Go from %s is invalid: %v\n%s", path, err, out)
			}
		})
	}
}
