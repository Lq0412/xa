package fake

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"linq-bs/server/internal/tool"
)

func TestFakeToolsRegisterAndRun(t *testing.T) {
	r := tool.NewRegistry()
	if err := Register(r); err != nil {
		t.Fatal(err)
	}
	ResetCounters()
	echo, _ := r.Get("fake_echo")
	res, err := echo.Execute(context.Background(), tool.Call{Arguments: json.RawMessage(`{"message":"hi"}`)}, tool.Env{})
	if err != nil || res.Display.Summary != "hi" {
		t.Fatal(err, res)
	}
	read, _ := r.Get("fake_read")
	if _, err = read.Execute(context.Background(), tool.Call{}, tool.Env{}); err != nil {
		t.Fatal(err)
	}
	if ReadCount() != 1 {
		t.Fatal()
	}
	write, _ := r.Get("fake_write")
	if _, err = write.Execute(context.Background(), tool.Call{}, tool.Env{}); err != nil {
		t.Fatal(err)
	}
	if WriteCount() != 1 {
		t.Fatal()
	}
	delay, _ := r.Get("fake_delay")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err = delay.Execute(ctx, tool.Call{Arguments: json.RawMessage(`{"sleep_ms":200}`)}, tool.Env{}); err == nil {
		t.Fatal("应超时")
	}
}
