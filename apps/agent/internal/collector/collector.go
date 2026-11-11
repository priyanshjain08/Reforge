// Package collector gathers host metrics from /proc (Linux). On platforms without /proc every metric
// is reported as unsupported (nil); values are never fabricated.
package collector

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var AllMetrics = []string{"cpu_percent", "memory_percent", "memory_available_mb", "disk_percent", "disk_read_bps",
	"disk_write_bps", "network_rx_bps", "network_tx_bps", "uptime_s", "process_count"}

type Collector struct {
	prevCPUTotal, prevCPUIdle uint64
	prevNet                   [2]uint64
	prevDisk                  [2]uint64
	prevAt                    time.Time
	DiskPath                  string
}

func New() *Collector { return &Collector{DiskPath: "/"} }

type Sample struct {
	Metrics     map[string]*float64
	Unsupported []string
}

func ptr(v float64) *float64 { return &v }

func (c *Collector) Collect() Sample {
	now := time.Now()
	dt := now.Sub(c.prevAt).Seconds()
	first := c.prevAt.IsZero() || dt <= 0
	s := Sample{Metrics: map[string]*float64{}}
	fail := func(name string) { s.Metrics[name] = nil; s.Unsupported = append(s.Unsupported, name) }

	if total, idle, err := readCPU(); err == nil {
		if !first && total > c.prevCPUTotal {
			s.Metrics["cpu_percent"] = ptr(100 * (1 - float64(idle-c.prevCPUIdle)/float64(total-c.prevCPUTotal)))
		} else {
			s.Metrics["cpu_percent"] = nil // needs two samples; not unsupported
		}
		c.prevCPUTotal, c.prevCPUIdle = total, idle
	} else {
		fail("cpu_percent")
	}
	if totalKB, availKB, err := readMem(); err == nil && totalKB > 0 {
		s.Metrics["memory_percent"] = ptr(100 * (1 - availKB/totalKB))
		s.Metrics["memory_available_mb"] = ptr(availKB / 1024)
	} else {
		fail("memory_percent")
		fail("memory_available_mb")
	}
	if v, err := diskPercent(c.DiskPath); err == nil {
		s.Metrics["disk_percent"] = ptr(v)
	} else {
		fail("disk_percent")
	}
	if rd, wr, err := readDisk(); err == nil {
		if !first {
			s.Metrics["disk_read_bps"] = ptr(float64(rd-c.prevDisk[0]) * 512 / dt)
			s.Metrics["disk_write_bps"] = ptr(float64(wr-c.prevDisk[1]) * 512 / dt)
		}
		c.prevDisk = [2]uint64{rd, wr}
	} else {
		fail("disk_read_bps")
		fail("disk_write_bps")
	}
	if rx, tx, err := readNet(); err == nil {
		if !first {
			s.Metrics["network_rx_bps"] = ptr(float64(rx-c.prevNet[0]) / dt)
			s.Metrics["network_tx_bps"] = ptr(float64(tx-c.prevNet[1]) / dt)
		}
		c.prevNet = [2]uint64{rx, tx}
	} else {
		fail("network_rx_bps")
		fail("network_tx_bps")
	}
	if up, err := readUptime(); err == nil {
		s.Metrics["uptime_s"] = ptr(up)
	} else {
		fail("uptime_s")
	}
	if n, err := countProcs(); err == nil {
		s.Metrics["process_count"] = ptr(float64(n))
	} else {
		fail("process_count")
	}
	c.prevAt = now
	return s
}

func ParseCPULine(line string) (total, idle uint64, ok bool) {
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, false
	}
	for i, v := range f[1:] {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		total += n
		if i == 3 || i == 4 { // idle + iowait
			idle += n
		}
	}
	return total, idle, true
}

func readCPU() (uint64, uint64, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if t, i, ok := ParseCPULine(sc.Text()); ok {
			return t, i, nil
		}
	}
	return 0, 0, os.ErrNotExist
}

func ParseMeminfo(data string) (totalKB, availKB float64) {
	for _, l := range strings.Split(data, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		switch f[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	return
}

func readMem() (float64, float64, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	t, a := ParseMeminfo(string(b))
	return t, a, nil
}

func readUptime() (float64, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.Fields(string(b))[0], 64)
}

func readNet() (rx, tx uint64, err error) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	for _, l := range strings.Split(string(b), "\n")[2:] {
		parts := strings.SplitN(l, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		rx, tx = rx+r, tx+t
	}
	return rx, tx, nil
}

var diskRe = regexp.MustCompile(`^(sd[a-z]+|vd[a-z]+|xvd[a-z]+|nvme\d+n\d+|mmcblk\d+)$`)

func readDisk() (rd, wr uint64, err error) {
	b, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return 0, 0, err
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 10 || !diskRe.MatchString(f[2]) {
			continue
		}
		r, _ := strconv.ParseUint(f[5], 10, 64)
		w, _ := strconv.ParseUint(f[9], 10, 64)
		rd, wr = rd+r, wr+w
	}
	return rd, wr, nil
}

func countProcs() (int, error) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ents {
		if _, err := strconv.Atoi(e.Name()); err == nil {
			n++
		}
	}
	return n, nil
}
