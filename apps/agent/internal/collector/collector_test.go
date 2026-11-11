package collector

import "testing"

func TestParseCPULine(t *testing.T) {
	total, idle, ok := ParseCPULine("cpu  100 0 50 800 50 0 0 0 0 0")
	if !ok || total != 1000 || idle != 850 {
		t.Fatalf("got total=%d idle=%d ok=%v", total, idle, ok)
	}
	if _, _, ok := ParseCPULine("cpu0 1 2 3"); ok {
		t.Fatal("per-core line must be ignored")
	}
}

func TestParseMeminfo(t *testing.T) {
	total, avail := ParseMeminfo("MemTotal:       1000 kB\nMemFree: 1 kB\nMemAvailable:    250 kB\n")
	if total != 1000 || avail != 250 {
		t.Fatalf("got %v %v", total, avail)
	}
}
