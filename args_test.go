package main

import (
	"flag"
	"reflect"
	"testing"
	"time"
)

func TestReorderArgs(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{[]string{"a.png", "-paper-mm", "40", "-align", "left"}, []string{"-paper-mm", "40", "-align", "left", "a.png"}},
		{[]string{"-point", "300", "b.png"}, []string{"-point", "300", "b.png"}},
		{[]string{"a.png"}, []string{"a.png"}},
	}
	for _, c := range cases {
		if got := reorderArgs(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("reorderArgs(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestReorderArgsParses 验证调换顺序后，位置参数后面的选项真的能被解析。
func TestReorderArgsParses(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	paper := fs.Float64("paper-mm", 0, "")
	align := fs.String("align", "center", "")
	point := fs.Int("point", 200, "")
	timeout := fs.Duration("timeout", 15*time.Second, "")
	arg := []string{"a.png", "-paper-mm", "40", "-align", "left", "-point", "300", "-timeout", "5s"}
	if err := fs.Parse(reorderArgs(arg)); err != nil {
		t.Fatal(err)
	}
	if *paper != 40 || *align != "left" || *point != 300 || *timeout != 5*time.Second {
		t.Errorf("解析结果 paper=%v align=%v point=%v timeout=%v", *paper, *align, *point, *timeout)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "a.png" {
		t.Errorf("位置参数 = %v", fs.Args())
	}
}
