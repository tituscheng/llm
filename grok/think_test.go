package grok

import "testing"

func TestStripThink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "Hello", want: "Hello"},
		{name: "wrapped", in: "<think>secret</think>Hello", want: "Hello"},
		{name: "around", in: "A<think>secret</think>B", want: "AB"},
		{name: "unclosed", in: "Yes<think>secret", want: "Yes"},
		{name: "only trace", in: "<think>secret</think>", want: ""},
		{name: "less than", in: "price < 5", want: "price < 5"},
		{name: "two traces", in: "<think>a</think>\n<think>b</think>ok", want: "\nok"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := stripThink(test.in); got != test.want {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestThinkFilterChunks(t *testing.T) {
	var filter thinkFilter
	var got string
	for _, chunk := range []string{"A<thi", "nk>no</thi", "nk>B"} {
		got += filter.push(chunk)
	}
	got += filter.flush()
	if got != "AB" {
		t.Fatalf("got %q", got)
	}

	filter = thinkFilter{}
	got = filter.push("yes<") + filter.flush()
	if got != "yes<" {
		t.Fatalf("partial %q", got)
	}
}
