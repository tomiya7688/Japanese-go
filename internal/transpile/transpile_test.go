package transpile

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func compileText(t *testing.T, src string) string {
	t.Helper()
	out, err := Compile([]byte(src))
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated source is invalid: %v\n%s", err, out)
	}
	return string(out)
}

func TestNaturalSyntaxCore(t *testing.T) {
	src := `パッケージ main
"fmt" を 読み込む

関数 判定(点数 int) string {
    もし 点数 >= 80 ならば {
        "優秀" を 返す
    } それ以外 {
        "不合格" 返す
    }
}

関数 main() {
    関数の終わりに fmt.Println("終了")
    繰り返し i が 0 から 3 まで {
        fmt.Println(i)
    }
    判定(80) で 分岐 {
        "優秀" の場合:
            fmt.Println("高得点")
        それ以外の場合:
            fmt.Println("その他")
    }
}`
	out := compileText(t, src)
	checks := []string{"package main", `import "fmt"`, "func 判定", "if 点数 >= 80", `return "優秀"`, "defer fmt.Println", "switch 判定(80)", `case "優秀":`, "default:"}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("generated source missing %q:\n%s", want, out)
		}
	}
}

func TestParticleAndParticlelessReturnMatch(t *testing.T) {
	a := compileText(t, "パッケージ p\n関数 f() int {\n1 を 返す\n}\n")
	b := compileText(t, "パッケージ p\n関数 f() int {\n1 返す\n}\n")
	if a != b {
		t.Fatalf("particle variants differ:\nA:\n%s\nB:\n%s", a, b)
	}
}

func TestRangeWhileAndConcurrency(t *testing.T) {
	src := `パッケージ main
"fmt" を 読み込む
関数 main() {
    一覧 := []int{1,2}
    一覧 から _, 値 を 列挙して 繰り返し {
        fmt.Println(値)
    }
    回数 := 0
    回数 < 2 の間 繰り返し {
        回数++
    }
    fmt.Println("x") を 並行で実行する
}`
	out := compileText(t, src)
	for _, want := range []string{"for _, 値 := range 一覧", "for 回数 < 2", `go fmt.Println("x")`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

func TestSelectCase(t *testing.T) {
	src := `パッケージ main
関数 main() {
    ch := make(チャネル int)
    通信を選ぶ {
        x := <-ch の場合:
            _ = x
        それ以外の場合:
    }
}`
	out := compileText(t, src)
	if !strings.Contains(out, "select {") || !strings.Contains(out, "case x := <-ch:") || !strings.Contains(out, "default:") {
		t.Fatalf("select conversion failed:\n%s", out)
	}
}

func TestIdentifiersStringsAndCommentsArePreserved(t *testing.T) {
	src := `パッケージ main
// 返す もし 分岐 はコメント中では変換しない
関数 main() {
    りんごの数 := "返す もし 分岐"
    ファイルを保存 := りんごの数
    _ = ファイルを保存
}`
	out := compileText(t, src)
	for _, want := range []string{"// 返す もし 分岐 はコメント中では変換しない", `"返す もし 分岐"`, "りんごの数", "ファイルを保存"} {
		if !strings.Contains(out, want) {
			t.Fatalf("did not preserve %q:\n%s", want, out)
		}
	}
}

func TestNumericLoopUsesOverflowSafeDoneFlag(t *testing.T) {
	src := `パッケージ p
関数 f() {
    繰り返し i が 0 から 3 まで {
        _ = i
    }
}`
	out := compileText(t, src)
	if !strings.Contains(out, "__jgo_done_") || !strings.Contains(out, "if i == __jgo_end_") {
		t.Fatalf("expected inclusive overflow-safe loop lowering:\n%s", out)
	}
}
