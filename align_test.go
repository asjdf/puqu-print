package main

import "testing"

func TestAlignOffset(t *testing.T) {
	cases := []struct {
		align   string
		head    int
		content int
		want    int
		wantErr bool
	}{
		{"center", 384, 320, 32, false},
		{"center", 384, 384, 0, false},
		{"", 384, 320, 32, false},
		{"left", 384, 320, 0, false},
		{"right", 384, 320, 64, false},
		{"middle", 384, 320, 0, true},
	}
	for _, c := range cases {
		got, err := alignOffset(c.align, c.head, c.content)
		if c.wantErr {
			if err == nil {
				t.Errorf("alignOffset(%q) 应报错", c.align)
			}
			continue
		}
		if err != nil {
			t.Errorf("alignOffset(%q) 报错: %v", c.align, err)
			continue
		}
		if got != c.want {
			t.Errorf("alignOffset(%q, %d, %d) = %d, want %d", c.align, c.head, c.content, got, c.want)
		}
	}
}
