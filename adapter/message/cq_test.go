package message

import (
	"testing"
)

func TestCQToHTML(t *testing.T) {
	input := "[CQ:at,qq=123456]"
	got := CQToHTML(input)
	expected := "<b>@123456</b> "
	if got != expected {
		t.Errorf("CQToHTML() = %v, want %v", got, expected)
	}
}
